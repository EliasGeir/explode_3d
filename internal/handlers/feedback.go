package handlers

import (
	"net/http"
	"strconv"

	authmw "3dmodels/internal/middleware"
	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type FeedbackHandler struct {
	feedbackRepo *repository.FeedbackRepository
}

func NewFeedbackHandler(fr *repository.FeedbackRepository) *FeedbackHandler {
	return &FeedbackHandler{feedbackRepo: fr}
}

// GET /feedback — pagina admin
func (h *FeedbackHandler) Page(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	feedbacks, err := h.feedbackRepo.List(status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	categories, err := h.feedbackRepo.GetAllCategories()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	username := authmw.GetUsername(r.Context())
	isAdmin := authmw.HasRole(r.Context(), "ROLE_ADMIN")
	templates.FeedbackPage(feedbacks, categories, username, isAdmin).Render(r.Context(), w)
}

// GET /api/feedback/modal — form modal con categorie (HTMX partial)
func (h *FeedbackHandler) Modal(w http.ResponseWriter, r *http.Request) {
	categories, err := h.feedbackRepo.GetAllCategories()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.FeedbackModalContent(categories).Render(r.Context(), w)
}

// GET /api/feedback — lista feedback (HTMX partial)
func (h *FeedbackHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	feedbacks, err := h.feedbackRepo.List(status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.FeedbackList(feedbacks).Render(r.Context(), w)
}

// POST /api/feedback — invia feedback
func (h *FeedbackHandler) Submit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	title := r.FormValue("title")
	message := r.FormValue("message")
	categoryIDStr := r.FormValue("category_id")

	if title == "" || message == "" {
		http.Error(w, "Titolo e messaggio sono obbligatori", http.StatusBadRequest)
		return
	}

	userID := authmw.GetUserID(r.Context())
	var userIDPtr *int64
	if userID != 0 {
		userIDPtr = &userID
	}

	var categoryIDPtr *int64
	if categoryIDStr != "" {
		if id, err := strconv.ParseInt(categoryIDStr, 10, 64); err == nil {
			categoryIDPtr = &id
		}
	}

	if _, err := h.feedbackRepo.Create(userIDPtr, categoryIDPtr, title, message); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.FeedbackSuccess().Render(r.Context(), w)
}

// PUT /api/feedback/{id}/status — aggiorna status
func (h *FeedbackHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "ID non valido", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	status := r.FormValue("status")
	if status != "pending" && status != "read" && status != "resolved" {
		http.Error(w, "Status non valido", http.StatusBadRequest)
		return
	}

	if err := h.feedbackRepo.UpdateStatus(id, status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ricarica lista completa aggiornata
	feedbacks, err := h.feedbackRepo.List("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.FeedbackList(feedbacks).Render(r.Context(), w)
}

// DELETE /api/feedback/{id} — elimina feedback
func (h *FeedbackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "ID non valido", http.StatusBadRequest)
		return
	}

	if err := h.feedbackRepo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ritorna stringa vuota per rimuovere l'elemento (hx-swap="outerHTML")
	w.WriteHeader(http.StatusOK)
}

// --- Categorie ---

// GET /api/feedback/categories
func (h *FeedbackHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.feedbackRepo.GetAllCategories()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.FeedbackCategoryList(categories).Render(r.Context(), w)
}

// POST /api/feedback/categories
func (h *FeedbackHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	color := r.FormValue("color")
	icon := r.FormValue("icon")
	sortOrderStr := r.FormValue("sort_order")

	if name == "" {
		http.Error(w, "Nome obbligatorio", http.StatusBadRequest)
		return
	}

	sortOrder, _ := strconv.Atoi(sortOrderStr)

	if _, err := h.feedbackRepo.CreateCategory(name, color, icon, sortOrder); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	categories, _ := h.feedbackRepo.GetAllCategories()
	templates.FeedbackCategoryList(categories).Render(r.Context(), w)
}

// PUT /api/feedback/categories/{id}
func (h *FeedbackHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "ID non valido", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := r.FormValue("name")
	color := r.FormValue("color")
	icon := r.FormValue("icon")
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))

	if err := h.feedbackRepo.UpdateCategory(id, name, color, icon, sortOrder); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	categories, _ := h.feedbackRepo.GetAllCategories()
	templates.FeedbackCategoryList(categories).Render(r.Context(), w)
}

// DELETE /api/feedback/categories/{id}
func (h *FeedbackHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "ID non valido", http.StatusBadRequest)
		return
	}

	if err := h.feedbackRepo.DeleteCategory(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	categories, _ := h.feedbackRepo.GetAllCategories()
	templates.FeedbackCategoryList(categories).Render(r.Context(), w)
}
