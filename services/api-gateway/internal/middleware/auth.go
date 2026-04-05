package middleware

import (
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/room-booking/pkg/httputil"
	pkgjwt "github.com/room-booking/pkg/jwt"
)

type AuthMiddleware struct {
	publicKey *rsa.PublicKey
}

func NewAuthMiddleware(publicKey *rsa.PublicKey) *AuthMiddleware {
	return &AuthMiddleware{publicKey: publicKey}
}

// RequireAuth validates JWT and injects X-User-ID and X-User-Role headers
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			httputil.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token")
			return
		}

		claims, err := pkgjwt.ValidateToken(m.publicKey, token)
		if err != nil {
			httputil.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
			return
		}

		// Inject user info into request headers for downstream services
		r.Header.Set("X-User-ID", claims.UserID.String())
		r.Header.Set("X-User-Role", claims.Role)

		next.ServeHTTP(w, r)
	})
}

// RequireRole checks that the user has one of the allowed roles
func (m *AuthMiddleware) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := r.Header.Get("X-User-Role")
			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			httputil.Forbidden(w, "insufficient permissions")
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}
