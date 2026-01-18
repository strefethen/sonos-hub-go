package auth

import (
	"net/http"
	"strings"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
	"github.com/strefethen/sonos-hub-go/internal/config"
)

var publicRoutes = map[string]struct{}{
	"/v1/auth/pair/start":    {},
	"/v1/auth/pair/complete": {},
	"/v1/auth/refresh":       {},
	"/v1/health":             {},
	"/v1/health/live":        {},
	"/v1/health/ready":       {},
	"/metrics":               {},
}

var publicPrefixes = []string{
	"/v1/health",
	"/v1/assets",
	"/v1/openapi",
}

// Middleware validates JWT tokens for protected routes.
func Middleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if isTestModeRequest(r, cfg) {
				user := User{
					Sub:        "test-device",
					DeviceName: "Test Device",
					Type:       TokenTypeAccess,
				}
				next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				api.WriteError(w, r, apperrors.NewUnauthorizedError("Missing Authorization header"))
				return
			}
			if !strings.HasPrefix(authHeader, "Bearer ") {
				api.WriteError(w, r, apperrors.NewUnauthorizedError("Invalid Authorization header format"))
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				api.WriteError(w, r, apperrors.NewUnauthorizedError("Invalid Authorization header format"))
				return
			}

			payload, err := VerifyToken(cfg, token)
			if err != nil {
				if err == ErrTokenExpired {
					api.WriteError(w, r, apperrors.NewUnauthorizedError("Token has expired", apperrors.ErrorCodeAuthTokenExpired))
					return
				}
				api.WriteError(w, r, apperrors.NewUnauthorizedError("Invalid token", apperrors.ErrorCodeAuthTokenInvalid))
				return
			}

			if payload.Type != TokenTypeAccess {
				api.WriteError(w, r, apperrors.NewUnauthorizedError("Invalid token type", apperrors.ErrorCodeAuthTokenInvalid))
				return
			}

			user := User{
				Sub:        payload.Sub,
				DeviceName: payload.DeviceName,
				Type:       payload.Type,
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
		})
	}
}

func isPublicRoute(path string) bool {
	if _, ok := publicRoutes[path]; ok {
		return true
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isTestModeRequest(r *http.Request, cfg config.Config) bool {
	if !cfg.AllowTestMode {
		return false
	}
	if cfg.NodeEnv != "development" {
		return false
	}
	return r.Header.Get("x-test-mode") == "true"
}
