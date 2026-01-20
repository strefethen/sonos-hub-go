package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/music"
	"github.com/strefethen/sonos-hub-go/internal/scene"
	"github.com/strefethen/sonos-hub-go/internal/sonos"
)

// RoutineExecutor handles music resolution before scene execution
type RoutineExecutor interface {
	ExecuteRoutine(routine *Routine, idempotencyKey *string) (*scene.SceneExecution, error)
}

// RoutineExecutorAdapter implements RoutineExecutor
// It resolves music content from routines and delegates to scene execution
type RoutineExecutorAdapter struct {
	sceneExecutor   SceneExecutor
	musicService    *music.Service
	contentResolver *sonos.ContentResolver
	deviceService   *devices.Service
	logger          *log.Logger
	timeout         time.Duration
}

// NewRoutineExecutorAdapter creates a new RoutineExecutorAdapter
func NewRoutineExecutorAdapter(
	sceneExecutor SceneExecutor,
	musicService *music.Service,
	contentResolver *sonos.ContentResolver,
	deviceService *devices.Service,
	timeout time.Duration,
) *RoutineExecutorAdapter {
	return &RoutineExecutorAdapter{
		sceneExecutor:   sceneExecutor,
		musicService:    musicService,
		contentResolver: contentResolver,
		deviceService:   deviceService,
		logger:          log.Default(),
		timeout:         timeout,
	}
}

// ExecuteRoutine resolves music content and executes the scene
func (a *RoutineExecutorAdapter) ExecuteRoutine(routine *Routine, idempotencyKey *string) (*scene.SceneExecution, error) {
	options := scene.ExecuteOptions{}

	// Set TV policy from routine if configured
	if routine.ArcTVPolicy != nil {
		options.TVPolicy = scene.TVPolicy(*routine.ArcTVPolicy)
	}

	// Resolve music content based on policy type
	musicContent, err := a.resolveMusicContent(routine)
	if err != nil {
		a.logger.Printf("Warning: failed to resolve music for routine %s: %v", routine.RoutineID, err)
		// Continue - scene still executes for grouping/volume
	} else if musicContent != nil {
		options.MusicContent = musicContent
		options.QueueMode = scene.QueueModeReplaceAndPlay
	}

	return a.sceneExecutor.ExecuteScene(routine.SceneID, idempotencyKey, options)
}

// resolveMusicContent dispatches based on MusicPolicyType
func (a *RoutineExecutorAdapter) resolveMusicContent(routine *Routine) (*scene.MusicContent, error) {
	switch routine.MusicPolicyType {
	case MusicPolicyTypeFixed:
		return a.resolveFixedContent(routine)
	case MusicPolicyTypeRotation, MusicPolicyTypeShuffle:
		return a.resolveSetContent(routine)
	default:
		// Check if there's content even without explicit policy
		if routine.MusicContentJSON != nil && *routine.MusicContentJSON != "" {
			return a.resolveDirectContentFromJSON(*routine.MusicContentJSON, routine)
		}
		if routine.MusicSonosFavoriteID != nil && *routine.MusicSonosFavoriteID != "" {
			return a.resolveFavorite(*routine.MusicSonosFavoriteID, routine)
		}
		return nil, nil // No music configured
	}
}

// resolveFixedContent resolves FIXED policy content
// Priority: DirectContent (MusicContentJSON) > Sonos Favorite
func (a *RoutineExecutorAdapter) resolveFixedContent(routine *Routine) (*scene.MusicContent, error) {
	// Try DirectContent first (preferred path - bypasses 70-favorite limit)
	if routine.MusicContentJSON != nil && *routine.MusicContentJSON != "" {
		return a.resolveDirectContentFromJSON(*routine.MusicContentJSON, routine)
	}

	// Fallback to Sonos Favorite
	if routine.MusicSonosFavoriteID != nil && *routine.MusicSonosFavoriteID != "" {
		return a.resolveFavorite(*routine.MusicSonosFavoriteID, routine)
	}

	return nil, nil
}

// resolveSetContent selects an item from music set and resolves it
func (a *RoutineExecutorAdapter) resolveSetContent(routine *Routine) (*scene.MusicContent, error) {
	if routine.MusicSetID == nil || *routine.MusicSetID == "" {
		return nil, nil
	}

	// Select item from set
	input := music.SelectItemInput{
		NoRepeatWindowMinutes: routine.MusicNoRepeatWindowMinutes,
	}
	result, err := a.musicService.SelectItem(*routine.MusicSetID, input)
	if err != nil {
		return nil, fmt.Errorf("select item from set %s: %w", *routine.MusicSetID, err)
	}
	if result == nil || result.Item == nil {
		return nil, fmt.Errorf("no item selected from set %s", *routine.MusicSetID)
	}

	item := result.Item
	a.logger.Printf("Selected item from set %s: favoriteID=%s position=%d",
		*routine.MusicSetID, item.SonosFavoriteID, item.Position)

	// Try DirectContent first (check ContentJSON on the item)
	if item.ContentJSON != nil && *item.ContentJSON != "" {
		content, err := a.resolveDirectContentFromJSON(*item.ContentJSON, routine)
		if err == nil && content != nil {
			// Record play history
			routineID := routine.RoutineID
			if err := a.musicService.RecordPlay(item.SonosFavoriteID, routine.MusicSetID, &routineID); err != nil {
				a.logger.Printf("Warning: failed to record play history: %v", err)
			}
			return content, nil
		}
		a.logger.Printf("DirectContent resolution failed, trying favorite: %v", err)
	}

	// Fallback to Sonos Favorite if available
	if item.SonosFavoriteID != "" {
		content, err := a.resolveFavorite(item.SonosFavoriteID, routine)
		if err == nil && content != nil {
			// Record play history
			routineID := routine.RoutineID
			if err := a.musicService.RecordPlay(item.SonosFavoriteID, routine.MusicSetID, &routineID); err != nil {
				a.logger.Printf("Warning: failed to record play history: %v", err)
			}
			return content, nil
		}
		return nil, fmt.Errorf("resolve favorite %s: %w", item.SonosFavoriteID, err)
	}

	return nil, fmt.Errorf("set item has no resolvable content")
}

// directContent represents the JSON structure stored in MusicContentJSON
type directContent struct {
	Type        string  `json:"type"`
	Service     *string `json:"service"`
	ContentType *string `json:"content_type"`
	ContentID   *string `json:"content_id"`
	Title       *string `json:"title"`
}

// resolveDirectContentFromJSON parses JSON and resolves DirectContent
func (a *RoutineExecutorAdapter) resolveDirectContentFromJSON(contentJSON string, routine *Routine) (*scene.MusicContent, error) {
	// Parse the stored JSON
	var content directContent
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return nil, fmt.Errorf("parse content JSON: %w", err)
	}

	// Validate required fields for direct content
	if content.Service == nil || *content.Service == "" {
		return nil, fmt.Errorf("direct content missing service")
	}
	if content.ContentID == nil || *content.ContentID == "" {
		return nil, fmt.Errorf("direct content missing content_id")
	}

	service := *content.Service
	contentType := ""
	if content.ContentType != nil {
		contentType = *content.ContentType
	}
	contentID := *content.ContentID
	title := ""
	if content.Title != nil {
		title = *content.Title
	}

	deviceIP, err := a.getDeviceIP(routine)
	if err != nil {
		return nil, fmt.Errorf("get device IP: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	a.logger.Printf("Resolving DirectContent: service=%s type=%s id=%s title=%s deviceIP=%s",
		service, contentType, contentID, title, deviceIP)

	// Use ContentResolver to build URI and metadata
	playable, err := a.contentResolver.ResolveDirectContent(ctx, service, contentType, contentID, title, deviceIP)
	if err != nil {
		return nil, fmt.Errorf("resolve direct content: %w", err)
	}

	return &scene.MusicContent{
		Type:      "direct",
		URI:       playable.URI,
		Metadata:  playable.Metadata,
		UsesQueue: playable.UsesQueue,
	}, nil
}

// resolveFavorite resolves a Sonos Favorite ID to playable content
func (a *RoutineExecutorAdapter) resolveFavorite(favoriteID string, routine *Routine) (*scene.MusicContent, error) {
	deviceIP, err := a.getDeviceIP(routine)
	if err != nil {
		return nil, fmt.Errorf("get device IP: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	playable, err := a.contentResolver.ResolveFavorite(ctx, favoriteID, deviceIP)
	if err != nil {
		return nil, fmt.Errorf("resolve favorite %s: %w", favoriteID, err)
	}

	return &scene.MusicContent{
		Type:      "sonos_favorite",
		URI:       playable.URI,
		Metadata:  playable.Metadata,
		UsesQueue: playable.UsesQueue,
	}, nil
}

// getDeviceIP gets a device IP for content resolution
func (a *RoutineExecutorAdapter) getDeviceIP(routine *Routine) (string, error) {
	// Try routine's speakers first
	if len(routine.SpeakersJSON) > 0 {
		udn := routine.SpeakersJSON[0].UDN
		ip, err := a.deviceService.ResolveDeviceIP(udn)
		if err != nil {
			a.logger.Printf("Warning: error resolving IP for speaker %s: %v", udn, err)
		} else if ip != "" {
			return ip, nil
		} else {
			a.logger.Printf("Warning: speaker %s not found in topology, using fallback", udn)
		}
	}

	// Fallback to any discovered device
	topology, err := a.deviceService.GetTopology()
	if err == nil && len(topology.Devices) > 0 {
		a.logger.Printf("Using fallback device for content resolution: %s (%s)",
			topology.Devices[0].RoomName, topology.Devices[0].IP)
		return topology.Devices[0].IP, nil
	}

	return "", fmt.Errorf("no device available for content resolution")
}
