package handlers

import (
	"net/http"
	"strconv"

	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type TagHandler struct {
	tagRepo *repository.TagRepository
}

func NewTagHandler(tr *repository.TagRepository) *TagHandler {
	return &TagHandler{tagRepo: tr}
}

func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	color := r.FormValue("color")

	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if _, err := h.tagRepo.Create(name, color); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tags, _ := h.tagRepo.GetAllWithCount()
	templates.TagList(tags).Render(r.Context(), w)
}

func (h *TagHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := r.FormValue("name")
	color := r.FormValue("color")

	if err := h.tagRepo.Update(id, name, color); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tags, _ := h.tagRepo.GetAllWithCount()
	templates.TagList(tags).Render(r.Context(), w)
}

func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.tagRepo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tags, _ := h.tagRepo.GetAllWithCount()
	templates.TagList(tags).Render(r.Context(), w)
}
