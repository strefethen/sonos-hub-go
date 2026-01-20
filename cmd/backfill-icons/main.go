// Command backfill-icons updates routines to include service logo URLs in music_content_json.
//
// This is a one-time migration to ensure existing routines have service logos stored
// in the music_content_json field, which is used by iOS to display service icons.
//
// Usage:
//
//	go run ./cmd/backfill-icons
//
// The script will:
// 1. Find all routines with music_sonos_favorite_id set
// 2. Look up the service info from favorites or use column data
// 3. Update music_content_json to include serviceLogoUrl and serviceName
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Service logo URL mappings (relative paths served by Go backend)
var serviceLogo = map[string]string{
	"spotify":       "/v1/assets/service-logos/spotify.png",
	"apple_music":   "/v1/assets/service-logos/apple-music.png",
	"apple music":   "/v1/assets/service-logos/apple-music.png",
	"amazon_music":  "/v1/assets/service-logos/amazon-music.png",
	"amazon music":  "/v1/assets/service-logos/amazon-music.png",
	"tunein":        "/v1/assets/service-logos/tunein.png",
	"pandora":       "/v1/assets/service-logos/pandora.png",
	"deezer":        "/v1/assets/service-logos/deezer.png",
	"tidal":         "/v1/assets/service-logos/tidal.png",
	"soundcloud":    "/v1/assets/service-logos/soundcloud.png",
	"youtube_music": "/v1/assets/service-logos/youtube-music.png",
	"youtube music": "/v1/assets/service-logos/youtube-music.png",
	"audible":       "/v1/assets/service-logos/audible.png",
	"plex":          "/v1/assets/service-logos/plex.png",
	"sonos_radio":   "/v1/assets/service-logos/sonos-radio.png",
	"sonos radio":   "/v1/assets/service-logos/sonos-radio.png",
}

// getServiceLogoURL returns the logo URL for a service name
func getServiceLogoURL(serviceName string) string {
	if serviceName == "" {
		return ""
	}
	key := strings.ToLower(strings.ReplaceAll(serviceName, " ", "_"))
	if url, ok := serviceLogo[key]; ok {
		return url
	}
	// Try without underscore conversion
	key = strings.ToLower(serviceName)
	if url, ok := serviceLogo[key]; ok {
		return url
	}
	return ""
}

type musicContentJSON struct {
	Type           string `json:"type"`
	FavoriteID     string `json:"favoriteId,omitempty"`
	Name           string `json:"name,omitempty"`
	ArtworkURL     string `json:"artworkUrl,omitempty"`
	ServiceLogoURL string `json:"serviceLogoUrl,omitempty"`
	ServiceName    string `json:"serviceName,omitempty"`
}

func main() {
	dbPath := os.Getenv("SQLITE_DB_PATH")
	if dbPath == "" {
		dbPath = "./data/sonos-hub.db"
	}

	log.Printf("Backfill: Opening database at %s", dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Find routines that need backfill:
	// - Has music_sonos_favorite_id set
	// - music_content_json is NULL or doesn't contain serviceLogoUrl
	rows, err := db.Query(`
		SELECT
			routine_id,
			name,
			music_sonos_favorite_id,
			music_sonos_favorite_name,
			music_sonos_favorite_artwork_url,
			music_sonos_favorite_service_logo_url,
			music_sonos_favorite_service_name,
			music_content_json
		FROM routines
		WHERE music_sonos_favorite_id IS NOT NULL
		AND music_sonos_favorite_id != ''
	`)
	if err != nil {
		log.Fatalf("Failed to query routines: %v", err)
	}
	defer rows.Close()

	var updated, skipped int
	for rows.Next() {
		var (
			routineID                       string
			routineName                     string
			sonosFavoriteID                 sql.NullString
			sonosFavoriteName               sql.NullString
			sonosFavoriteArtworkURL         sql.NullString
			sonosFavoriteServiceLogoURL     sql.NullString
			sonosFavoriteServiceName        sql.NullString
			existingContentJSON             sql.NullString
		)

		if err := rows.Scan(
			&routineID,
			&routineName,
			&sonosFavoriteID,
			&sonosFavoriteName,
			&sonosFavoriteArtworkURL,
			&sonosFavoriteServiceLogoURL,
			&sonosFavoriteServiceName,
			&existingContentJSON,
		); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		// Check if already has serviceLogoUrl in JSON
		if existingContentJSON.Valid && existingContentJSON.String != "" {
			if strings.Contains(existingContentJSON.String, "serviceLogoUrl") {
				log.Printf("Skipping %s (%s): already has serviceLogoUrl", routineID, routineName)
				skipped++
				continue
			}
		}

		// Build service logo URL
		var serviceLogoURL string
		var serviceName string

		// First try from column data
		if sonosFavoriteServiceLogoURL.Valid && sonosFavoriteServiceLogoURL.String != "" {
			serviceLogoURL = sonosFavoriteServiceLogoURL.String
		}
		if sonosFavoriteServiceName.Valid && sonosFavoriteServiceName.String != "" {
			serviceName = sonosFavoriteServiceName.String
		}

		// If no logo URL from column, try to derive from service name
		if serviceLogoURL == "" && serviceName != "" {
			serviceLogoURL = getServiceLogoURL(serviceName)
		}

		// Skip if we have no service logo to add
		if serviceLogoURL == "" {
			log.Printf("Skipping %s (%s): no service logo available", routineID, routineName)
			skipped++
			continue
		}

		// Build new music_content_json
		content := musicContentJSON{
			Type:           "sonos_favorite",
			FavoriteID:     sonosFavoriteID.String,
			ServiceLogoURL: serviceLogoURL,
		}

		if sonosFavoriteName.Valid && sonosFavoriteName.String != "" {
			content.Name = sonosFavoriteName.String
		}
		if sonosFavoriteArtworkURL.Valid && sonosFavoriteArtworkURL.String != "" {
			content.ArtworkURL = sonosFavoriteArtworkURL.String
		}
		if serviceName != "" {
			content.ServiceName = serviceName
		}

		contentBytes, err := json.Marshal(content)
		if err != nil {
			log.Printf("Error marshaling JSON for %s: %v", routineID, err)
			continue
		}
		contentJSON := string(contentBytes)

		// Update the routine
		_, err = db.Exec(`
			UPDATE routines
			SET music_content_json = ?,
				updated_at = datetime('now')
			WHERE routine_id = ?
		`, contentJSON, routineID)

		if err != nil {
			log.Printf("Error updating %s: %v", routineID, err)
			continue
		}

		log.Printf("Updated %s (%s) with serviceLogoUrl: %s", routineID, routineName, serviceLogoURL)
		updated++
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	fmt.Printf("\nBackfill complete: %d updated, %d skipped\n", updated, skipped)
}
