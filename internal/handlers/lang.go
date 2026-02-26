package handlers

import (
	"net/http"
	"time"

	"3dmodels/internal/i18n"
)

type LangHandler struct{}

func NewLangHandler() *LangHandler {
	return &LangHandler{}
}

// SetLang sets the language cookie and redirects back to the referring page.
func (h *LangHandler) SetLang(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang != i18n.LangIT && lang != i18n.LangEN {
		lang = i18n.DefaultLang
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lang",
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 year
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
