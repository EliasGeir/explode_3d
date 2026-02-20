package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	userIDKey   contextKey = "userID"
	usernameKey contextKey = "username"
)

func RequireAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ""

			// Check Authorization header first
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				tokenStr = strings.TrimPrefix(auth, "Bearer ")
			}

			// Fall back to cookie
			if tokenStr == "" {
				if c, err := r.Cookie("token"); err == nil {
					tokenStr = c.Value
				}
			}

			if tokenStr == "" {
				redirectToLogin(w, r)
				return
			}

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})
			if err != nil || !token.Valid {
				// Clear invalid cookie
				http.SetCookie(w, &http.Cookie{
					Name:     "token",
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
				})
				redirectToLogin(w, r)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				redirectToLogin(w, r)
				return
			}

			sub, _ := claims.GetSubject()
			username, _ := claims["username"].(string)

			var userID int64
			switch v := claims["sub"].(type) {
			case float64:
				userID = int64(v)
			default:
				_ = sub // keep the variable used
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = context.WithValue(ctx, usernameKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(userIDKey).(int64); ok {
		return id
	}
	return 0
}

func GetUsername(ctx context.Context) string {
	if name, ok := ctx.Value(usernameKey).(string); ok {
		return name
	}
	return ""
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") || !strings.Contains(accept, "application/json") {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
