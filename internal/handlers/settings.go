package handlers

import (
	"net/http"
	"strings"

	"3dmodels/internal/repository"
	"3dmodels/internal/scanner"
	"3dmodels/templates"
)

type SettingsHandler struct {
	settingsRepo *repository.SettingsRepository
	scanner      *scanner.Scanner
}

func NewSettingsHandler(settingsRepo *repository.SettingsRepository, sc *scanner.Scanner) *SettingsHandler {
	return &SettingsHandler{
		settingsRepo: settingsRepo,
		scanner:      sc,
	}
}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {

	ignoredFolders := scanner.DefaultIgnoredFolders()

	if val, err := h.settingsRepo.Get("ignored_folder_names"); err == nil && val != "" {

		ignoredFolders = val

	}



	excludedFolders := h.settingsRepo.GetString("excluded_folders", "")

	scannerMinDepth := h.settingsRepo.GetString("scanner_min_depth", "2")



	data := templates.SettingsData{

		AutoScanEnabled:    h.settingsRepo.GetBool("auto_scan_enabled", true),

		ScanScheduleHour:   h.settingsRepo.GetInt("scan_schedule_hour", 3),

		ScanStatus:         h.scanner.Status(),

		IgnoredFolderNames: ignoredFolders,

		ScannerMinDepth:    scannerMinDepth,

		ExcludedFolders:    excludedFolders,

	}



	lastScan, err := h.settingsRepo.Get("last_scan_at")

	if err == nil {

		data.LastScanAt = lastScan

	}



	templates.SettingsPage(data).Render(r.Context(), w)

}

func (h *SettingsHandler) SaveScannerDepth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	depth := r.FormValue("scanner_min_depth")
	h.settingsRepo.Set("scanner_min_depth", depth)

	templates.ScannerDepthSaved().Render(r.Context(), w)
}

func (h *SettingsHandler) SaveExcludedFolders(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	excludedFolders := strings.TrimSpace(r.FormValue("excluded_folders"))
	h.settingsRepo.Set("excluded_folders", excludedFolders)

	// Refresh the scanner's excluded folders cache
	h.scanner.RefreshExcludedFolders()

	templates.ExcludedFoldersSaved(excludedFolders).Render(r.Context(), w)
}

func (h *SettingsHandler) ForceScan(w http.ResponseWriter, r *http.Request) {
	h.scanner.StartScan()
	status := h.scanner.Status()
	templates.ScanStarted(status).Render(r.Context(), w)
}

func (h *SettingsHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	autoScan := r.FormValue("auto_scan_enabled") == "true"
	if autoScan {
		h.settingsRepo.Set("auto_scan_enabled", "true")
	} else {
		h.settingsRepo.Set("auto_scan_enabled", "false")
	}

	if hour := r.FormValue("scan_schedule_hour"); hour != "" {
		h.settingsRepo.Set("scan_schedule_hour", hour)
	}

	templates.SettingsSaved().Render(r.Context(), w)
}

func (h *SettingsHandler) SaveIgnoredFolders(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	value := strings.TrimSpace(r.FormValue("ignored_folder_names"))
	h.settingsRepo.Set("ignored_folder_names", value)

	templates.IgnoredFoldersSaved(value).Render(r.Context(), w)
}

func (h *SettingsHandler) AddIgnoredFolder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Empty name", http.StatusBadRequest)
		return
	}

	// Load current value
	current := scanner.DefaultIgnoredFolders()
	if val, err := h.settingsRepo.Get("ignored_folder_names"); err == nil && val != "" {
		current = val
	}

	// Check if already present (case-insensitive)
	nameLower := strings.ToLower(name)
	for _, existing := range strings.Split(current, ",") {
		if strings.ToLower(strings.TrimSpace(existing)) == nameLower {
			// Already in list
			w.WriteHeader(http.StatusOK)
			templates.IgnoredFolderAdded(name, true).Render(r.Context(), w)
			return
		}
	}

	// Append
	updated := current + "," + name
	h.settingsRepo.Set("ignored_folder_names", updated)

	templates.IgnoredFolderAdded(name, false).Render(r.Context(), w)
}
