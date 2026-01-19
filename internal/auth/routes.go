package auth

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
	"github.com/strefethen/sonos-hub-go/internal/config"
)

// RegisterRoutes wires auth routes to the router.
func RegisterRoutes(router chi.Router, store *PairingStore, cfg config.Config) {
	router.Method(http.MethodPost, "/v1/auth/pair/start", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		requestID := api.GetRequestID(r)
		store.CleanupExpired()

		pairCode, err := store.Create(requestID)
		if err != nil {
			return apperrors.NewInternalError("Failed to generate pairing code")
		}

		log.Printf("Pairing code generated - enter this on your device: %s", pairCode)

		return api.WriteAction(w, http.StatusOK, map[string]any{
			"object":       "pairing_start",
			"pairing_hint": "Enter pairing code on your device. Code: " + pairCode,
		})
	}))

	router.Method(http.MethodPost, "/v1/auth/pair/complete", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		var body struct {
			PairCode   string `json:"pair_code"`
			DeviceName string `json:"device_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return apperrors.NewValidationError("pair_code is required", nil)
		}
		if body.PairCode == "" {
			return apperrors.NewValidationError("pair_code is required", nil)
		}
		if body.DeviceName == "" {
			return apperrors.NewValidationError("device_name is required", nil)
		}

		_, ok, expired := store.Lookup(body.PairCode)
		if !ok {
			return apperrors.NewUnauthorizedError("Invalid or expired pairing code")
		}
		if expired {
			store.Consume(body.PairCode)
			return apperrors.NewUnauthorizedError("Pairing code has expired")
		}

		store.Consume(body.PairCode)

		deviceID := uuid.NewString()
		tokens, err := GenerateTokenPair(cfg, TokenPayload{
			Sub:        deviceID,
			DeviceName: body.DeviceName,
		})
		if err != nil {
			return apperrors.NewInternalError("Failed to generate token pair")
		}

		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":         "token_pair",
			"access_token":   tokens.AccessToken,
			"refresh_token":  tokens.RefreshToken,
			"expires_in_sec": tokens.ExpiresInSec,
		})
	}))

	router.Method(http.MethodPost, "/v1/auth/refresh", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return apperrors.NewValidationError("refresh_token is required", nil)
		}
		if body.RefreshToken == "" {
			return apperrors.NewValidationError("refresh_token is required", nil)
		}

		accessToken, expiresIn, err := RefreshAccessToken(cfg, body.RefreshToken)
		if err != nil {
			switch err {
			case ErrTokenExpired:
				return apperrors.NewUnauthorizedError("Refresh token has expired")
			case ErrTokenType:
				return apperrors.NewUnauthorizedError("Invalid token: expected refresh token")
			default:
				return apperrors.NewUnauthorizedError("Invalid refresh token")
			}
		}

		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":         "token_refresh",
			"access_token":   accessToken,
			"expires_in_sec": expiresIn,
		})
	}))
}
