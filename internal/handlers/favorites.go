package handlers

import (
	"net/http"
	"strconv"

	"3dmodels/internal/middleware"
	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/go-chi/chi/v5"
)

type FavoritesHandler struct {
	favRepo *repository.FavoritesRepository
}

func NewFavoritesHandler(favRepo *repository.FavoritesRepository) *FavoritesHandler {
	return &FavoritesHandler{favRepo: favRepo}
}

func (h *FavoritesHandler) Add(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.favRepo.Add(userID, modelID); err != nil {
		http.Error(w, "Failed to add favorite", http.StatusInternalServerError)
		return
	}

	templates.StarButton(modelID, true).Render(r.Context(), w)
}

func (h *FavoritesHandler) Remove(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	modelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.favRepo.Remove(userID, modelID); err != nil {
		http.Error(w, "Failed to remove favorite", http.StatusInternalServerError)
		return
	}

	templates.StarButton(modelID, false).Render(r.Context(), w)
}
