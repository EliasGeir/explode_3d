package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"3dmodels/internal/middleware"
	"3dmodels/internal/repository"
	"3dmodels/templates"
)

const duplicatesPageSize = 20

type DuplicateHandler struct {
	dupRepo   *repository.DuplicateRepository
	modelRepo *repository.ModelRepository
	tagRepo   *repository.TagRepository
	scanPath  string
}

func NewDuplicateHandler(dr *repository.DuplicateRepository, mr *repository.ModelRepository, tr *repository.TagRepository, scanPath string) *DuplicateHandler {
	return &DuplicateHandler{dupRepo: dr, modelRepo: mr, tagRepo: tr, scanPath: scanPath}
}

func parseDuplicatePage(r *http.Request) (int, int) {
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	offset := (page - 1) * duplicatesPageSize
	return page, offset
}

// Page renders the duplicates management page.
func (h *DuplicateHandler) Page(w http.ResponseWriter, r *http.Request) {
	page, offset := parseDuplicatePage(r)
	pairs, total, err := h.dupRepo.GetPendingPairs(duplicatesPageSize, offset)
	if err != nil {
		log.Printf("[duplicates] error loading pairs: %v", err)
		http.Error(w, "Failed to load duplicates", http.StatusInternalServerError)
		return
	}

	totalPages := (total + duplicatesPageSize - 1) / duplicatesPageSize

	data := templates.DuplicatesData{
		Pairs:      pairs,
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
		IsRunning:  h.dupRepo.IsRunning(),
	}

	username := middleware.GetUsername(r.Context())
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")
	templates.LayoutWithUser("Duplicates", username, isAdmin, nil, nil, templates.DuplicatesPage(data)).Render(r.Context(), w)
}

// ListPairs returns the pairs list (HTMX partial).
func (h *DuplicateHandler) ListPairs(w http.ResponseWriter, r *http.Request) {
	page, offset := parseDuplicatePage(r)
	pairs, total, err := h.dupRepo.GetPendingPairs(duplicatesPageSize, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := (total + duplicatesPageSize - 1) / duplicatesPageSize

	data := templates.DuplicatesData{
		Pairs:      pairs,
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
		IsRunning:  h.dupRepo.IsRunning(),
	}
	templates.DuplicatePairList(data).Render(r.Context(), w)
}

// ForceDetection triggers duplicate detection manually.
func (h *DuplicateHandler) ForceDetection(w http.ResponseWriter, r *http.Request) {
	if h.dupRepo.IsRunning() {
		http.Error(w, "Detection already running", http.StatusConflict)
		return
	}

	go func() {
		if err := h.dupRepo.RunDetection(0.7); err != nil {
			log.Printf("[duplicates] force detection error: %v", err)
		}
	}()

	w.WriteHeader(http.StatusOK)
	// Self-polling element: will check status again after 2s
	fmt.Fprint(w, `<span hx-get="/api/duplicates/status" hx-target="#detection-status" hx-swap="innerHTML" hx-trigger="load delay:2s" class="text-yellow-400 text-sm">Detection running...</span>`)
}

// DetectionStatus returns the current detection status (for polling).
// Uses self-polling: the response itself triggers the next poll if still running.
func (h *DuplicateHandler) DetectionStatus(w http.ResponseWriter, r *http.Request) {
	if h.dupRepo.IsRunning() {
		// Still running: self-poll again in 2s
		fmt.Fprint(w, `<span hx-get="/api/duplicates/status" hx-target="#detection-status" hx-swap="innerHTML" hx-trigger="load delay:2s" class="text-yellow-400 text-sm">Detection running...</span>`)
		return
	}

	// Done: show result, no more polling
	count := h.dupRepo.GetPendingCount()
	fmt.Fprintf(w, `<span class="text-green-400 text-sm">%d pairs found — <a href="/duplicates" class="underline hover:text-green-300">reload</a></span>`, count)
}

// Compare renders the side-by-side comparison view for a pair.
func (h *DuplicateHandler) Compare(w http.ResponseWriter, r *http.Request) {
	pairIDStr := r.URL.Query().Get("pair_id")
	pairID, err := strconv.ParseInt(pairIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid pair ID", http.StatusBadRequest)
		return
	}

	pair, err := h.dupRepo.GetPairByID(pairID)
	if err != nil {
		http.Error(w, "Pair not found", http.StatusNotFound)
		return
	}

	model1, err := h.modelRepo.GetByID(pair.ModelID1)
	if err != nil {
		http.Error(w, "Model 1 not found", http.StatusNotFound)
		return
	}

	model2, err := h.modelRepo.GetByID(pair.ModelID2)
	if err != nil {
		http.Error(w, "Model 2 not found", http.StatusNotFound)
		return
	}

	images1 := findAllImages(filepath.Join(h.scanPath, model1.Path), h.scanPath)
	images2 := findAllImages(filepath.Join(h.scanPath, model2.Path), h.scanPath)

	data := templates.DuplicateCompareData{
		PairID:     pairID,
		Model1:     *model1,
		Model2:     *model2,
		Images1:    images1,
		Images2:    images2,
		Similarity: pair.Similarity,
	}

	templates.DuplicateCompare(data).Render(r.Context(), w)
}

// KeepBoth dismisses the pair (marks as "keep both").
func (h *DuplicateHandler) KeepBoth(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	pairID, err := strconv.ParseInt(r.FormValue("pair_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid pair ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.dupRepo.DismissPair(pairID, userID); err != nil {
		http.Error(w, "Failed to dismiss pair", http.StatusInternalServerError)
		return
	}

	log.Printf("[duplicates] pair %d dismissed by user %d", pairID, userID)
	h.ListPairs(w, r)
}

// KeepOne keeps the specified model and deletes the other.
func (h *DuplicateHandler) KeepOne(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	pairID, err := strconv.ParseInt(r.FormValue("pair_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid pair ID", http.StatusBadRequest)
		return
	}
	deleteModelID, err := strconv.ParseInt(r.FormValue("delete_model_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid model ID to delete", http.StatusBadRequest)
		return
	}

	pair, err := h.dupRepo.GetPairByID(pairID)
	if err != nil {
		http.Error(w, "Pair not found", http.StatusNotFound)
		return
	}

	if deleteModelID != pair.ModelID1 && deleteModelID != pair.ModelID2 {
		http.Error(w, "Model is not part of this pair", http.StatusBadRequest)
		return
	}

	model, err := h.modelRepo.GetByID(deleteModelID)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	// Clean up all pairs involving this model
	if err := h.dupRepo.DeletePairsForModel(deleteModelID); err != nil {
		log.Printf("[duplicates] failed to clean up pairs for model %d: %v", deleteModelID, err)
	}

	// Delete the model's folder from filesystem
	modelDir := filepath.Join(h.scanPath, model.Path)
	if err := os.RemoveAll(modelDir); err != nil {
		log.Printf("[duplicates] failed to remove directory %s: %v", modelDir, err)
		http.Error(w, "Failed to delete model folder: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete from database
	if err := h.modelRepo.Delete(deleteModelID); err != nil {
		log.Printf("[duplicates] failed to delete model %d from DB: %v", deleteModelID, err)
		http.Error(w, "Failed to delete model from database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[duplicates] model %d (%s) deleted, pair %d resolved", deleteModelID, model.Name, pairID)
	h.ListPairs(w, r)
}
