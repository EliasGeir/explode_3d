package handlers

import (
	"net/http"
	"time"

	"3dmodels/internal/repository"
	"3dmodels/templates"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userRepo  *repository.UserRepository
	jwtSecret string
}

func NewAuthHandler(userRepo *repository.UserRepository, jwtSecret string) *AuthHandler {
	return &AuthHandler{userRepo: userRepo, jwtSecret: jwtSecret}
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	count, _ := h.userRepo.Count()
	if count == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	templates.LoginPage("").Render(r.Context(), w)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.userRepo.GetByUsername(username)
	if err != nil {
		templates.LoginPage("Invalid username or password").Render(r.Context(), w)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		templates.LoginPage("Invalid username or password").Render(r.Context(), w)
		return
	}

	h.setTokenCookie(w, user.ID, user.Username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) SetupPage(w http.ResponseWriter, r *http.Request) {
	count, _ := h.userRepo.Count()
	if count > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	templates.SetupPage("").Render(r.Context(), w)
}

func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	count, _ := h.userRepo.Count()
	if count > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if username == "" || email == "" || password == "" {
		templates.SetupPage("All fields are required").Render(r.Context(), w)
		return
	}

	if password != confirmPassword {
		templates.SetupPage("Passwords do not match").Render(r.Context(), w)
		return
	}

	if len(password) < 6 {
		templates.SetupPage("Password must be at least 6 characters").Render(r.Context(), w)
		return
	}

	if err := h.userRepo.Create(username, email, password); err != nil {
		templates.SetupPage("Failed to create user: " + err.Error()).Render(r.Context(), w)
		return
	}

	user, err := h.userRepo.GetByUsername(username)
	if err != nil {
		templates.SetupPage("User created but login failed").Render(r.Context(), w)
		return
	}

	h.setTokenCookie(w, user.ID, user.Username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) setTokenCookie(w http.ResponseWriter, userID int64, username string) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenStr, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    tokenStr,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}
