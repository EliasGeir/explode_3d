package i18n

import (
	"net/http"
	"strings"
)

// Middleware reads the preferred language from cookie or Accept-Language header
// and stores it in the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := DefaultLang

		// 1) Cookie
		if c, err := r.Cookie("lang"); err == nil {
			if c.Value == LangIT || c.Value == LangEN {
				lang = c.Value
			}
		} else {
			// 2) Accept-Language header
			lang = parseAcceptLanguage(r.Header.Get("Accept-Language"))
		}

		ctx := WithLocale(r.Context(), lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseAcceptLanguage(header string) string {
	if header == "" {
		return DefaultLang
	}
	// Simple parser: look for "it" or "en" in the first few entries
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		tag = strings.ToLower(tag)
		if strings.HasPrefix(tag, "it") {
			return LangIT
		}
		if strings.HasPrefix(tag, "en") {
			return LangEN
		}
	}
	return DefaultLang
}
