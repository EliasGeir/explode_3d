package handlers

import (
	"net/http"
	"strconv"

	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type CategoryHandler struct {
	categoryRepo *repository.CategoryRepository
}

func NewCategoryHandler(cr *repository.CategoryRepository) *CategoryHandler {
	return &CategoryHandler{categoryRepo: cr}
}

func (h *CategoryHandler) GetChildren(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	children, err := h.categoryRepo.GetChildren(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templates.CategoryChildrenList(children).Render(r.Context(), w)
}
