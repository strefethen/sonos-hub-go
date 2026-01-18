package sonoscloud

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// RegisterRoutes wires Sonos Cloud routes to the router.
func RegisterRoutes(router chi.Router, client *Client) {
	// Auth endpoints
	router.Method(http.MethodGet, "/v1/sonos-cloud/auth/start", api.Handler(authStart(client)))
	router.Method(http.MethodGet, "/v1/sonos-cloud/auth/callback", api.Handler(authCallback(client)))
	router.Method(http.MethodGet, "/v1/sonos-cloud/auth/status", api.Handler(authStatus(client)))
	router.Method(http.MethodPost, "/v1/sonos-cloud/auth/refresh", api.Handler(authRefresh(client)))
	router.Method(http.MethodPost, "/v1/sonos-cloud/auth/disconnect", api.Handler(authDisconnect(client)))

	// API endpoints
	router.Method(http.MethodGet, "/v1/sonos-cloud/households", api.Handler(getHouseholds(client)))
	router.Method(http.MethodGet, "/v1/sonos-cloud/groups", api.Handler(getGroups(client)))
	router.Method(http.MethodGet, "/v1/sonos-cloud/players", api.Handler(getPlayers(client)))
	router.Method(http.MethodPost, "/v1/sonos-cloud/players/{playerId}/audioClip", api.Handler(playAudioClip(client)))
}

func authStart(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		authResp, err := client.GetAuthURL()
		if err != nil {
			return apperrors.NewInternalError("Failed to generate auth URL")
		}

		return api.SingleResponse(w, r, http.StatusOK, "auth", authResp)
	}
}

func authCallback(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errorParam := r.URL.Query().Get("error")

		if errorParam != "" {
			errorDesc := r.URL.Query().Get("error_description")
			return apperrors.NewAppError(
				apperrors.ErrorCodeServiceAuthFailed,
				"OAuth authorization failed: "+errorDesc,
				400,
				map[string]any{
					"error":             errorParam,
					"error_description": errorDesc,
				},
				nil,
			)
		}

		if code == "" {
			return apperrors.NewValidationError("code is required", nil)
		}
		if state == "" {
			return apperrors.NewValidationError("state is required", nil)
		}

		token, err := client.ExchangeCode(code, state)
		if err != nil {
			if err.Error() == "invalid or expired state" {
				return apperrors.NewValidationError("Invalid or expired state parameter", map[string]any{
					"hint": "The authorization flow may have expired. Please try again.",
				})
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				return apperrors.NewAppError(
					apperrors.ErrorCodeServiceAuthFailed,
					"Token exchange failed: "+apiErr.Error(),
					apiErr.HTTPStatus,
					nil,
					nil,
				)
			}
			return apperrors.NewInternalError("Failed to exchange authorization code")
		}

		return api.SingleResponse(w, r, http.StatusOK, "connection", map[string]any{
			"connected":    true,
			"expires_at":   token.ExpiresAt.UTC().Format(time.RFC3339),
			"connected_at": token.CreatedAt.UTC().Format(time.RFC3339),
			"scope":        token.Scope,
		})
	}
}

func authStatus(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		status, err := client.GetStatus()
		if err != nil {
			return apperrors.NewInternalError("Failed to get connection status")
		}

		data := map[string]any{
			"connected": status.Connected,
		}
		if status.ExpiresAt != nil {
			data["expires_at"] = status.ExpiresAt.UTC().Format(time.RFC3339)
		}
		if status.ConnectedAt != nil {
			data["connected_at"] = status.ConnectedAt.UTC().Format(time.RFC3339)
		}
		if status.Scope != "" {
			data["scope"] = status.Scope
		}

		return api.SingleResponse(w, r, http.StatusOK, "status", data)
	}
}

func authRefresh(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		token, err := client.RefreshToken()
		if err != nil {
			if err.Error() == "no token to refresh" {
				return apperrors.NewAppError(
					apperrors.ErrorCodeServiceNotBootstrapped,
					"Not connected to Sonos Cloud",
					400,
					nil,
					&apperrors.Remediation{
						Action:   "authorize",
						Endpoint: "/v1/sonos-cloud/auth/start",
					},
				)
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				return apperrors.NewAppError(
					apperrors.ErrorCodeServiceAuthFailed,
					"Token refresh failed: "+apiErr.Error(),
					apiErr.HTTPStatus,
					nil,
					nil,
				)
			}
			return apperrors.NewInternalError("Failed to refresh token")
		}

		return api.ActionResponse(w, r, http.StatusOK, map[string]any{
			"refreshed":  true,
			"expires_at": token.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func authDisconnect(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		err := client.Disconnect()
		if err != nil {
			// Not found is okay - already disconnected
			return apperrors.NewInternalError("Failed to disconnect")
		}

		return api.ActionResponse(w, r, http.StatusOK, map[string]any{
			"disconnected": true,
		})
	}
}

func getHouseholds(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		households, err := client.GetHouseholds()
		if err != nil {
			return handleAPIError(err)
		}

		return api.ListResponse(w, r, http.StatusOK, "households", households, nil)
	}
}

func getGroups(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		householdID := r.URL.Query().Get("householdId")
		if householdID == "" {
			return apperrors.NewValidationError("householdId query parameter is required", nil)
		}

		groupsResp, err := client.GetGroups(householdID)
		if err != nil {
			return handleAPIError(err)
		}

		return api.SingleResponse(w, r, http.StatusOK, "groups", map[string]any{
			"items":   groupsResp.Groups,
			"players": groupsResp.Players,
		})
	}
}

func getPlayers(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		householdID := r.URL.Query().Get("householdId")
		if householdID == "" {
			return apperrors.NewValidationError("householdId query parameter is required", nil)
		}

		players, err := client.GetPlayers(householdID)
		if err != nil {
			return handleAPIError(err)
		}

		return api.ListResponse(w, r, http.StatusOK, "players", players, nil)
	}
}

func playAudioClip(client *Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		playerID := chi.URLParam(r, "playerId")
		if playerID == "" {
			return apperrors.NewValidationError("playerId is required", nil)
		}

		var clipReq AudioClipRequest
		if err := json.NewDecoder(r.Body).Decode(&clipReq); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if clipReq.AppID == "" {
			return apperrors.NewValidationError("appId is required", nil)
		}

		clipResp, err := client.PlayAudioClip(playerID, &clipReq)
		if err != nil {
			return handleAPIError(err)
		}

		return api.ActionResponse(w, r, http.StatusOK, clipResp)
	}
}

func handleAPIError(err error) error {
	if err.Error() == "not connected to Sonos Cloud" {
		return apperrors.NewAppError(
			apperrors.ErrorCodeServiceNotBootstrapped,
			"Not connected to Sonos Cloud",
			401,
			nil,
			&apperrors.Remediation{
				Action:   "authorize",
				Endpoint: "/v1/sonos-cloud/auth/start",
			},
		)
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		statusCode := apiErr.HTTPStatus
		if statusCode == 0 {
			statusCode = 500
		}
		return apperrors.NewAppError(
			apperrors.ErrorCodeSonosRejected,
			"Sonos Cloud API error: "+apiErr.Error(),
			statusCode,
			map[string]any{
				"error_code": apiErr.ErrorCode,
			},
			nil,
		)
	}

	return apperrors.NewInternalError("Sonos Cloud API request failed")
}
