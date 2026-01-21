package system

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/music"
)

// Version is the hub version, set at build time or defaulted.
var Version = "1.0.0"

// SchedulerStatusProvider provides scheduler running status.
type SchedulerStatusProvider interface {
	IsRunning() bool
}

// DBPair interface for dependency injection (matches db.DBPair).
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// Service provides system information and dashboard data.
// Uses reader connection only as this service only performs SELECT queries.
type Service struct {
	cfg              config.Config
	logger           *log.Logger
	reader           *sql.DB // Read-only queries
	deviceService    *devices.Service
	musicService     *music.Service
	schedulerStatus  SchedulerStatusProvider
	startTime        time.Time
}

// NewService creates a new system service.
// Accepts a DBPair but only uses the reader (read-only service).
func NewService(cfg config.Config, dbPair DBPair, logger *log.Logger, deviceService *devices.Service, musicService *music.Service, schedulerStatus SchedulerStatusProvider) *Service {
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		cfg:             cfg,
		logger:          logger,
		reader:          dbPair.Reader(),
		deviceService:   deviceService,
		musicService:    musicService,
		schedulerStatus: schedulerStatus,
		startTime:       time.Now(),
	}
}

// SystemInfo holds system information.
// Matches Node.js system.ts SystemInfoResponse interface.
type SystemInfo struct {
	HubVersion       string      `json:"hub_version"`
	Uptime           int64       `json:"uptime_seconds"`
	MemoryUsageMB    float64     `json:"memory_mb"`
	SQLiteConnected  bool        `json:"sqlite_connected"`
	DevicesOnline    int         `json:"devices_online"`
	DevicesTotal     int         `json:"devices_total"`
	SchedulerRunning bool        `json:"scheduler_running"`
	LastDiscovery    *time.Time  `json:"last_discovery,omitempty"`
}

// RoutineSummary is a summary of a routine for dashboard display.
type RoutineSummary struct {
	RoutineID    string     `json:"routine_id"`
	Name         string     `json:"name"`
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
	SceneID      string     `json:"scene_id"`
	Enabled      bool       `json:"enabled"`
	TargetRooms  []string   `json:"target_rooms,omitempty"`
	MusicPreview *string    `json:"music_preview,omitempty"`
	ArtworkURL   *string    `json:"artwork_url,omitempty"`
	TemplateID   *string    `json:"template_id,omitempty"`
}

// AttentionItem represents an item that needs user attention.
type AttentionItem struct {
	Type        string         `json:"type"`
	Severity    string         `json:"severity"`
	Message     string         `json:"message"`
	Details     map[string]any `json:"details,omitempty"`
	ResolveHint string         `json:"resolve_hint,omitempty"`
}

// DashboardData holds data for the dashboard view.
type DashboardData struct {
	NextRoutine      *RoutineSummary  `json:"next_routine,omitempty"`
	UpcomingRoutines []RoutineSummary `json:"upcoming_routines"`
	AttentionItems   []AttentionItem  `json:"attention_items"`
}

// GetSystemInfo returns current system information.
// Mirrors Node.js system.ts GET /v1/system/info handler.
func (s *Service) GetSystemInfo() (*SystemInfo, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Check SQLite connection (Node.js: checkSqliteConnection)
	sqliteConnected := true
	if err := s.reader.Ping(); err != nil {
		sqliteConnected = false
	}

	// Get device stats (Node.js: devicesOnline, devicesTotal)
	devicesOnline := 0
	devicesTotal := 0
	var lastDiscovery *time.Time

	if s.deviceService != nil {
		// Use non-blocking call - don't trigger SSDP discovery
		topology := s.deviceService.GetTopologyIfCached()
		if topology != nil {
			devicesTotal = len(topology.Devices)
			// Node.js: devices.filter(d => d.health !== 'OFFLINE').length
			// Consider OK or DEGRADED as online, only OFFLINE as offline
			for _, device := range topology.Devices {
				if device.Health != devices.DeviceHealthOffline {
					devicesOnline++
				}
			}

			// Get last discovery time
			if !topology.UpdatedAt.IsZero() {
				lastDiscovery = &topology.UpdatedAt
			}
		}
	}

	// Check scheduler status from injected provider
	// Node.js: jobRunner.isRunning()
	schedulerRunning := false
	if s.schedulerStatus != nil {
		schedulerRunning = s.schedulerStatus.IsRunning()
	}

	return &SystemInfo{
		HubVersion:       Version,
		Uptime:           int64(time.Since(s.startTime).Seconds()),
		MemoryUsageMB:    float64(memStats.Alloc) / 1024 / 1024,
		SQLiteConnected:  sqliteConnected,
		DevicesOnline:    devicesOnline,
		DevicesTotal:     devicesTotal,
		SchedulerRunning: schedulerRunning,
		LastDiscovery:    lastDiscovery,
	}, nil
}

// GetDashboardData returns data for the dashboard view.
// Mirrors the Node.js implementation: queries PENDING jobs for today.
func (s *Service) GetDashboardData() (*DashboardData, error) {
	dashboard := &DashboardData{
		UpcomingRoutines: []RoutineSummary{},
		AttentionItems:   []AttentionItem{},
	}

	// Calculate end of today (like Node.js: endOfToday.setHours(23, 59, 59, 999))
	now := time.Now()
	endOfToday := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())

	// Query PENDING jobs for today, joined with routines for details
	// This mirrors Node.js: schedulerService.queryJobs({ status: 'PENDING', from: now, to: endOfToday })
	// Use subquery to deduplicate by routine_id, returning only the earliest job per routine
	nowStr := now.UTC().Format(time.RFC3339)
	endOfTodayStr := endOfToday.UTC().Format(time.RFC3339)
	rows, err := s.reader.Query(`
		SELECT j.job_id, j.routine_id, j.scheduled_for,
		       r.name, r.scene_id, r.speakers_json,
		       r.music_policy_type, r.music_set_id, r.music_sonos_favorite_id,
		       r.music_content_json, r.template_id,
		       r.music_sonos_favorite_name, r.music_sonos_favorite_artwork_url
		FROM jobs j
		INNER JOIN routines r ON j.routine_id = r.routine_id
		WHERE j.status = 'PENDING'
		  AND j.scheduled_for >= ?
		  AND j.scheduled_for <= ?
		  AND j.job_id = (
		      SELECT j2.job_id FROM jobs j2
		      WHERE j2.routine_id = j.routine_id
		        AND j2.status = 'PENDING'
		        AND j2.scheduled_for >= ?
		        AND j2.scheduled_for <= ?
		      ORDER BY j2.scheduled_for ASC
		      LIMIT 1
		  )
		ORDER BY j.scheduled_for ASC
		LIMIT 20
	`, nowStr, endOfTodayStr, nowStr, endOfTodayStr)
	if err != nil {
		s.logger.Printf("Failed to query jobs for dashboard: %v", err)
		return dashboard, nil
	}
	defer rows.Close()

	// Build device ID to room name map for room lookups
	deviceRoomMap := s.buildDeviceRoomMap()

	for rows.Next() {
		var (
			jobID, routineID      string
			scheduledFor          string
			name, sceneID         string
			speakersJSON          sql.NullString
			musicPolicyType       sql.NullString
			musicSetID            sql.NullString
			musicFavoriteID       sql.NullString
			musicContentJSON      sql.NullString
			templateID            sql.NullString
			musicFavoriteName     sql.NullString
			musicFavoriteArtwork  sql.NullString
		)

		if err := rows.Scan(
			&jobID, &routineID, &scheduledFor,
			&name, &sceneID, &speakersJSON,
			&musicPolicyType, &musicSetID, &musicFavoriteID,
			&musicContentJSON, &templateID,
			&musicFavoriteName, &musicFavoriteArtwork,
		); err != nil {
			s.logger.Printf("Failed to scan job row: %v", err)
			continue
		}

		// Parse scheduled_for time
		scheduledTime, err := time.Parse(time.RFC3339, scheduledFor)
		if err != nil {
			// Try alternate format
			scheduledTime, err = time.Parse("2006-01-02 15:04:05", scheduledFor)
			if err != nil {
				s.logger.Printf("Failed to parse scheduled_for for job %s: %v", jobID, err)
				continue
			}
		}

		// Build summary
		summary := RoutineSummary{
			RoutineID: routineID,
			Name:      name,
			SceneID:   sceneID,
			Enabled:   true, // Jobs only exist for enabled routines
			NextRunAt: &scheduledTime,
		}

		// Extract target rooms from scene members (primary) or speakers (fallback)
		// Node.js gets target_rooms from scene members, not routine speakers
		if sceneID != "" {
			summary.TargetRooms = s.extractRoomNamesFromScene(sceneID, deviceRoomMap)
		}
		// Fallback to routine speakers if no scene rooms found
		if len(summary.TargetRooms) == 0 && speakersJSON.Valid && speakersJSON.String != "" {
			summary.TargetRooms = s.extractRoomNamesWithDeviceMap(speakersJSON.String, deviceRoomMap)
		}

		// Build music preview and artwork URL based on music policy
		// Mirrors Node.js logic exactly (see Node.js dashboard.ts lines 100-117)
		if musicPolicyType.Valid {
			switch musicPolicyType.String {
			case "FIXED":
				// Check for direct music content first (musicContent JSON field)
				if musicContentJSON.Valid && musicContentJSON.String != "" {
					var content struct {
						Type       string `json:"type"`
						Title      string `json:"title"`
						Name       string `json:"name"`
						ArtworkURL string `json:"artworkUrl"`
					}
					if err := json.Unmarshal([]byte(musicContentJSON.String), &content); err == nil {
						if content.Type == "sonos_favorite" && content.Name != "" {
							summary.MusicPreview = &content.Name
						} else if content.Type == "direct" && content.Title != "" {
							summary.MusicPreview = &content.Title
						}
						if content.ArtworkURL != "" {
							summary.ArtworkURL = &content.ArtworkURL
						}
					}
				}
				// Fall back to favorite name from routine columns (NOT a separate table)
				// Node.js: routine.musicPolicy.sonosFavoriteName
				if summary.MusicPreview == nil && musicFavoriteName.Valid && musicFavoriteName.String != "" {
					summary.MusicPreview = &musicFavoriteName.String
					if musicFavoriteArtwork.Valid && musicFavoriteArtwork.String != "" {
						summary.ArtworkURL = &musicFavoriteArtwork.String
					}
				}
			case "ROTATION", "SHUFFLE":
				preview := "From music set"
				summary.MusicPreview = &preview
				// Get artwork from music set's first item
				if musicSetID.Valid && musicSetID.String != "" && s.musicService != nil {
					enrichment, err := s.musicService.GetSetEnrichment(musicSetID.String)
					if err == nil && enrichment != nil && enrichment.ArtworkURL != nil {
						summary.ArtworkURL = enrichment.ArtworkURL
					}
				}
			}
		}

		// Set template ID
		if templateID.Valid && templateID.String != "" {
			summary.TemplateID = &templateID.String
		}

		// Fallback: If no artwork yet, try to get template image
		// Priority order: music content artwork > sonos favorite artwork > music set artwork > template image
		if summary.ArtworkURL == nil && templateID.Valid && templateID.String != "" {
			templateImg := s.getTemplateImageURL(templateID.String)
			if templateImg != "" {
				summary.ArtworkURL = &templateImg
			}
		}

		dashboard.UpcomingRoutines = append(dashboard.UpcomingRoutines, summary)
	}

	// Set the next routine if we have any (first item - backwards compat with iOS)
	if len(dashboard.UpcomingRoutines) > 0 {
		dashboard.NextRoutine = &dashboard.UpcomingRoutines[0]
	}

	// Check for attention items
	dashboard.AttentionItems = s.checkAttentionItems()

	return dashboard, nil
}

// buildDeviceRoomMap creates a map of udn -> room_name from the device service.
// NON-BLOCKING: Returns empty map if topology not yet cached.
func (s *Service) buildDeviceRoomMap() map[string]string {
	deviceRoomMap := make(map[string]string)
	if s.deviceService != nil {
		// Use non-blocking call - don't trigger SSDP discovery
		topology := s.deviceService.GetTopologyIfCached()
		if topology != nil {
			for _, device := range topology.Devices {
				deviceRoomMap[device.UDN] = device.RoomName
			}
		}
	}
	return deviceRoomMap
}

// extractRoomNamesFromScene extracts room names from a scene's members using device registry lookup.
func (s *Service) extractRoomNamesFromScene(sceneID string, deviceRoomMap map[string]string) []string {
	var membersJSON sql.NullString
	err := s.reader.QueryRow(`SELECT members FROM scenes WHERE scene_id = ?`, sceneID).Scan(&membersJSON)
	if err != nil || !membersJSON.Valid || membersJSON.String == "" {
		return nil
	}

	var members []struct {
		UDN      string `json:"udn"`
		RoomName string `json:"room_name,omitempty"`
	}

	if err := json.Unmarshal([]byte(membersJSON.String), &members); err != nil {
		return nil
	}

	rooms := make([]string, 0, len(members))
	seen := make(map[string]bool) // Deduplicate room names

	for _, member := range members {
		var roomName string
		// Try room_name from member first
		if member.RoomName != "" {
			roomName = member.RoomName
		} else if deviceRoomMap != nil {
			// Fall back to device registry lookup
			roomName = deviceRoomMap[member.UDN]
		}
		if roomName != "" && !seen[roomName] {
			rooms = append(rooms, roomName)
			seen[roomName] = true
		}
	}

	return rooms
}

// extractRoomNamesWithDeviceMap extracts room names using device registry lookup.
func (s *Service) extractRoomNamesWithDeviceMap(speakersJSON string, deviceRoomMap map[string]string) []string {
	var speakers []struct {
		UDN      string `json:"udn"`
		RoomName string `json:"room_name,omitempty"`
	}

	if err := json.Unmarshal([]byte(speakersJSON), &speakers); err != nil {
		return nil
	}

	rooms := make([]string, 0, len(speakers))
	seen := make(map[string]bool) // Deduplicate room names

	for _, speaker := range speakers {
		var roomName string
		// Try room_name from speakers JSON first
		if speaker.RoomName != "" {
			roomName = speaker.RoomName
		} else if deviceRoomMap != nil {
			// Fall back to device registry lookup
			roomName = deviceRoomMap[speaker.UDN]
		}
		if roomName != "" && !seen[roomName] {
			rooms = append(rooms, roomName)
			seen[roomName] = true
		}
	}

	return rooms
}

// checkAttentionItems checks for items that need user attention.
func (s *Service) checkAttentionItems() []AttentionItem {
	var items []AttentionItem

	// Check for offline devices
	if s.deviceService != nil {
		// Use non-blocking call - don't trigger SSDP discovery
		topology := s.deviceService.GetTopologyIfCached()
		if topology != nil {
			offlineCount := 0
			for _, device := range topology.Devices {
				if device.Health == devices.DeviceHealthOffline {
					offlineCount++
				}
			}
			if offlineCount > 0 {
				items = append(items, AttentionItem{
					Type:     "device_offline",
					Severity: "warning",
					Message:  "Some devices are offline",
					Details: map[string]any{
						"offline_count": offlineCount,
					},
					ResolveHint: "Check device power and network connectivity",
				})
			}
		}
	}

	// Check for failed jobs
	var failedJobCount int
	err := s.reader.QueryRow(`
		SELECT COUNT(*) FROM jobs WHERE status = 'FAILED' AND created_at > datetime('now', '-24 hours')
	`).Scan(&failedJobCount)
	if err == nil && failedJobCount > 0 {
		items = append(items, AttentionItem{
			Type:     "failed_jobs",
			Severity: "error",
			Message:  "Some routines failed to execute",
			Details: map[string]any{
				"failed_count": fmt.Sprintf("%d", failedJobCount),
				"time_window":  "24 hours",
			},
			ResolveHint: "Review job execution history for details",
		})
	}

	// Check SQLite database health
	if err := s.reader.Ping(); err != nil {
		items = append(items, AttentionItem{
			Type:        "database_unhealthy",
			Severity:    "critical",
			Message:     "Database connection is unhealthy",
			ResolveHint: "Check database file permissions and disk space",
		})
	}

	return items
}

// getTemplateImageURL returns the image URL for a template if it has one.
func (s *Service) getTemplateImageURL(templateID string) string {
	var imageName sql.NullString
	err := s.reader.QueryRow(
		`SELECT image_name FROM routine_templates WHERE template_id = ?`,
		templateID,
	).Scan(&imageName)
	if err != nil || !imageName.Valid || imageName.String == "" {
		return ""
	}
	return "/v1/assets/templates/" + imageName.String + ".jpg"
}

