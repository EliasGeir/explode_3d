package handlers

import (
	"net/http"
	"strconv"

	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type AuthorHandler struct {
	authorRepo *repository.AuthorRepository
}

func NewAuthorHandler(ar *repository.AuthorRepository) *AuthorHandler {
	return &AuthorHandler{authorRepo: ar}
}

func (h *AuthorHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	url := r.FormValue("url")

	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if _, err := h.authorRepo.Create(name, url); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	authors, _ := h.authorRepo.GetAllWithCount()
	templates.AuthorList(authors).Render(r.Context(), w)
}

func (h *AuthorHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := r.FormValue("name")
	url := r.FormValue("url")

	if err := h.authorRepo.Update(id, name, url); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	authors, _ := h.authorRepo.GetAllWithCount()
	templates.AuthorList(authors).Render(r.Context(), w)
}

func (h *AuthorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.authorRepo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	authors, _ := h.authorRepo.GetAllWithCount()
	templates.AuthorList(authors).Render(r.Context(), w)
}
