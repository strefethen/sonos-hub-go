package api

import (
	"log"
	"net/http"

	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// Handler adapts handlers that return errors into http.Handler.
type Handler func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler.
func (handler Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := handler(w, r); err != nil {
		WriteError(w, r, err)
	}
}

// RecovererMiddleware converts panics into 500 responses.
func RecovererMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v", recovered)
				WriteError(w, r, apperrors.NewInternalError("Internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
