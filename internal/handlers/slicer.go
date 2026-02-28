package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"3dmodels/internal/middleware"
	"3dmodels/internal/models"
	"3dmodels/internal/repository"
	"3dmodels/internal/slicer"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type SlicerHandler struct {
	slicerRepo *repository.SlicerRepository
	modelRepo  *repository.ModelRepository
	engine     *slicer.Engine
	scanPath   string
}

func NewSlicerHandler(sr *repository.SlicerRepository, mr *repository.ModelRepository, engine *slicer.Engine, scanPath string) *SlicerHandler {
	return &SlicerHandler{slicerRepo: sr, modelRepo: mr, engine: engine, scanPath: scanPath}
}

func (h *SlicerHandler) Page(w http.ResponseWriter, r *http.Request) {
	username := middleware.GetUsername(r.Context())
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")

	profiles, _ := h.slicerRepo.GetAllProfiles()

	// Parse file IDs and model ID from query
	fileIDsStr := r.URL.Query().Get("files")
	modelIDStr := r.URL.Query().Get("model_id")

	// If no files or model_id provided, redirect to home.
	// This ensures the slicer is only accessible from the model detail page as requested.
	if fileIDsStr == "" || modelIDStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var files []models.ModelFile
	var modelID int64
	var modelName string

	if modelIDStr != "" {
		mid, err := strconv.ParseInt(modelIDStr, 10, 64)
		if err == nil {
			modelID = mid
			model, err := h.modelRepo.GetByID(mid)
			if err == nil {
				modelName = model.Name
			}
		}
	}

	if fileIDsStr != "" {
		for _, idStr := range strings.Split(fileIDsStr, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				continue
			}
			// Get file from model files
			if modelID > 0 {
				modelFiles, err := h.modelRepo.GetFilesByModel(modelID)
				if err == nil {
					for _, f := range modelFiles {
						if f.ID == id {
							files = append(files, f)
						}
					}
				}
			}
		}
	}

	// Determine which profile to select: cookie > first profile
	selectedProfileID := int64(0)
	if cookie, err := r.Cookie("slicer_profile_id"); err == nil {
		if pid, err := strconv.ParseInt(cookie.Value, 10, 64); err == nil {
			// Verify this profile still exists
			for _, p := range profiles {
				if p.ID == pid {
					selectedProfileID = pid
					break
				}
			}
		}
	}
	if selectedProfileID == 0 && len(profiles) > 0 {
		selectedProfileID = profiles[0].ID
	}

	// Load default settings for selected profile
	var settings *models.PrintSettings
	if selectedProfileID > 0 {
		settings, _ = h.slicerRepo.GetDefaultSettings(selectedProfileID)
	}

	data := templates.SlicerPageData{
		Files:             files,
		ModelID:           modelID,
		ModelName:         modelName,
		Profiles:          profiles,
		Settings:          settings,
		SelectedProfileID: selectedProfileID,
		Username:          username,
		IsAdmin:           isAdmin,
	}

	templates.LayoutWithUser(
		"Slicer",
		username,
		isAdmin,
		nil,
		nil,
		templates.SlicerPage(data),
	).Render(r.Context(), w)
}

func (h *SlicerHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.slicerRepo.GetAllProfiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	selectedID := int64(0)
	if idStr := r.URL.Query().Get("selected"); idStr != "" {
		selectedID, _ = strconv.ParseInt(idStr, 10, 64)
	}

	templates.ProfileSelector(profiles, selectedID).Render(r.Context(), w)
}

func (h *SlicerHandler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	p := &models.PrinterProfile{
		Name:          r.FormValue("name"),
		Manufacturer:  r.FormValue("manufacturer"),
		BuildWidthMM:  parseFloat(r.FormValue("build_width_mm")),
		BuildDepthMM:  parseFloat(r.FormValue("build_depth_mm")),
		BuildHeightMM: parseFloat(r.FormValue("build_height_mm")),
		ResolutionX:   parseInt(r.FormValue("resolution_x")),
		ResolutionY:   parseInt(r.FormValue("resolution_y")),
		PixelSizeUM:   parseFloat(r.FormValue("pixel_size_um")),
		FileFormat:    r.FormValue("file_format"),
	}

	if err := h.slicerRepo.CreateProfile(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Also create default settings
	s := &models.PrintSettings{
		Name:             "Default",
		ProfileID:        p.ID,
		LayerHeightMM:    0.05,
		ExposureTimeS:    2.0,
		BottomExposureS:  30.0,
		BottomLayers:     5,
		LiftHeightMM:     6.0,
		LiftSpeedMMPS:    2.0,
		RetractSpeedMMPS: 4.0,
		AntiAliasing:     1,
		IsDefault:        true,
	}
	h.slicerRepo.CreateSettings(s)

	profiles, _ := h.slicerRepo.GetAllProfiles()

	// Return appropriate template based on context
	if isSettingsRequest(r) {
		templates.PrinterProfilesList(profiles).Render(r.Context(), w)
	} else {
		templates.ProfileSelector(profiles, p.ID).Render(r.Context(), w)
	}
}

func (h *SlicerHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	p := &models.PrinterProfile{
		ID:            id,
		Name:          r.FormValue("name"),
		Manufacturer:  r.FormValue("manufacturer"),
		BuildWidthMM:  parseFloat(r.FormValue("build_width_mm")),
		BuildDepthMM:  parseFloat(r.FormValue("build_depth_mm")),
		BuildHeightMM: parseFloat(r.FormValue("build_height_mm")),
		ResolutionX:   parseInt(r.FormValue("resolution_x")),
		ResolutionY:   parseInt(r.FormValue("resolution_y")),
		PixelSizeUM:   parseFloat(r.FormValue("pixel_size_um")),
		FileFormat:    r.FormValue("file_format"),
	}

	if err := h.slicerRepo.UpdateProfile(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	profiles, _ := h.slicerRepo.GetAllProfiles()

	if isSettingsRequest(r) {
		templates.PrinterProfilesList(profiles).Render(r.Context(), w)
	} else {
		templates.ProfileSelector(profiles, id).Render(r.Context(), w)
	}
}

func (h *SlicerHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.slicerRepo.DeleteProfile(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	profiles, _ := h.slicerRepo.GetAllProfiles()

	if isSettingsRequest(r) {
		templates.PrinterProfilesList(profiles).Render(r.Context(), w)
	} else {
		templates.ProfileSelector(profiles, 0).Render(r.Context(), w)
	}
}

// isSettingsRequest checks if the HTMX request originated from the settings page
func isSettingsRequest(r *http.Request) bool {
	referer := r.Header.Get("Hx-Current-Url")
	if referer == "" {
		referer = r.Header.Get("Referer")
	}
	return strings.Contains(referer, "/settings")
}

func (h *SlicerHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	profileIDStr := chi.URLParam(r, "profileId")
	profileID, err := strconv.ParseInt(profileIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	settings, err := h.slicerRepo.GetDefaultSettings(profileID)
	if err != nil {
		// Return empty default settings
		settings = &models.PrintSettings{
			ProfileID:        profileID,
			LayerHeightMM:    0.05,
			ExposureTimeS:    2.0,
			BottomExposureS:  30.0,
			BottomLayers:     5,
			LiftHeightMM:     6.0,
			LiftSpeedMMPS:    2.0,
			RetractSpeedMMPS: 4.0,
			AntiAliasing:     1,
		}
	}

	profile, _ := h.slicerRepo.GetProfileByID(profileID)

	templates.PrintSettingsForm(settings, profile).Render(r.Context(), w)
}

func (h *SlicerHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	s := &models.PrintSettings{
		ID:               id,
		LayerHeightMM:    parseFloat(r.FormValue("layer_height_mm")),
		ExposureTimeS:    parseFloat(r.FormValue("exposure_time_s")),
		BottomExposureS:  parseFloat(r.FormValue("bottom_exposure_s")),
		BottomLayers:     parseInt(r.FormValue("bottom_layers")),
		LiftHeightMM:     parseFloat(r.FormValue("lift_height_mm")),
		LiftSpeedMMPS:    parseFloat(r.FormValue("lift_speed_mmps")),
		RetractSpeedMMPS: parseFloat(r.FormValue("retract_speed_mmps")),
		AntiAliasing:     parseInt(r.FormValue("anti_aliasing")),
	}

	if err := h.slicerRepo.UpdateSettings(s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<div class="text-green-400 text-sm mt-2">Saved</div>`))
}

func (h *SlicerHandler) StartSlice(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	fileIDsStr := r.FormValue("file_ids")
	profileIDStr := r.FormValue("profile_id")
	modelName := r.FormValue("model_name")

	profileID, err := strconv.ParseInt(profileIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	profile, err := h.slicerRepo.GetProfileByID(profileID)
	if err != nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	settings, err := h.slicerRepo.GetDefaultSettings(profileID)
	if err != nil {
		http.Error(w, "Settings not found", http.StatusNotFound)
		return
	}

	// Override settings from form if provided
	if v := r.FormValue("layer_height_mm"); v != "" {
		settings.LayerHeightMM = parseFloat(v)
	}
	if v := r.FormValue("exposure_time_s"); v != "" {
		settings.ExposureTimeS = parseFloat(v)
	}
	if v := r.FormValue("bottom_exposure_s"); v != "" {
		settings.BottomExposureS = parseFloat(v)
	}
	if v := r.FormValue("bottom_layers"); v != "" {
		settings.BottomLayers = parseInt(v)
	}
	if v := r.FormValue("lift_height_mm"); v != "" {
		settings.LiftHeightMM = parseFloat(v)
	}
	if v := r.FormValue("lift_speed_mmps"); v != "" {
		settings.LiftSpeedMMPS = parseFloat(v)
	}
	if v := r.FormValue("retract_speed_mmps"); v != "" {
		settings.RetractSpeedMMPS = parseFloat(v)
	}
	if v := r.FormValue("anti_aliasing"); v != "" {
		settings.AntiAliasing = parseInt(v)
	}

	// Resolve file paths
	modelIDStr := r.FormValue("model_id")
	modelID, _ := strconv.ParseInt(modelIDStr, 10, 64)

	var filePaths []string
	if fileIDsStr != "" && modelID > 0 {
		modelFiles, err := h.modelRepo.GetFilesByModel(modelID)
		if err != nil {
			http.Error(w, "Failed to get model files", http.StatusInternalServerError)
			return
		}

		fileIDs := make(map[int64]bool)
		for _, idStr := range strings.Split(fileIDsStr, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err == nil {
				fileIDs[id] = true
			}
		}

		for _, f := range modelFiles {
			if fileIDs[f.ID] {
				absPath := filepath.Join(h.scanPath, f.FilePath)
				filePaths = append(filePaths, absPath)
			}
		}
	}

	if len(filePaths) == 0 {
		http.Error(w, "No valid files to slice", http.StatusBadRequest)
		return
	}

	req := slicer.SliceRequest{
		FilePaths: filePaths,
		Profile:   profile,
		Settings:  settings,
		ModelName: modelName,
	}

	jobID, err := h.engine.StartSlice(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job, _ := h.engine.GetJobStatus(jobID)
	templates.SliceProgress(job).Render(r.Context(), w)
}

func (h *SlicerHandler) SliceStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	job, err := h.engine.GetJobStatus(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if job.Status == "complete" {
		fileName := fmt.Sprintf("model.%s", job.Extension)
		templates.SliceComplete(job, fileName).Render(r.Context(), w)
		return
	}

	templates.SliceProgress(job).Render(r.Context(), w)
}

func (h *SlicerHandler) Download(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	job, err := h.engine.GetJobStatus(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	outputPath, err := h.engine.GetOutputFile(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	fileName := fmt.Sprintf("model.%s", job.Extension)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	http.ServeFile(w, r, outputPath)

	// Cleanup after download
	go h.engine.CleanupJob(jobID)
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
