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
	rolesKey    contextKey = "roles"
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
				_ = sub
			}

			// Estrai ruoli dal claim JWT
			var roles []string
			if rawRoles, ok := claims["roles"].([]interface{}); ok {
				for _, r := range rawRoles {
					if s, ok := r.(string); ok {
						roles = append(roles, s)
					}
				}
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = context.WithValue(ctx, usernameKey, username)
			ctx = context.WithValue(ctx, rolesKey, roles)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole è un middleware che blocca l'accesso se l'utente non ha il ruolo richiesto.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !HasRole(r.Context(), role) {
				http.Error(w, "Accesso negato: permessi insufficienti.", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
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

func GetRoles(ctx context.Context) []string {
	if roles, ok := ctx.Value(rolesKey).([]string); ok {
		return roles
	}
	return nil
}

func HasRole(ctx context.Context, role string) bool {
	for _, r := range GetRoles(ctx) {
		if r == role {
			return true
		}
	}
	return false
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") || !strings.Contains(accept, "application/json") {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
