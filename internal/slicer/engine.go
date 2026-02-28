package slicer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"3dmodels/internal/models"
)

type Engine struct {
	mu   sync.Mutex
	jobs map[string]*models.SliceJob
	sem  chan struct{} // Concurrency semaphore
}

func NewEngine() *Engine {
	return &Engine{
		jobs: make(map[string]*models.SliceJob),
		sem:  make(chan struct{}, 1), // Only 1 concurrent slice job
	}
}

type SliceRequest struct {
	FilePaths []string
	Profile   *models.PrinterProfile
	Settings  *models.PrintSettings
	ModelName string
}

func (e *Engine) StartSlice(req SliceRequest) (string, error) {
	if len(req.FilePaths) == 0 {
		return "", fmt.Errorf("no files to slice")
	}
	if req.Profile == nil || req.Settings == nil {
		return "", fmt.Errorf("profile and settings required")
	}

	jobID := generateID()
	job := &models.SliceJob{
		ID:      jobID,
		Status:  "pending",
		Message: "Waiting in queue...",
	}

	e.mu.Lock()
	e.jobs[jobID] = job
	e.mu.Unlock()

	go func() {
		// Acquire semaphore
		e.sem <- struct{}{}
		defer func() { <-e.sem }()

		e.sliceWorker(job, req)
	}()

	return jobID, nil
}

func (e *Engine) GetJobStatus(jobID string) (*models.SliceJob, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	job, ok := e.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// Return a copy
	cp := *job
	return &cp, nil
}

func (e *Engine) GetOutputFile(jobID string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	job, ok := e.jobs[jobID]
	if !ok {
		return "", fmt.Errorf("job not found: %s", jobID)
	}
	if job.Status != "complete" {
		return "", fmt.Errorf("job not complete")
	}
	return job.OutputPath, nil
}

// CleanupJob removes a job and its output file.
func (e *Engine) CleanupJob(jobID string) {
	e.mu.Lock()
	job, ok := e.jobs[jobID]
	if ok {
		if job.OutputPath != "" {
			os.Remove(job.OutputPath)
		}
		delete(e.jobs, jobID)
	}
	e.mu.Unlock()
}

func (e *Engine) updateJob(job *models.SliceJob, status string, progress int, msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	job.Status = status
	job.Progress = progress
	job.Message = msg
}

func (e *Engine) sliceWorker(job *models.SliceJob, req SliceRequest) {
	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			e.setError(job, fmt.Sprintf("Internal error: %v", r))
		}
	}()

	// Schedule cleanup after 30 minutes
	go func() {
		time.Sleep(30 * time.Minute)
		e.CleanupJob(job.ID)
	}()

	// Step 1: Parse all STL files and merge
	e.updateJob(job, "slicing", 0, "Initializing...")

	var merged *Mesh
	validFileCount := 0
	for i, fp := range req.FilePaths {
		mesh, err := ParseSTL(fp)
		if err != nil {
			// Log the error but skip to next file instead of failing
			log.Printf("Warning: Skipping file %s: %v", filepath.Base(fp), err)
			continue
		}

		log.Printf("DEBUG: After parse - File: %s, Bounds: [%.4f,%.4f,%.4f] to [%.4f,%.4f,%.4f], Triangles: %d",
			filepath.Base(fp),
			mesh.MinBound[0], mesh.MinBound[1], mesh.MinBound[2],
			mesh.MaxBound[0], mesh.MaxBound[1], mesh.MaxBound[2],
			len(mesh.Triangles))

		validFileCount++

		// Center on plate
		centerX := req.Profile.BuildWidthMM / 2
		centerY := req.Profile.BuildDepthMM / 2
		mesh.CenterOnPlate(centerX, centerY)

		log.Printf("DEBUG: After CenterOnPlate - Bounds: [%.4f,%.4f,%.4f] to [%.4f,%.4f,%.4f]",
			mesh.MinBound[0], mesh.MinBound[1], mesh.MinBound[2],
			mesh.MaxBound[0], mesh.MaxBound[1], mesh.MaxBound[2])

		if merged == nil {
			merged = mesh
		} else {
			merged.MergeMesh(mesh)
		}

		log.Printf("DEBUG: After merge - Bounds: [%.4f,%.4f,%.4f] to [%.4f,%.4f,%.4f], Total triangles: %d",
			merged.MinBound[0], merged.MinBound[1], merged.MinBound[2],
			merged.MaxBound[0], merged.MaxBound[1], merged.MaxBound[2],
			len(merged.Triangles))

		pct := int(float64(i+1) / float64(len(req.FilePaths)) * 5) // 0-5% for parsing
		e.updateJob(job, "slicing", pct, fmt.Sprintf("Parsed %d/%d files", validFileCount, len(req.FilePaths)))
	}

	// Check if any files were successfully parsed
	if merged == nil {
		e.setError(job, "No valid STL files could be parsed. Please check the files and try again.")
		return
	}

	if validFileCount < len(req.FilePaths) {
		log.Printf("Warning: Only %d of %d files were valid and will be sliced", validFileCount, len(req.FilePaths))
	}

	// Step 2: Auto-scale to fit build volume if too large
	meshWidth := float64(merged.MaxBound[0] - merged.MinBound[0])
	meshDepth := float64(merged.MaxBound[1] - merged.MinBound[1])
	meshHeight := float64(merged.MaxBound[2] - merged.MinBound[2])

	log.Printf("DEBUG: Final mesh dimensions: W=%.2f, D=%.2f, H=%.2f (Triangles: %d)", 
		meshWidth, meshDepth, meshHeight, len(merged.Triangles))

	// Validate mesh height is not NaN or Infinite
	if math.IsNaN(meshHeight) || math.IsInf(meshHeight, 0) {
		e.setError(job, fmt.Sprintf("Invalid model geometry detected (mesh height is %.2f). The STL file may be corrupted or contain invalid vertex coordinates.", meshHeight))
		return
	}

	// Use printer profile limits or a hardcoded safety limit
	limitX := math.Max(req.Profile.BuildWidthMM, 10.0)
	limitY := math.Max(req.Profile.BuildDepthMM, 10.0)
	limitZ := math.Max(req.Profile.BuildHeightMM, 10.0)
	
	// Safety cap to prevent memory issues
	const safetyCapMM = 1000.0
	if limitZ > safetyCapMM {
		limitZ = safetyCapMM
	}

	scaleX := 1.0
	if meshWidth > limitX {
		scaleX = limitX / meshWidth
	}
	scaleY := 1.0
	if meshDepth > limitY {
		scaleY = limitY / meshDepth
	}
	scaleZ := 1.0
	if meshHeight > limitZ {
		scaleZ = limitZ / meshHeight
	}

	minScale := math.Min(scaleX, math.Min(scaleY, scaleZ))
	
	// If it's too large, it might be in micrometers instead of millimeters.
	// If the model is > build volume, we automatically scale it down to fit.
	if minScale < 0.999 || meshHeight > limitZ {
		log.Printf("Auto-scaling model: current height=%.2fmm, max height=%.2fmm, scale factor=%.6f", meshHeight, limitZ, minScale)
		merged.Scale(float32(minScale))
		
		// Re-center on plate after scaling
		centerX := req.Profile.BuildWidthMM / 2
		centerY := req.Profile.BuildDepthMM / 2
		merged.CenterOnPlate(centerX, centerY)
		
		meshHeight = float64(merged.MaxBound[2] - merged.MinBound[2])
		log.Printf("Scaled model height: %.4f (MinBound[2]=%.4f, MaxBound[2]=%.4f)", 
			meshHeight, merged.MinBound[2], merged.MaxBound[2])
	}

	if meshHeight < 0.001 {
		e.setError(job, fmt.Sprintf("Model has zero or negative height (bounds Z: %.4f to %.4f, %d triangles). Check if the STL file is valid.",
			merged.MinBound[2], merged.MaxBound[2], len(merged.Triangles)))
		return
	}

	layerHeight := req.Settings.LayerHeightMM

	floatLayers := math.Ceil(meshHeight / layerHeight)
	if floatLayers > 1000000 {
		e.setError(job, fmt.Sprintf("Too many layers (%.0f). Please increase layer height or check model scale.", floatLayers))
		return
	}

	totalLayers := int(floatLayers)
	if totalLayers <= 0 {
		e.setError(job, fmt.Sprintf("Invalid layer count: %d", totalLayers))
		return
	}

	e.mu.Lock()
	job.TotalLayers = totalLayers
	e.mu.Unlock()

	// Step 3: Slice each layer
	isDLP := req.Profile.FileFormat == "dlp"
	var encodedLayers [][]byte
	var dlpLayers []*image.Gray

	if isDLP {
		dlpLayers = make([]*image.Gray, totalLayers)
	} else {
		encodedLayers = make([][]byte, totalLayers)
	}

	aaLevel := req.Settings.AntiAliasing
	if aaLevel < 1 {
		aaLevel = 1
	}
	if aaLevel > 8 {
		aaLevel = 8 // Hard limit for safety
	}

	// Validate resolution * AA to prevent huge allocations
	maxRes := 65535 // Max dimension
	if req.Profile.ResolutionX*aaLevel > maxRes || req.Profile.ResolutionY*aaLevel > maxRes {
		e.setError(job, fmt.Sprintf("Effective resolution (%d x %d) with AA %dx is too high.", 
			req.Profile.ResolutionX*aaLevel, req.Profile.ResolutionY*aaLevel, aaLevel))
		return
	}

	offsetX := req.Profile.BuildWidthMM / 2
	offsetY := req.Profile.BuildDepthMM / 2

	for i := 0; i < totalLayers; i++ {
		// Slice at middle of each layer. After CenterOnPlate, MinBound[2] == 0
		z := float32(float64(i)*layerHeight + layerHeight/2)

		contours := SliceAtZ(merged, z)
		SortContoursByArea(contours)

		var layerImg *image.Gray
		if aaLevel > 1 {
			layerImg = RasterizeLayerAA(contours, req.Profile, offsetX, offsetY, aaLevel)
		} else {
			layerImg = RasterizeLayer(contours, req.Profile, offsetX, offsetY)
		}

		if isDLP {
			dlpLayers[i] = layerImg
		} else {
			encodedLayers[i] = RLEEncode(layerImg)
		}

		e.mu.Lock()
		job.CurrentLayer = i + 1
		job.Progress = 5 + int(float64(i+1)/float64(totalLayers)*85) // 5-90%
		job.Message = fmt.Sprintf("Slicing layer %d/%d", i+1, totalLayers)
		e.mu.Unlock()
	}

	// Step 4: Write output file
	ext := "photon"
	if isDLP {
		ext = "dlp"
	}
	e.updateJob(job, "encoding", 92, fmt.Sprintf("Writing .%s file...", ext))

	tmpFile, err := os.CreateTemp("", "slice-*."+ext)
	if err != nil {
		e.setError(job, fmt.Sprintf("Failed to create temp file: %v", err))
		return
	}

	if isDLP {
		if err := WriteDLPFile(tmpFile, req.Profile, req.Settings, dlpLayers); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			e.setError(job, fmt.Sprintf("Failed to write DLP file: %v", err))
			return
		}
	} else {
		header := PhotonHeader{
			BedXMM:           float32(req.Profile.BuildWidthMM),
			BedYMM:           float32(req.Profile.BuildDepthMM),
			BedZMM:           float32(req.Profile.BuildHeightMM),
			LayerHeightMM:    float32(req.Settings.LayerHeightMM),
			ExposureS:        float32(req.Settings.ExposureTimeS),
			BottomExposureS:  float32(req.Settings.BottomExposureS),
			BottomLayers:     uint32(req.Settings.BottomLayers),
			ResolutionX:      uint32(req.Profile.ResolutionX),
			ResolutionY:      uint32(req.Profile.ResolutionY),
			LayerCount:       uint32(totalLayers),
			LiftHeightMM:     float32(req.Settings.LiftHeightMM),
			LiftSpeedMMPS:    float32(req.Settings.LiftSpeedMMPS),
			RetractSpeedMMPS: float32(req.Settings.RetractSpeedMMPS),
			AntiAliasing:     uint32(aaLevel),
		}

		if err := WritePhotonFile(tmpFile, header, encodedLayers); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			e.setError(job, fmt.Sprintf("Failed to write photon file: %v", err))
			return
		}
	}
	tmpFile.Close()

	// Done
	e.mu.Lock()
	job.Status = "complete"
	job.Progress = 100
	job.Message = "Complete"
	job.Extension = ext
	job.OutputPath = tmpFile.Name()
	e.mu.Unlock()
}

func (e *Engine) setError(job *models.SliceJob, msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	job.Status = "error"
	job.Message = msg
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
