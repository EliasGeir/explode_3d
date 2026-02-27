package slicer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
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
}

func NewEngine() *Engine {
	return &Engine{
		jobs: make(map[string]*models.SliceJob),
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
		Message: "Initializing...",
	}

	e.mu.Lock()
	e.jobs[jobID] = job
	e.mu.Unlock()

	go e.sliceWorker(job, req)

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
	// Schedule cleanup after 30 minutes
	go func() {
		time.Sleep(30 * time.Minute)
		e.CleanupJob(job.ID)
	}()

	// Step 1: Parse all STL files and merge
	e.updateJob(job, "slicing", 0, "Parsing STL files...")

	var merged *Mesh
	for i, fp := range req.FilePaths {
		mesh, err := ParseSTL(fp)
		if err != nil {
			e.setError(job, fmt.Sprintf("Failed to parse %s: %v", filepath.Base(fp), err))
			return
		}

		// Center on plate
		centerX := req.Profile.BuildWidthMM / 2
		centerY := req.Profile.BuildDepthMM / 2
		mesh.CenterOnPlate(centerX, centerY)

		if merged == nil {
			merged = mesh
		} else {
			merged.MergeMesh(mesh)
		}

		pct := int(float64(i+1) / float64(len(req.FilePaths)) * 5) // 0-5% for parsing
		e.updateJob(job, "slicing", pct, fmt.Sprintf("Parsed %d/%d files", i+1, len(req.FilePaths)))
	}

	// Step 2: Calculate layers
	meshHeight := float64(merged.MaxBound[2] - merged.MinBound[2])
	layerHeight := req.Settings.LayerHeightMM

	if meshHeight < 0.001 {
		e.setError(job, fmt.Sprintf("Model has zero height (bounds Z: %.4f to %.4f, %d triangles)",
			merged.MinBound[2], merged.MaxBound[2], len(merged.Triangles)))
		return
	}
	if layerHeight < 0.001 {
		layerHeight = 0.05
	}

	totalLayers := int(math.Ceil(meshHeight / layerHeight))

	e.mu.Lock()
	job.TotalLayers = totalLayers
	e.mu.Unlock()

	// Step 3: Slice each layer and RLE encode
	encodedLayers := make([][]byte, totalLayers)
	aaLevel := req.Settings.AntiAliasing
	if aaLevel < 1 {
		aaLevel = 1
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

		encodedLayers[i] = RLEEncode(layerImg)

		e.mu.Lock()
		job.CurrentLayer = i + 1
		job.Progress = 5 + int(float64(i+1)/float64(totalLayers)*85) // 5-90%
		job.Message = fmt.Sprintf("Slicing layer %d/%d", i+1, totalLayers)
		e.mu.Unlock()
	}

	// Step 4: Write photon file
	e.updateJob(job, "encoding", 92, "Writing .photon file...")

	tmpFile, err := os.CreateTemp("", "slice-*.photon")
	if err != nil {
		e.setError(job, fmt.Sprintf("Failed to create temp file: %v", err))
		return
	}

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
	tmpFile.Close()

	// Done
	e.mu.Lock()
	job.Status = "complete"
	job.Progress = 100
	job.Message = "Complete"
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
