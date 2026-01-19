package sonos

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// MusicContent represents content to be played
type MusicContent struct {
	Type        string  `json:"type"`                     // "sonos_favorite" or "direct"
	FavoriteID  *string `json:"favorite_id,omitempty"`    // e.g., "FV:2/34"
	Service     *string `json:"service,omitempty"`        // spotify, apple_music
	ContentType *string `json:"content_type,omitempty"`   // playlist, album, track, station
	ContentID   *string `json:"content_id,omitempty"`     // service-specific content ID
	Title       *string `json:"title,omitempty"`          // optional title for display
}

// PlayableContent is the resolved content ready for playback
type PlayableContent struct {
	URI         string `json:"uri"`
	Metadata    string `json:"metadata"`
	Title       string `json:"title"`
	ContentType string `json:"content_type"`
	Service     string `json:"service"`
	UsesQueue   bool   `json:"uses_queue"` // True if needs queue-based playback
}

// ValidationResult contains the result of content validation
type ValidationResult struct {
	Valid           bool   `json:"valid"`
	ContentType     string `json:"content_type,omitempty"`
	CanBeQueued     bool   `json:"can_be_queued"`
	Service         string `json:"service,omitempty"`
	ServiceReady    bool   `json:"service_ready"`
	DeviceAvailable bool   `json:"device_available"`
	Error           string `json:"error,omitempty"`
	Remediation     string `json:"remediation,omitempty"`
}

// ServiceStatus represents the status of a music service
type ServiceStatus struct {
	Object                string   `json:"object"`
	Service               string   `json:"service"`
	DisplayName           string   `json:"display_name"`
	Status                string   `json:"status"` // "ready", "needs_bootstrap", "not_supported"
	Ready                 bool     `json:"ready"`
	HasCredential         bool     `json:"has_credential"`
	SupportedContentTypes []string `json:"supported_content_types,omitempty"`
	LogoURL               string   `json:"logo_url,omitempty"`
	Error                 string   `json:"error,omitempty"`
	Remediation           string   `json:"remediation,omitempty"`
}

// ServiceCredentials contains credentials extracted from Sonos favorites
type ServiceCredentials struct {
	Service       string    `json:"service"`
	AccountID     string    `json:"account_id"`
	SID           string    `json:"sid"`            // Service ID (e.g., "12" for Spotify)
	SN            string    `json:"sn"`             // Service Number/Account ID
	Token         string    `json:"token"`          // Token from SA_RINCON descriptor
	SessionSuffix string    `json:"session_suffix"` // Session suffix from #Svc descriptor
	ExtractedAt   time.Time `json:"extracted_at"`
}

// Service IDs for known music services
const (
	SIDSpotify     = "12"
	SIDAppleMusic  = "204"
	SIDAmazonMusic = "201"
)

// Service name constants
const (
	ServiceSpotify     = "spotify"
	ServiceAppleMusic  = "apple_music"
	ServiceAmazonMusic = "amazon_music"
)

// Status values
const (
	StatusReady          = "ready"
	StatusNeedsBootstrap = "needs_bootstrap"
	StatusNotSupported   = "not_supported"
)

// Service display names and logos
var serviceLogos = map[string]string{
	ServiceSpotify:     "/v1/assets/service-logos/spotify.png",
	ServiceAppleMusic:  "/v1/assets/service-logos/apple-music.png",
	ServiceAmazonMusic: "/v1/assets/service-logos/amazon-music.png",
}

var serviceDisplayNames = map[string]string{
	ServiceSpotify:     "Spotify",
	ServiceAppleMusic:  "Apple Music",
	ServiceAmazonMusic: "Amazon Music",
}

var serviceSupportedContentTypes = map[string][]string{
	ServiceSpotify:     {"track", "album", "playlist", "artist"},
	ServiceAppleMusic:  {"track", "album", "playlist"},
	ServiceAmazonMusic: {}, // Not supported for direct play
}

// Regex patterns for credential extraction
var (
	sidPattern     = regexp.MustCompile(`sid=(\d+)`)
	snPattern      = regexp.MustCompile(`sn=(\d+)`)
	tokenPattern   = regexp.MustCompile(`SA_RINCON(\d+)_`)
	sessionPattern = regexp.MustCompile(`#Svc\d+-([a-f0-9]+)-Token`)
)

// cachedCredentials wraps credentials with cache metadata
type cachedCredentials struct {
	credentials *ServiceCredentials
	cachedAt    time.Time
}

// CredentialExtractor extracts service credentials from Sonos favorites
type CredentialExtractor struct {
	soapClient *soap.Client
	timeout    time.Duration
	logger     *log.Logger
	cache      map[string]*cachedCredentials
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
}

// NewCredentialExtractor creates a new CredentialExtractor
func NewCredentialExtractor(soapClient *soap.Client, timeout time.Duration, logger *log.Logger) *CredentialExtractor {
	return &CredentialExtractor{
		soapClient: soapClient,
		timeout:    timeout,
		logger:     logger,
		cache:      make(map[string]*cachedCredentials),
		cacheTTL:   24 * time.Hour,
	}
}

// GetCredentials retrieves credentials for a service from favorites
func (e *CredentialExtractor) GetCredentials(ctx context.Context, service, deviceIP string) (*ServiceCredentials, error) {
	// Check cache first
	e.cacheMu.RLock()
	cached, ok := e.cache[service]
	e.cacheMu.RUnlock()

	if ok && time.Since(cached.cachedAt) < e.cacheTTL {
		return cached.credentials, nil
	}

	// Extract fresh credentials from favorites
	allCreds, err := e.ExtractFromFavorites(ctx, deviceIP)
	if err != nil {
		return nil, err
	}

	creds, ok := allCreds[service]
	if !ok {
		return nil, &ServiceNeedsBootstrapError{Service: service}
	}

	return creds, nil
}

// ExtractFromFavorites parses favorites and extracts credentials for all services
func (e *CredentialExtractor) ExtractFromFavorites(ctx context.Context, deviceIP string) (map[string]*ServiceCredentials, error) {
	if e.soapClient == nil {
		return nil, fmt.Errorf("SOAP client not configured")
	}
	// Fetch all favorites
	result, err := e.soapClient.Browse(ctx, deviceIP, "FV:2", "BrowseDirectChildren", "*", 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to browse favorites: %w", err)
	}

	credentials := make(map[string]*ServiceCredentials)

	for _, item := range result.Items {
		creds := e.extractFromItem(item)
		if creds != nil {
			// Only store if we have meaningful credentials and don't already have this service
			if _, exists := credentials[creds.Service]; !exists {
				credentials[creds.Service] = creds
				e.cacheCredentials(creds.Service, creds)
				if e.logger != nil {
					e.logger.Printf("Extracted credentials for %s: SID=%s, SN=%s", creds.Service, creds.SID, creds.SN)
				}
			}
		}
	}

	return credentials, nil
}

// extractFromItem extracts credentials from a single favorite item using regex patterns
func (e *CredentialExtractor) extractFromItem(item soap.FavoriteItem) *ServiceCredentials {
	service := e.detectServiceFromItem(item)
	if service == "" {
		return nil
	}

	creds := &ServiceCredentials{
		Service:     service,
		ExtractedAt: time.Now(),
	}

	// Extract SID and SN from resource URI using regex
	if item.Resource != "" {
		if matches := sidPattern.FindStringSubmatch(item.Resource); len(matches) > 1 {
			creds.SID = matches[1]
		}
		if matches := snPattern.FindStringSubmatch(item.Resource); len(matches) > 1 {
			creds.SN = matches[1]
		}
	}

	// Extract token and session suffix from metadata descriptor using regex
	if item.ResourceMetaData != "" {
		if matches := tokenPattern.FindStringSubmatch(item.ResourceMetaData); len(matches) > 1 {
			creds.Token = matches[1]
		}
		if matches := sessionPattern.FindStringSubmatch(item.ResourceMetaData); len(matches) > 1 {
			creds.SessionSuffix = matches[1]
		}
		// Also extract AccountID from SA_RINCON pattern
		if idx := strings.Index(strings.ToUpper(item.ResourceMetaData), "SA_RINCON"); idx != -1 {
			remainder := item.ResourceMetaData[idx:]
			if endIdx := strings.IndexAny(remainder, " <>&"); endIdx > 0 {
				creds.AccountID = remainder[:endIdx]
			} else {
				creds.AccountID = remainder
			}
		}
	}

	// Only return if we have at least SID or token
	if creds.SID == "" && creds.Token == "" {
		return nil
	}

	return creds
}

// detectServiceFromItem determines which service a favorite belongs to
func (e *CredentialExtractor) detectServiceFromItem(item soap.FavoriteItem) string {
	resource := strings.ToLower(item.Resource)
	metadata := strings.ToLower(item.ResourceMetaData)

	// Check for Spotify
	if strings.Contains(resource, "spotify") ||
		strings.Contains(metadata, "spotify") ||
		e.hasSID(item.Resource, SIDSpotify) {
		return ServiceSpotify
	}

	// Check for Apple Music
	if strings.Contains(resource, "apple") ||
		strings.Contains(metadata, "sa_rincon52231") ||
		e.hasSID(item.Resource, SIDAppleMusic) {
		return ServiceAppleMusic
	}

	// Check for Amazon Music
	if strings.Contains(resource, "amazon") ||
		strings.Contains(resource, "amzn") ||
		e.hasSID(item.Resource, SIDAmazonMusic) {
		return ServiceAmazonMusic
	}

	return ""
}

// hasSID checks if a URI contains a specific service ID
func (e *CredentialExtractor) hasSID(uri, expectedSID string) bool {
	matches := sidPattern.FindStringSubmatch(uri)
	if len(matches) > 1 {
		return matches[1] == expectedSID
	}
	return false
}

// cacheCredentials stores credentials in the cache
func (e *CredentialExtractor) cacheCredentials(service string, creds *ServiceCredentials) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()
	e.cache[service] = &cachedCredentials{
		credentials: creds,
		cachedAt:    time.Now(),
	}
}

// GetServiceStatus returns the status of a specific service
func (e *CredentialExtractor) GetServiceStatus(service string) string {
	// Amazon Music is never supported for direct play
	if service == ServiceAmazonMusic {
		return StatusNotSupported
	}

	e.cacheMu.RLock()
	cached, ok := e.cache[service]
	e.cacheMu.RUnlock()

	if ok && time.Since(cached.cachedAt) < e.cacheTTL {
		return StatusReady
	}

	return StatusNeedsBootstrap
}

// GetAllServiceStatuses returns status for all known services
func (e *CredentialExtractor) GetAllServiceStatuses(ctx context.Context, deviceIP string) map[string]ServiceStatus {
	// Try to refresh credentials from favorites (only if we have a SOAP client)
	if e.soapClient != nil {
		_, _ = e.ExtractFromFavorites(ctx, deviceIP)
	}

	statuses := make(map[string]ServiceStatus)
	knownServices := []string{ServiceSpotify, ServiceAppleMusic, ServiceAmazonMusic}

	for _, service := range knownServices {
		status := e.GetServiceStatus(service)
		serviceStatus := ServiceStatus{
			Object:                "service_status",
			Service:               service,
			DisplayName:           serviceDisplayNames[service],
			Status:                status,
			Ready:                 status == StatusReady,
			HasCredential:         status == StatusReady,
			SupportedContentTypes: serviceSupportedContentTypes[service],
			LogoURL:               serviceLogos[service],
		}

		// Add remediation message for services that need bootstrap
		if status == StatusNeedsBootstrap {
			serviceStatus.Remediation = fmt.Sprintf("Add a %s favorite in the Sonos app to enable direct playback", serviceDisplayNames[service])
		} else if status == StatusNotSupported {
			serviceStatus.Remediation = "This service does not support direct playback through the Sonos API"
		}

		statuses[service] = serviceStatus
	}

	return statuses
}

// IsCacheValid checks if credentials are cached and valid for a service
func (e *CredentialExtractor) IsCacheValid(service string) bool {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()

	cached, ok := e.cache[service]
	if !ok {
		return false
	}

	return time.Since(cached.cachedAt) < e.cacheTTL
}

// SetCacheTTL allows adjusting the cache TTL (useful for testing)
func (e *CredentialExtractor) SetCacheTTL(ttl time.Duration) {
	e.cacheTTL = ttl
}

// HasCredentials checks if credentials exist for a service
func (e *CredentialExtractor) HasCredentials(ctx context.Context, service, deviceIP string) bool {
	creds, _ := e.GetCredentials(ctx, service, deviceIP)
	return creds != nil
}

// ClearCache clears the credentials cache
func (e *CredentialExtractor) ClearCache() {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()
	e.cache = make(map[string]*cachedCredentials)
}

// URIBuilder builds Sonos-compatible URIs for different services
type URIBuilder struct {
	logger *log.Logger
}

// NewURIBuilder creates a new URIBuilder
func NewURIBuilder(logger *log.Logger) *URIBuilder {
	return &URIBuilder{logger: logger}
}

// BuildURI builds a playable URI for the given service and content
func (b *URIBuilder) BuildURI(service, contentType, contentID string, creds *ServiceCredentials) (string, error) {
	switch service {
	case "spotify":
		return b.buildSpotifyURI(contentType, contentID, creds)
	case "apple_music":
		return b.buildAppleMusicURI(contentType, contentID, creds)
	default:
		return "", &ServiceNotSupportedError{Service: service}
	}
}

// BuildMetadata builds DIDL-Lite metadata for the content
func (b *URIBuilder) BuildMetadata(service, contentType, contentID, title string, creds *ServiceCredentials) (string, error) {
	switch service {
	case "spotify":
		return b.buildSpotifyMetadata(contentType, contentID, title, creds)
	case "apple_music":
		return b.buildAppleMusicMetadata(contentType, contentID, title, creds)
	default:
		return "", &ServiceNotSupportedError{Service: service}
	}
}

func (b *URIBuilder) buildSpotifyURI(contentType, contentID string, creds *ServiceCredentials) (string, error) {
	// Spotify URIs use x-rincon-cpcontainer for containers (playlists, albums)
	// and x-sonos-spotify for tracks
	switch contentType {
	case "playlist":
		return fmt.Sprintf("x-rincon-cpcontainer:1006206c%s?sid=%s&flags=8300&sn=1",
			contentID, creds.SID), nil
	case "album":
		return fmt.Sprintf("x-rincon-cpcontainer:1004206c%s?sid=%s&flags=8300&sn=1",
			contentID, creds.SID), nil
	case "track":
		return fmt.Sprintf("x-sonos-spotify:spotify:track:%s?sid=%s&flags=8224&sn=1",
			contentID, creds.SID), nil
	case "station", "radio":
		return fmt.Sprintf("x-sonosapi-radio:spotify:station:%s?sid=%s&flags=8300&sn=1",
			contentID, creds.SID), nil
	default:
		return "", fmt.Errorf("unsupported content type for Spotify: %s", contentType)
	}
}

func (b *URIBuilder) buildAppleMusicURI(contentType, contentID string, creds *ServiceCredentials) (string, error) {
	switch contentType {
	case "playlist":
		return fmt.Sprintf("x-rincon-cpcontainer:1006006cplaylist:%s?sid=%s",
			contentID, creds.SID), nil
	case "album":
		return fmt.Sprintf("x-rincon-cpcontainer:1004006calbum:%s?sid=%s",
			contentID, creds.SID), nil
	case "track":
		return fmt.Sprintf("x-sonos-http:song%%3a%s.mp4?sid=%s",
			contentID, creds.SID), nil
	case "station", "radio":
		return fmt.Sprintf("x-sonosapi-radio:station:%s?sid=%s",
			contentID, creds.SID), nil
	default:
		return "", fmt.Errorf("unsupported content type for Apple Music: %s", contentType)
	}
}

func (b *URIBuilder) buildSpotifyMetadata(contentType, contentID, title string, creds *ServiceCredentials) (string, error) {
	var upnpClass string
	var itemID string

	switch contentType {
	case "playlist":
		upnpClass = "object.container.playlistContainer"
		itemID = "1006206c" + contentID
	case "album":
		upnpClass = "object.container.album.musicAlbum"
		itemID = "1004206c" + contentID
	case "track":
		upnpClass = "object.item.audioItem.musicTrack"
		itemID = "00032020" + contentID
	case "station", "radio":
		upnpClass = "object.item.audioItem.audioBroadcast"
		itemID = "100c206c" + contentID
	default:
		return "", fmt.Errorf("unsupported content type: %s", contentType)
	}

	return buildDidlMetadata(itemID, title, upnpClass, creds.AccountID), nil
}

func (b *URIBuilder) buildAppleMusicMetadata(contentType, contentID, title string, creds *ServiceCredentials) (string, error) {
	var upnpClass string
	var itemID string

	switch contentType {
	case "playlist":
		upnpClass = "object.container.playlistContainer"
		itemID = "1006006cplaylist:" + contentID
	case "album":
		upnpClass = "object.container.album.musicAlbum"
		itemID = "1004006calbum:" + contentID
	case "track":
		upnpClass = "object.item.audioItem.musicTrack"
		itemID = "10032020song:" + contentID
	case "station", "radio":
		upnpClass = "object.item.audioItem.audioBroadcast"
		itemID = "100c006cstation:" + contentID
	default:
		return "", fmt.Errorf("unsupported content type: %s", contentType)
	}

	return buildDidlMetadata(itemID, title, upnpClass, creds.AccountID), nil
}

func buildDidlMetadata(itemID, title, upnpClass, accountID string) string {
	if title == "" {
		title = "Unknown"
	}
	return fmt.Sprintf(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="%s" parentID="0" restricted="true"><dc:title>%s</dc:title><upnp:class>%s</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">%s</desc></item></DIDL-Lite>`,
		itemID, title, upnpClass, accountID)
}

// DeviceResolver interface for device IP resolution
type DeviceResolver interface {
	ResolveDeviceIP(deviceID string) (string, error)
}

// ContentResolver is the main orchestrator for resolving music content to playable URIs
type ContentResolver struct {
	soapClient          *soap.Client
	credentialExtractor *CredentialExtractor
	uriBuilder          *URIBuilder
	deviceService       DeviceResolver
	timeout             time.Duration
	logger              *log.Logger
}

// NewContentResolver creates a new ContentResolver
func NewContentResolver(soapClient *soap.Client, deviceResolver DeviceResolver, timeout time.Duration, logger *log.Logger) *ContentResolver {
	return &ContentResolver{
		soapClient:          soapClient,
		credentialExtractor: NewCredentialExtractor(soapClient, timeout, logger),
		uriBuilder:          NewURIBuilder(logger),
		deviceService:       deviceResolver,
		timeout:             timeout,
		logger:              logger,
	}
}

// ResolveContent resolves MusicContent to a PlayableContent
// For sonos_favorite: fetches and parses the favorite
// For direct: builds URI using credentials extracted from favorites
func (r *ContentResolver) ResolveContent(ctx context.Context, content MusicContent, deviceIP string) (*PlayableContent, error) {
	switch content.Type {
	case "sonos_favorite":
		if content.FavoriteID == nil || *content.FavoriteID == "" {
			return nil, fmt.Errorf("favorite_id is required for sonos_favorite type")
		}
		return r.ResolveFavorite(ctx, *content.FavoriteID, deviceIP)

	case "direct":
		if content.Service == nil || *content.Service == "" {
			return nil, fmt.Errorf("service is required for direct type")
		}
		if content.ContentType == nil || *content.ContentType == "" {
			return nil, fmt.Errorf("content_type is required for direct type")
		}
		if content.ContentID == nil || *content.ContentID == "" {
			return nil, fmt.Errorf("content_id is required for direct type")
		}
		title := ""
		if content.Title != nil {
			title = *content.Title
		}
		return r.ResolveDirectContent(ctx, *content.Service, *content.ContentType, *content.ContentID, title, deviceIP)

	default:
		return nil, fmt.Errorf("unknown content type: %s", content.Type)
	}
}

// ResolveFavorite fetches a Sonos favorite and returns playable content
func (r *ContentResolver) ResolveFavorite(ctx context.Context, favoriteID, deviceIP string) (*PlayableContent, error) {
	// Browse favorites
	browseResult, err := r.soapClient.Browse(ctx, deviceIP, "FV:2", "BrowseDirectChildren", "*", 0, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to browse favorites: %w", err)
	}

	// Find the specific favorite
	var favorite *soap.FavoriteItem
	for i := range browseResult.Items {
		if browseResult.Items[i].ID == favoriteID {
			favorite = &browseResult.Items[i]
			break
		}
	}

	if favorite == nil {
		return nil, &FavoriteNotFoundError{FavoriteID: favoriteID}
	}

	// Determine content type from upnp:class
	contentType := r.determineContentType(favorite.UpnpClass)

	// Determine service from resource URI
	service := detectServiceName(favorite.Resource, favorite.ResourceMetaData)

	// Determine if queue-based playback is needed
	usesQueue := strings.HasPrefix(strings.ToLower(favorite.Resource), "x-rincon-cpcontainer")

	return &PlayableContent{
		URI:         favorite.Resource,
		Metadata:    favorite.ResourceMetaData,
		Title:       favorite.Title,
		ContentType: contentType,
		Service:     service,
		UsesQueue:   usesQueue,
	}, nil
}

// ResolveDirectContent builds playable content for a direct service request
func (r *ContentResolver) ResolveDirectContent(ctx context.Context, service, contentType, contentID, title, deviceIP string) (*PlayableContent, error) {
	// Validate service is supported
	if !r.isServiceSupported(service) {
		return nil, &ServiceNotSupportedError{Service: service}
	}

	// Get credentials from favorites
	creds, err := r.credentialExtractor.GetCredentials(ctx, service, deviceIP)
	if err != nil {
		return nil, err
	}

	// Build URI
	uri, err := r.uriBuilder.BuildURI(service, contentType, contentID, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to build URI: %w", err)
	}

	// Build metadata
	metadata, err := r.uriBuilder.BuildMetadata(service, contentType, contentID, title, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to build metadata: %w", err)
	}

	// Determine if queue-based playback is needed
	usesQueue := contentType == "playlist" || contentType == "album"

	displayTitle := title
	if displayTitle == "" {
		displayTitle = fmt.Sprintf("%s %s", service, contentType)
	}

	return &PlayableContent{
		URI:         uri,
		Metadata:    metadata,
		Title:       displayTitle,
		ContentType: contentType,
		Service:     service,
		UsesQueue:   usesQueue,
	}, nil
}

// UsesQueuePlayback returns true if the content type requires queue-based playback
// Containers (playlists, albums) use queue; tracks and stations use direct setAVTransportURI
func (r *ContentResolver) UsesQueuePlayback(content MusicContent) bool {
	if content.Type == "sonos_favorite" {
		// For favorites, we can't determine this without fetching the favorite
		// Caller should check after resolution
		return false
	}
	if content.ContentType != nil {
		ct := *content.ContentType
		// Playlists and albums use queue; tracks and stations use direct
		return ct == "playlist" || ct == "album"
	}
	return false
}

// ValidateContent validates that content can be played without actually playing it
func (r *ContentResolver) ValidateContent(ctx context.Context, content MusicContent, deviceIP string) (*ValidationResult, error) {
	result := &ValidationResult{
		DeviceAvailable: true, // Assume available if we can reach it
	}

	// Check device availability by attempting a simple SOAP call
	_, err := r.soapClient.GetTransportInfo(ctx, deviceIP)
	if err != nil {
		result.DeviceAvailable = false
		result.Valid = false
		result.Error = "device not reachable"
		result.Remediation = "Check that the Sonos device is powered on and connected to the network"
		return result, nil
	}

	switch content.Type {
	case "sonos_favorite":
		if content.FavoriteID == nil || *content.FavoriteID == "" {
			result.Valid = false
			result.Error = "favorite_id is required"
			return result, nil
		}

		// Try to resolve the favorite
		playable, err := r.ResolveFavorite(ctx, *content.FavoriteID, deviceIP)
		if err != nil {
			if _, ok := err.(*FavoriteNotFoundError); ok {
				result.Valid = false
				result.Error = "favorite not found"
				result.Remediation = "Check the favorite ID or browse available favorites"
				return result, nil
			}
			result.Valid = false
			result.Error = err.Error()
			return result, nil
		}

		result.Valid = true
		result.ContentType = playable.ContentType
		result.CanBeQueued = playable.UsesQueue
		result.Service = playable.Service
		result.ServiceReady = true

	case "direct":
		if content.Service == nil || *content.Service == "" {
			result.Valid = false
			result.Error = "service is required"
			return result, nil
		}

		service := *content.Service
		result.Service = service

		if !r.isServiceSupported(service) {
			result.Valid = false
			result.Error = fmt.Sprintf("service '%s' is not supported for direct playback", service)
			result.Remediation = "Supported services: spotify, apple_music"
			return result, nil
		}

		// Check for credentials
		if !r.credentialExtractor.HasCredentials(ctx, service, deviceIP) {
			result.Valid = false
			result.ServiceReady = false
			result.Error = fmt.Sprintf("no credentials found for %s", service)
			result.Remediation = fmt.Sprintf("Add a %s item to your Sonos favorites to bootstrap credentials", service)
			return result, nil
		}

		result.ServiceReady = true

		if content.ContentType == nil || *content.ContentType == "" {
			result.Valid = false
			result.Error = "content_type is required"
			return result, nil
		}

		if content.ContentID == nil || *content.ContentID == "" {
			result.Valid = false
			result.Error = "content_id is required"
			return result, nil
		}

		result.Valid = true
		result.ContentType = *content.ContentType
		result.CanBeQueued = r.UsesQueuePlayback(content)

	default:
		result.Valid = false
		result.Error = fmt.Sprintf("unknown content type: %s", content.Type)
		result.Remediation = "Valid types are: sonos_favorite, direct"
	}

	return result, nil
}

// GetServiceCapabilities returns capabilities for all music services
func (r *ContentResolver) GetServiceCapabilities(deviceIP string) []ServiceStatus {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	services := []string{"spotify", "apple_music", "amazon_music"}
	statuses := make([]ServiceStatus, 0, len(services))

	for _, svc := range services {
		status := ServiceStatus{
			Object:  "service_status",
			Service: svc,
		}

		if r.isServiceSupported(svc) {
			creds, err := r.credentialExtractor.GetCredentials(ctx, svc, deviceIP)
			if err != nil {
				if _, ok := err.(*ServiceNeedsBootstrapError); ok {
					status.Ready = false
					status.HasCredential = false
					status.Error = fmt.Sprintf("Add a %s item to Sonos favorites to enable", svc)
				} else {
					status.Ready = false
					status.Error = err.Error()
				}
			} else {
				status.Ready = true
				status.HasCredential = creds != nil
			}
		} else {
			status.Ready = false
			status.Error = "Direct playback not supported for this service"
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// GetServiceHealth checks if a specific service is ready
func (r *ContentResolver) GetServiceHealth(service, deviceIP string) (*ServiceStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	status := &ServiceStatus{
		Object:  "service_status",
		Service: service,
	}

	if !r.isServiceSupported(service) {
		status.Ready = false
		status.Error = "Service not supported for direct playback"
		return status, nil
	}

	creds, err := r.credentialExtractor.GetCredentials(ctx, service, deviceIP)
	if err != nil {
		if _, ok := err.(*ServiceNeedsBootstrapError); ok {
			status.Ready = false
			status.HasCredential = false
			status.Error = fmt.Sprintf("Add a %s item to Sonos favorites to bootstrap credentials", service)
		} else {
			status.Ready = false
			status.Error = err.Error()
		}
		return status, nil
	}

	status.Ready = true
	status.HasCredential = creds != nil
	return status, nil
}

// isServiceSupported checks if a service supports direct playback
func (r *ContentResolver) isServiceSupported(service string) bool {
	switch service {
	case "spotify", "apple_music":
		return true
	default:
		return false
	}
}

// determineContentType determines content type from upnp:class
func (r *ContentResolver) determineContentType(upnpClass string) string {
	upnpClass = strings.ToLower(upnpClass)
	switch {
	case strings.Contains(upnpClass, "audiobroadcast") || strings.Contains(upnpClass, "radio"):
		return "station"
	case strings.Contains(upnpClass, "playlistcontainer") || strings.Contains(upnpClass, "playlist"):
		return "playlist"
	case strings.Contains(upnpClass, "album") || strings.Contains(upnpClass, "musicalbum"):
		return "album"
	case strings.Contains(upnpClass, "musictrack") || strings.Contains(upnpClass, "audioitem"):
		return "track"
	default:
		return "unknown"
	}
}

// Error types

// FavoriteNotFoundError indicates a favorite was not found
type FavoriteNotFoundError struct {
	FavoriteID string
}

func (e *FavoriteNotFoundError) Error() string {
	return fmt.Sprintf("favorite not found: %s", e.FavoriteID)
}

// ServiceNotSupportedError indicates a service doesn't support direct playback
type ServiceNotSupportedError struct {
	Service string
}

func (e *ServiceNotSupportedError) Error() string {
	return fmt.Sprintf("service '%s' does not support direct playback", e.Service)
}

// ServiceNeedsBootstrapError indicates credentials need to be bootstrapped
type ServiceNeedsBootstrapError struct {
	Service string
}

func (e *ServiceNeedsBootstrapError) Error() string {
	return fmt.Sprintf("service '%s' needs credentials - add a %s item to Sonos favorites", e.Service, e.Service)
}

// ContentUnavailableError indicates content could not be resolved
type ContentUnavailableError struct {
	Reason string
}

func (e *ContentUnavailableError) Error() string {
	return fmt.Sprintf("content unavailable: %s", e.Reason)
}
