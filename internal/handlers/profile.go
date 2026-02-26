package handlers

import (
	"net/http"

	"3dmodels/internal/middleware"
	"3dmodels/internal/repository"
	"3dmodels/templates"
)

type ProfileHandler struct {
	userRepo *repository.UserRepository
	favRepo  *repository.FavoritesRepository
}

func NewProfileHandler(userRepo *repository.UserRepository, favRepo *repository.FavoritesRepository) *ProfileHandler {
	return &ProfileHandler{userRepo: userRepo, favRepo: favRepo}
}

func (h *ProfileHandler) Page(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	favorites, _ := h.favRepo.GetFavoritesGrouped(userID)
	isAdmin := middleware.HasRole(r.Context(), "ROLE_ADMIN")

	data := templates.ProfileData{
		User:      *user,
		Favorites: favorites,
		IsAdmin:   isAdmin,
	}
	templates.ProfilePage(data).Render(r.Context(), w)
}

func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	email := r.FormValue("email")

	if username == "" || email == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<div class="px-4 py-2 bg-red-600/20 border border-red-600 rounded text-red-400 text-sm">Username and email are required.</div>`))
		return
	}

	if err := h.userRepo.UpdateProfile(userID, username, email); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="px-4 py-2 bg-red-600/20 border border-red-600 rounded text-red-400 text-sm">Failed to update profile. Username or email may already be taken.</div>`))
		return
	}

	w.Write([]byte(`<div class="px-4 py-2 bg-green-600/20 border border-green-600 rounded text-green-400 text-sm">Profile updated successfully.</div>`))
}

func (h *ProfileHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")

	if currentPassword == "" || newPassword == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<div class="px-4 py-2 bg-red-600/20 border border-red-600 rounded text-red-400 text-sm">Both fields are required.</div>`))
		return
	}

	ok, err := h.userRepo.VerifyPassword(userID, currentPassword)
	if err != nil || !ok {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<div class="px-4 py-2 bg-red-600/20 border border-red-600 rounded text-red-400 text-sm">Current password is incorrect.</div>`))
		return
	}

	if err := h.userRepo.UpdatePassword(userID, newPassword); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="px-4 py-2 bg-red-600/20 border border-red-600 rounded text-red-400 text-sm">Failed to update password.</div>`))
		return
	}

	w.Write([]byte(`<div class="px-4 py-2 bg-green-600/20 border border-green-600 rounded text-green-400 text-sm">Password changed successfully.</div>`))
}

func (h *ProfileHandler) FavoritesList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	favorites, _ := h.favRepo.GetFavoritesGrouped(userID)
	templates.FavoritesGrid(favorites).Render(r.Context(), w)
}
