package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "requestID"

// RequestIDMiddleware ensures every request has a request ID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("x-request-id")
		if requestID == "" {
			requestID = uuid.NewString()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		w.Header().Set("x-request-id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request ID for the current request.
func GetRequestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := r.Context().Value(requestIDKey); value != nil {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}
