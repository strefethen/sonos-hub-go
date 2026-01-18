package sonos

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// PlayRequest represents a request to resume playback
type PlayRequest struct {
	CoordinatorDeviceID *string `json:"coordinator_device_id"`
	IP                  *string `json:"ip"`
}

// PlayResponse represents the response from a play request
type PlayResponse struct {
	CoordinatorDeviceID *string   `json:"coordinator_device_id,omitempty"`
	IP                  string    `json:"ip"`
	StartedAt           time.Time `json:"started_at"`
}

// PlayFavoriteRequest represents a request to play a Sonos favorite
type PlayFavoriteRequest struct {
	DeviceID      *string `json:"device_id"`
	IP            *string `json:"ip"`
	FavoriteID    string  `json:"favorite_id"`
	GroupBehavior *string `json:"group_behavior"` // UNGROUP_AND_PLAY, AUTO_REDIRECT
}

// PlayFavoriteResponse represents the response from playing a favorite
type PlayFavoriteResponse struct {
	DeviceID       string    `json:"device_id"`
	FavoriteID     string    `json:"favorite_id"`
	FavoriteTitle  string    `json:"favorite_title"`
	ContentType    string    `json:"content_type"`
	ServiceLogoURL string    `json:"service_logo_url,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	WasUngrouped   bool      `json:"was_ungrouped"`
}

// PlayContentRequest represents a request to play direct content
type PlayContentRequest struct {
	DeviceID      *string      `json:"device_id"`
	IP            *string      `json:"ip"`
	Content       MusicContent `json:"content"`
	QueueMode     *string      `json:"queue_mode"`     // REPLACE_AND_PLAY, PLAY_NEXT, ADD_TO_END, QUEUE_ONLY
	GroupBehavior *string      `json:"group_behavior"` // UNGROUP_AND_PLAY, AUTO_REDIRECT
}

// PlayContentResponse represents the response from playing content
type PlayContentResponse struct {
	DeviceID      string    `json:"device_id"`
	QueueMode     string    `json:"queue_mode"`
	GroupBehavior string    `json:"group_behavior"`
	WasUngrouped  bool      `json:"was_ungrouped"`
	CoordinatorID *string   `json:"coordinator_id,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	Service       string    `json:"service,omitempty"`
	ContentType   string    `json:"content_type,omitempty"`
	ContentID     string    `json:"content_id,omitempty"`
	Title         string    `json:"title,omitempty"`
}

// ValidateContentRequest represents a request to validate content
type ValidateContentRequest struct {
	Content  MusicContent `json:"content"`
	DeviceID *string      `json:"device_id"`
}

// Queue mode constants
const (
	QueueModeReplaceAndPlay = "REPLACE_AND_PLAY"
	QueueModePlayNext       = "PLAY_NEXT"
	QueueModeAddToEnd       = "ADD_TO_END"
	QueueModeQueueOnly      = "QUEUE_ONLY"
)

// Group behavior constants
const (
	GroupBehaviorUngroupAndPlay = "UNGROUP_AND_PLAY"
	GroupBehaviorAutoRedirect   = "AUTO_REDIRECT"
)

// PlayService handles music playback operations
type PlayService struct {
	soapClient      *soap.Client
	deviceService   *devices.Service
	contentResolver *ContentResolver
	timeout         time.Duration
	logger          *log.Logger
}

// NewPlayService creates a new PlayService
func NewPlayService(soapClient *soap.Client, deviceService *devices.Service, timeout time.Duration, logger *log.Logger) *PlayService {
	return &PlayService{
		soapClient:      soapClient,
		deviceService:   deviceService,
		contentResolver: NewContentResolver(soapClient, deviceService, timeout, logger),
		timeout:         timeout,
		logger:          logger,
	}
}

// resolveDeviceIP resolves the IP address for a device
func (s *PlayService) resolveDeviceIP(deviceID *string, ip *string) (string, string, error) {
	if ip != nil && *ip != "" {
		resolvedDeviceID := ""
		if deviceID != nil {
			resolvedDeviceID = *deviceID
		}
		return *ip, resolvedDeviceID, nil
	}

	if deviceID == nil || *deviceID == "" {
		return "", "", fmt.Errorf("device_id or ip is required")
	}

	if s.deviceService == nil {
		return "", "", fmt.Errorf("device service not available")
	}

	resolvedIP, err := s.deviceService.ResolveDeviceIP(*deviceID)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve device: %w", err)
	}

	return resolvedIP, *deviceID, nil
}

// Play resumes playback on a device
func (s *PlayService) Play(ctx context.Context, req PlayRequest) (*PlayResponse, error) {
	deviceIP, deviceID, err := s.resolveDeviceIP(req.CoordinatorDeviceID, req.IP)
	if err != nil {
		return nil, err
	}

	if err := s.soapClient.Play(ctx, deviceIP); err != nil {
		return nil, fmt.Errorf("failed to start playback: %w", err)
	}

	response := &PlayResponse{
		IP:        deviceIP,
		StartedAt: time.Now().UTC(),
	}
	if deviceID != "" {
		response.CoordinatorDeviceID = &deviceID
	}

	return response, nil
}

// PlayFavorite plays a Sonos favorite
func (s *PlayService) PlayFavorite(ctx context.Context, req PlayFavoriteRequest) (*PlayFavoriteResponse, error) {
	// Resolve device IP
	deviceIP, deviceID, err := s.resolveDeviceIP(req.DeviceID, req.IP)
	if err != nil {
		return nil, err
	}

	// Handle group behavior - ungroup if requested
	wasUngrouped := false
	if req.GroupBehavior != nil && *req.GroupBehavior == GroupBehaviorUngroupAndPlay {
		if err := s.soapClient.BecomeCoordinatorOfStandaloneGroup(ctx, deviceIP); err != nil {
			s.logf("Warning: failed to ungroup device: %v", err)
		} else {
			wasUngrouped = true
		}
	}

	// Resolve favorite using content resolver
	playable, err := s.contentResolver.ResolveFavorite(ctx, req.FavoriteID, deviceIP)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve favorite: %w", err)
	}

	// Play the content
	if playable.UsesQueue {
		// Queue-based playback: clear queue, add to queue, set transport, play
		if err := s.soapClient.RemoveAllTracksFromQueue(ctx, deviceIP); err != nil {
			return nil, fmt.Errorf("failed to clear queue: %w", err)
		}

		_, err := s.soapClient.AddURIToQueue(ctx, deviceIP, playable.URI, playable.Metadata, 0, false)
		if err != nil {
			return nil, fmt.Errorf("failed to add to queue: %w", err)
		}

		// Set transport URI to queue
		queueURI := fmt.Sprintf("x-rincon-queue:%s#0", s.getDeviceUUID(ctx, deviceIP))
		if err := s.soapClient.SetAVTransportURI(ctx, deviceIP, queueURI, ""); err != nil {
			return nil, fmt.Errorf("failed to set transport URI: %w", err)
		}
	} else {
		// Direct playback: set transport URI directly
		if err := s.soapClient.SetAVTransportURI(ctx, deviceIP, playable.URI, playable.Metadata); err != nil {
			return nil, fmt.Errorf("failed to set transport URI: %w", err)
		}
	}

	// Start playback
	if err := s.soapClient.Play(ctx, deviceIP); err != nil {
		return nil, fmt.Errorf("failed to start playback: %w", err)
	}

	// Build response
	response := &PlayFavoriteResponse{
		DeviceID:      deviceID,
		FavoriteID:    req.FavoriteID,
		FavoriteTitle: playable.Title,
		ContentType:   playable.ContentType,
		StartedAt:     time.Now().UTC(),
		WasUngrouped:  wasUngrouped,
	}

	if playable.Service != "" {
		response.ServiceLogoURL = GetServiceLogoFromName(playable.Service)
	}

	return response, nil
}

// PlayContent plays direct content from a streaming service
func (s *PlayService) PlayContent(ctx context.Context, req PlayContentRequest) (*PlayContentResponse, error) {
	// Get queue mode with default
	queueMode := QueueModeReplaceAndPlay
	if req.QueueMode != nil && *req.QueueMode != "" {
		queueMode = *req.QueueMode
	}

	// Validate queue mode
	validQueueModes := map[string]bool{
		QueueModeReplaceAndPlay: true,
		QueueModePlayNext:       true,
		QueueModeAddToEnd:       true,
		QueueModeQueueOnly:      true,
	}
	if !validQueueModes[queueMode] {
		return nil, fmt.Errorf("invalid queue_mode: %s", queueMode)
	}

	// Stations can only use REPLACE_AND_PLAY
	if req.Content.ContentType != nil && (*req.Content.ContentType == "station" || *req.Content.ContentType == "radio") {
		if queueMode != QueueModeReplaceAndPlay {
			return nil, fmt.Errorf("stations can only use REPLACE_AND_PLAY queue mode")
		}
	}

	// Resolve device IP
	deviceIP, deviceID, err := s.resolveDeviceIP(req.DeviceID, req.IP)
	if err != nil {
		return nil, err
	}

	// Get group behavior with default
	groupBehavior := GroupBehaviorAutoRedirect
	if req.GroupBehavior != nil && *req.GroupBehavior != "" {
		groupBehavior = *req.GroupBehavior
	}

	// Handle group behavior
	wasUngrouped := false
	if groupBehavior == GroupBehaviorUngroupAndPlay {
		if err := s.soapClient.BecomeCoordinatorOfStandaloneGroup(ctx, deviceIP); err != nil {
			s.logf("Warning: failed to ungroup device: %v", err)
		} else {
			wasUngrouped = true
		}
	}

	// Resolve content using content resolver
	playable, err := s.contentResolver.ResolveContent(ctx, req.Content, deviceIP)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve content: %w", err)
	}

	// Execute based on queue mode
	switch queueMode {
	case QueueModeReplaceAndPlay:
		if playable.UsesQueue {
			// Clear queue, add content, set transport to queue, play
			if err := s.soapClient.RemoveAllTracksFromQueue(ctx, deviceIP); err != nil {
				return nil, fmt.Errorf("failed to clear queue: %w", err)
			}

			_, err := s.soapClient.AddURIToQueue(ctx, deviceIP, playable.URI, playable.Metadata, 0, false)
			if err != nil {
				return nil, fmt.Errorf("failed to add to queue: %w", err)
			}

			queueURI := fmt.Sprintf("x-rincon-queue:%s#0", s.getDeviceUUID(ctx, deviceIP))
			if err := s.soapClient.SetAVTransportURI(ctx, deviceIP, queueURI, ""); err != nil {
				return nil, fmt.Errorf("failed to set transport URI: %w", err)
			}
		} else {
			// Direct playback
			if err := s.soapClient.SetAVTransportURI(ctx, deviceIP, playable.URI, playable.Metadata); err != nil {
				return nil, fmt.Errorf("failed to set transport URI: %w", err)
			}
		}

		// Start playback
		if err := s.soapClient.Play(ctx, deviceIP); err != nil {
			return nil, fmt.Errorf("failed to start playback: %w", err)
		}

	case QueueModePlayNext:
		// Add to queue as next track
		_, err := s.soapClient.AddURIToQueue(ctx, deviceIP, playable.URI, playable.Metadata, 0, true)
		if err != nil {
			return nil, fmt.Errorf("failed to add to queue: %w", err)
		}

	case QueueModeAddToEnd:
		// Add to end of queue
		_, err := s.soapClient.AddURIToQueue(ctx, deviceIP, playable.URI, playable.Metadata, 0, false)
		if err != nil {
			return nil, fmt.Errorf("failed to add to queue: %w", err)
		}

	case QueueModeQueueOnly:
		// Just add to queue, don't play
		_, err := s.soapClient.AddURIToQueue(ctx, deviceIP, playable.URI, playable.Metadata, 0, false)
		if err != nil {
			return nil, fmt.Errorf("failed to add to queue: %w", err)
		}
	}

	// Build response
	response := &PlayContentResponse{
		DeviceID:      deviceID,
		QueueMode:     queueMode,
		GroupBehavior: groupBehavior,
		WasUngrouped:  wasUngrouped,
		StartedAt:     time.Now().UTC(),
		Service:       playable.Service,
		ContentType:   playable.ContentType,
		Title:         playable.Title,
	}

	if req.Content.ContentID != nil {
		response.ContentID = *req.Content.ContentID
	}

	return response, nil
}

// ValidateContent validates content can be played
func (s *PlayService) ValidateContent(ctx context.Context, req ValidateContentRequest) (*ValidationResult, error) {
	// Resolve device IP if provided
	deviceIP := ""
	if req.DeviceID != nil && *req.DeviceID != "" {
		if s.deviceService != nil {
			ip, err := s.deviceService.ResolveDeviceIP(*req.DeviceID)
			if err != nil {
				return &ValidationResult{
					Valid:           false,
					DeviceAvailable: false,
					Error:           "failed to resolve device",
					Remediation:     "Check that the device ID is correct",
				}, nil
			}
			deviceIP = ip
		}
	}

	// If no device IP, use a default for validation
	if deviceIP == "" {
		// Try to get any available device
		return &ValidationResult{
			Valid:           false,
			DeviceAvailable: false,
			Error:           "device_id is required for validation",
			Remediation:     "Provide a device_id to validate content",
		}, nil
	}

	return s.contentResolver.ValidateContent(ctx, req.Content, deviceIP)
}

// GetServices returns all music service capabilities
func (s *PlayService) GetServices(ctx context.Context, deviceIP string) ([]ServiceStatus, error) {
	if deviceIP == "" {
		return nil, fmt.Errorf("device IP is required")
	}

	return s.contentResolver.GetServiceCapabilities(deviceIP), nil
}

// GetServiceHealth returns health status for a specific service
func (s *PlayService) GetServiceHealth(ctx context.Context, service, deviceIP string) (*ServiceStatus, error) {
	if service == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if deviceIP == "" {
		return nil, fmt.Errorf("device IP is required")
	}

	return s.contentResolver.GetServiceHealth(service, deviceIP)
}

// getDeviceUUID retrieves the UUID for a device
func (s *PlayService) getDeviceUUID(ctx context.Context, deviceIP string) string {
	zoneState, err := s.soapClient.GetZoneGroupState(ctx, deviceIP)
	if err != nil {
		s.logf("Warning: failed to get zone group state: %v", err)
		return deviceIP // Fallback to IP
	}

	// Find the device in the zone groups
	for _, group := range zoneState.Groups {
		for _, member := range group.Members {
			// Match by checking if this device's location contains the IP
			if member.Location != "" && containsIP(member.Location, deviceIP) {
				return member.UUID
			}
		}
	}

	return deviceIP // Fallback to IP if UUID not found
}

// containsIP checks if a location string contains the given IP
func containsIP(location, ip string) bool {
	return len(location) > 0 && len(ip) > 0 &&
		(location == ip ||
		 len(location) > len(ip)+9 && location[7:7+len(ip)] == ip)
}

// logf logs a message if logger is available
func (s *PlayService) logf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}
