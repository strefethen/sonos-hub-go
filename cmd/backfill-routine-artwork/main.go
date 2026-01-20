// Command backfill-routine-artwork populates artwork URLs for routines from Sonos favorites.
//
// This tool fetches album art URLs from Sonos favorites and updates routines
// that have music_sonos_favorite_id set but no artwork URL.
//
// Usage:
//
//	# Ensure .env is loaded and Sonos devices are available
//	set -a && source .env && set +a && go run ./cmd/backfill-routine-artwork
//
//	# Or with explicit device IP
//	DEFAULT_SONOS_IP=192.168.1.10 go run ./cmd/backfill-routine-artwork
//
// The script will:
// 1. Query routines where music_sonos_favorite_artwork_url IS NULL and music_sonos_favorite_id is set
// 2. Fetch favorites from Sonos device via SOAP/UPnP
// 3. Match favorites by ID and update both the column and music_content_json
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

func main() {
	dbPath := os.Getenv("SQLITE_DB_PATH")
	if dbPath == "" {
		dbPath = "./data/sonos-hub.db"
	}

	deviceIP := os.Getenv("DEFAULT_SONOS_IP")
	if deviceIP == "" {
		deviceIP = "192.168.1.10"
	}

	log.Printf("Backfill Routine Artwork: Opening database at %s", dbPath)
	log.Printf("Backfill Routine Artwork: Using Sonos device at %s", deviceIP)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Query routines needing artwork backfill
	rows, err := db.Query(`
		SELECT routine_id, name, music_sonos_favorite_id, music_content_json
		FROM routines
		WHERE music_sonos_favorite_id IS NOT NULL
		AND music_sonos_favorite_id != ''
		AND music_sonos_favorite_id LIKE 'FV:2/%'
		AND (music_sonos_favorite_artwork_url IS NULL OR music_sonos_favorite_artwork_url = '')
	`)
	if err != nil {
		log.Fatalf("Failed to query routines: %v", err)
	}
	defer rows.Close()

	type routineRef struct {
		RoutineID       string
		Name            string
		SonosFavoriteID string
		ContentJSON     sql.NullString
	}

	var routinesToUpdate []routineRef
	for rows.Next() {
		var r routineRef
		if err := rows.Scan(&r.RoutineID, &r.Name, &r.SonosFavoriteID, &r.ContentJSON); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		routinesToUpdate = append(routinesToUpdate, r)
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	if len(routinesToUpdate) == 0 {
		fmt.Println("No routines need artwork backfill.")
		return
	}

	log.Printf("Found %d routines needing artwork backfill", len(routinesToUpdate))

	// Fetch favorites from Sonos
	client := soap.NewClient(10 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Fetching favorites from Sonos device...")
	result, err := client.Browse(ctx, deviceIP, "FV:2", "BrowseDirectChildren", "*", 0, 200)
	if err != nil {
		log.Fatalf("Failed to fetch favorites: %v", err)
	}

	log.Printf("Fetched %d favorites from Sonos", len(result.Items))

	// Build map of favorite ID -> artwork URL
	artworkMap := make(map[string]string)
	for _, fav := range result.Items {
		if fav.AlbumArtURI != "" {
			artworkMap[fav.ID] = fav.AlbumArtURI
		}
	}

	log.Printf("Found %d favorites with artwork", len(artworkMap))

	// Update routines
	var updated, notFound int
	for _, routine := range routinesToUpdate {
		artworkURL, found := artworkMap[routine.SonosFavoriteID]
		if !found {
			log.Printf("No artwork found for %s (%s)", routine.SonosFavoriteID, routine.Name)
			notFound++
			continue
		}

		// Clean up artwork URL
		artworkURL = cleanArtworkURL(artworkURL)

		// Update music_content_json if it exists
		var newContentJSON sql.NullString
		if routine.ContentJSON.Valid && routine.ContentJSON.String != "" {
			var content map[string]any
			if err := json.Unmarshal([]byte(routine.ContentJSON.String), &content); err == nil {
				content["artworkUrl"] = artworkURL
				if jsonBytes, err := json.Marshal(content); err == nil {
					newContentJSON = sql.NullString{String: string(jsonBytes), Valid: true}
				}
			}
		}

		// Update both the column and JSON
		var execErr error
		if newContentJSON.Valid {
			_, execErr = db.Exec(`
				UPDATE routines
				SET music_sonos_favorite_artwork_url = ?,
					music_content_json = ?,
					updated_at = datetime('now')
				WHERE routine_id = ?
			`, artworkURL, newContentJSON.String, routine.RoutineID)
		} else {
			_, execErr = db.Exec(`
				UPDATE routines
				SET music_sonos_favorite_artwork_url = ?,
					updated_at = datetime('now')
				WHERE routine_id = ?
			`, artworkURL, routine.RoutineID)
		}

		if execErr != nil {
			log.Printf("Error updating %s: %v", routine.Name, execErr)
			continue
		}

		log.Printf("Updated %s with artwork: %s", routine.Name, truncateURL(artworkURL))
		updated++
	}

	fmt.Printf("\nBackfill complete: %d updated, %d not found in Sonos favorites\n", updated, notFound)

	// Show remaining
	var remaining int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM routines
		WHERE music_sonos_favorite_id IS NOT NULL
		AND music_sonos_favorite_id != ''
		AND (music_sonos_favorite_artwork_url IS NULL OR music_sonos_favorite_artwork_url = '')
	`).Scan(&remaining)
	if err == nil && remaining > 0 {
		fmt.Printf("Remaining routines without artwork: %d\n", remaining)
	}
}

// cleanArtworkURL normalizes the artwork URL
func cleanArtworkURL(url string) string {
	// Some URLs have HTML entities that need decoding
	url = strings.ReplaceAll(url, "&amp;", "&")
	return url
}

// truncateURL shortens a URL for logging
func truncateURL(url string) string {
	if len(url) > 60 {
		return url[:60] + "..."
	}
	return url
}
