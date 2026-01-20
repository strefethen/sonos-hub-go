// Command backfill-artwork populates artwork_url for set_items from Sonos favorites.
//
// This tool fetches album art URLs from Sonos favorites and updates set_items
// that have NULL artwork_url but reference a valid Sonos favorite ID.
//
// Usage:
//
//	# Ensure .env is loaded and Sonos devices are available
//	set -a && source .env && set +a && go run ./cmd/backfill-artwork
//
//	# Or with explicit device IP
//	DEFAULT_SONOS_IP=192.168.1.10 go run ./cmd/backfill-artwork
//
// The script will:
// 1. Query set_items where artwork_url IS NULL and sonos_favorite_id starts with "FV:2/"
// 2. Fetch favorites from Sonos device via SOAP/UPnP
// 3. Match favorites by ID and update artwork_url
package main

import (
	"context"
	"database/sql"
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

	log.Printf("Backfill Artwork: Opening database at %s", dbPath)
	log.Printf("Backfill Artwork: Using Sonos device at %s", deviceIP)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Query items needing artwork backfill
	rows, err := db.Query(`
		SELECT set_id, sonos_favorite_id
		FROM set_items
		WHERE artwork_url IS NULL
		AND sonos_favorite_id LIKE 'FV:2/%'
	`)
	if err != nil {
		log.Fatalf("Failed to query set_items: %v", err)
	}
	defer rows.Close()

	type itemRef struct {
		SetID           string
		SonosFavoriteID string
	}

	var itemsToUpdate []itemRef
	for rows.Next() {
		var item itemRef
		if err := rows.Scan(&item.SetID, &item.SonosFavoriteID); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		itemsToUpdate = append(itemsToUpdate, item)
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	if len(itemsToUpdate) == 0 {
		fmt.Println("No items need artwork backfill.")
		return
	}

	log.Printf("Found %d items needing artwork backfill", len(itemsToUpdate))

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

	// Update items
	var updated, notFound int
	for _, item := range itemsToUpdate {
		artworkURL, found := artworkMap[item.SonosFavoriteID]
		if !found {
			log.Printf("No artwork found for %s (set %s)", item.SonosFavoriteID, truncateID(item.SetID))
			notFound++
			continue
		}

		// Clean up artwork URL if needed
		artworkURL = cleanArtworkURL(artworkURL)

		_, err := db.Exec(`
			UPDATE set_items
			SET artwork_url = ?
			WHERE set_id = ? AND sonos_favorite_id = ?
		`, artworkURL, item.SetID, item.SonosFavoriteID)

		if err != nil {
			log.Printf("Error updating %s: %v", item.SonosFavoriteID, err)
			continue
		}

		log.Printf("Updated %s with artwork", item.SonosFavoriteID)
		updated++
	}

	fmt.Printf("\nBackfill complete: %d updated, %d not found in Sonos favorites\n", updated, notFound)

	// Show remaining
	var remaining int
	err = db.QueryRow(`SELECT COUNT(*) FROM set_items WHERE artwork_url IS NULL`).Scan(&remaining)
	if err == nil {
		fmt.Printf("Remaining items without artwork: %d\n", remaining)
	}
}

// cleanArtworkURL normalizes the artwork URL
func cleanArtworkURL(url string) string {
	// Some URLs have HTML entities that need decoding
	url = strings.ReplaceAll(url, "&amp;", "&")
	return url
}

// truncateID shortens a UUID for logging
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
