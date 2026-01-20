package spotifysearch

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/strefethen/sonos-hub-go/internal/api"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin (Chrome extension)
	},
}

// RegisterRoutes wires Spotify search routes to the router
func RegisterRoutes(router chi.Router, manager *ConnectionManager) {
	// WebSocket endpoint for Chrome extension
	router.HandleFunc("/ws/spotify-search", websocketHandler(manager))

	// Status endpoint
	router.Method(http.MethodGet, "/v1/music/providers/spotify/search/status", api.Handler(statusHandler(manager)))
}

func websocketHandler(manager *ConnectionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade failed - error already written to response
			return
		}

		manager.SetConnection(conn)
	}
}

func statusHandler(manager *ConnectionManager) api.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		status := manager.GetStatus()
		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":           "spotify_search_status",
			"extension":        status.Extension,
			"pending_searches": status.PendingSearches,
		})
	}
}
