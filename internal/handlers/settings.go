package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"3dmodels/internal/middleware"
	"3dmodels/internal/repository"
	"3dmodels/internal/scanner"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type SettingsHandler struct {
	settingsRepo *repository.SettingsRepository
	scanner      *scanner.Scanner
	userRepo     *repository.UserRepository
}

func NewSettingsHandler(settingsRepo *repository.SettingsRepository, sc *scanner.Scanner, userRepo *repository.UserRepository) *SettingsHandler {
	return &SettingsHandler{
		settingsRepo: settingsRepo,
		scanner:      sc,
		userRepo:     userRepo,
	}
}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = "scanner"
	}

	ignoredFolders := scanner.DefaultIgnoredFolders()
	if val, err := h.settingsRepo.Get("ignored_folder_names"); err == nil && val != "" {
		ignoredFolders = val
	}

	excludedFolders := h.settingsRepo.GetString("excluded_folders", "")

	excludedPathsStr := h.settingsRepo.GetString("excluded_paths", "")
	var excludedPaths []string
	for _, p := range strings.Split(excludedPathsStr, "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			excludedPaths = append(excludedPaths, p)
		}
	}

	scannerMinDepth := h.settingsRepo.GetString("scanner_min_depth", "2")

	users, _ := h.userRepo.GetAll()

	username := middleware.GetUsername(r.Context())

	data := templates.SettingsData{
		AutoScanEnabled:    h.settingsRepo.GetBool("auto_scan_enabled", true),
		ScanScheduleHour:   h.settingsRepo.GetInt("scan_schedule_hour", 3),
		ScanStatus:         h.scanner.Status(),
		IgnoredFolderNames: ignoredFolders,
		ScannerMinDepth:    scannerMinDepth,
		ExcludedFolders:    excludedFolders,
		ExcludedPaths:      excludedPaths,
		Users:              users,
		ActiveTab:          activeTab,
		Username:           username,
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

func (h *SettingsHandler) RemoveExcludedPath(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	h.scanner.RemoveExcludedPath(path)

	// Return updated list
	val := h.settingsRepo.GetString("excluded_paths", "")
	var paths []string
	for _, p := range strings.Split(val, "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	templates.ExcludedPathsList(paths).Render(r.Context(), w)
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

func (h *SettingsHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if username == "" || email == "" || password == "" {
		templates.CreateUserForm("All fields are required").Render(r.Context(), w)
		return
	}

	if len(password) < 6 {
		templates.CreateUserForm("Password must be at least 6 characters").Render(r.Context(), w)
		return
	}

	if err := h.userRepo.Create(username, email, password); err != nil {
		templates.CreateUserForm("Failed to create user: " + err.Error()).Render(r.Context(), w)
		return
	}

	// Re-render user list via OOB swap + success form
	users, _ := h.userRepo.GetAll()
	w.Header().Set("Content-Type", "text/html")
	templates.CreateUserSuccess().Render(r.Context(), w)
	// OOB update for user list
	w.Write([]byte(`<div id="users-section" hx-swap-oob="innerHTML">`))
	templates.UsersList(users).Render(r.Context(), w)
	w.Write([]byte(`</div>`))
}

func (h *SettingsHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Don't allow deleting the last user
	count, _ := h.userRepo.Count()
	if count <= 1 {
		http.Error(w, "Cannot delete the last user", http.StatusBadRequest)
		return
	}

	if err := h.userRepo.Delete(id); err != nil {
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	users, _ := h.userRepo.GetAll()
	templates.UsersList(users).Render(r.Context(), w)
}
