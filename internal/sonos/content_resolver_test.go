package sonos

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
	"github.com/stretchr/testify/require"
)

// mockDeviceResolver implements DeviceResolver for testing
type mockDeviceResolver struct {
	ip  string
	err error
}

func (m *mockDeviceResolver) ResolveDeviceIP(deviceID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.ip, nil
}

// testHelper wraps test dependencies
type testHelper struct {
	soapClient *soap.Client
	resolver   *ContentResolver
	logger     *log.Logger
}

func setupTest(t *testing.T) *testHelper {
	t.Helper()
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	deviceResolver := &mockDeviceResolver{ip: "192.168.1.100"}
	resolver := NewContentResolver(soapClient, deviceResolver, 5*time.Second, logger)

	return &testHelper{
		soapClient: soapClient,
		resolver:   resolver,
		logger:     logger,
	}
}

func TestNewContentResolver(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	deviceResolver := &mockDeviceResolver{ip: "192.168.1.100"}

	resolver := NewContentResolver(soapClient, deviceResolver, 5*time.Second, logger)

	require.NotNil(t, resolver)
	require.NotNil(t, resolver.soapClient)
	require.NotNil(t, resolver.credentialExtractor)
	require.NotNil(t, resolver.uriBuilder)
	require.NotNil(t, resolver.deviceService)
	require.Equal(t, 5*time.Second, resolver.timeout)
}

func TestMusicContent_Types(t *testing.T) {
	// Test sonos_favorite type
	favID := "FV:2/34"
	favContent := MusicContent{
		Type:       "sonos_favorite",
		FavoriteID: &favID,
	}
	require.Equal(t, "sonos_favorite", favContent.Type)
	require.NotNil(t, favContent.FavoriteID)
	require.Equal(t, "FV:2/34", *favContent.FavoriteID)

	// Test direct type
	service := "spotify"
	contentType := "playlist"
	contentID := "37i9dQZF1DX5Ejj0EkURtP"
	title := "My Playlist"
	directContent := MusicContent{
		Type:        "direct",
		Service:     &service,
		ContentType: &contentType,
		ContentID:   &contentID,
		Title:       &title,
	}
	require.Equal(t, "direct", directContent.Type)
	require.Equal(t, "spotify", *directContent.Service)
	require.Equal(t, "playlist", *directContent.ContentType)
	require.Equal(t, "37i9dQZF1DX5Ejj0EkURtP", *directContent.ContentID)
	require.Equal(t, "My Playlist", *directContent.Title)
}

func TestUsesQueuePlayback(t *testing.T) {
	h := setupTest(t)

	tests := []struct {
		name        string
		content     MusicContent
		expectQueue bool
	}{
		{
			name: "playlist uses queue",
			content: MusicContent{
				Type:        "direct",
				ContentType: strPtr("playlist"),
			},
			expectQueue: true,
		},
		{
			name: "album uses queue",
			content: MusicContent{
				Type:        "direct",
				ContentType: strPtr("album"),
			},
			expectQueue: true,
		},
		{
			name: "track does not use queue",
			content: MusicContent{
				Type:        "direct",
				ContentType: strPtr("track"),
			},
			expectQueue: false,
		},
		{
			name: "station does not use queue",
			content: MusicContent{
				Type:        "direct",
				ContentType: strPtr("station"),
			},
			expectQueue: false,
		},
		{
			name: "sonos_favorite returns false (needs resolution)",
			content: MusicContent{
				Type:       "sonos_favorite",
				FavoriteID: strPtr("FV:2/34"),
			},
			expectQueue: false,
		},
		{
			name: "nil content type",
			content: MusicContent{
				Type: "direct",
			},
			expectQueue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.resolver.UsesQueuePlayback(tt.content)
			require.Equal(t, tt.expectQueue, result)
		})
	}
}

func TestURIBuilder_BuildSpotifyURI(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	builder := NewURIBuilder(logger)

	creds := &ServiceCredentials{
		Service:   "spotify",
		SID:       "12",
		AccountID: "SA_RINCON12345",
	}

	tests := []struct {
		name        string
		contentType string
		contentID   string
		expectURI   string
		expectError bool
	}{
		{
			name:        "spotify playlist",
			contentType: "playlist",
			contentID:   "37i9dQZF1DX5Ejj0EkURtP",
			expectURI:   "x-rincon-cpcontainer:1006206c37i9dQZF1DX5Ejj0EkURtP?sid=12&flags=8300&sn=1",
		},
		{
			name:        "spotify album",
			contentType: "album",
			contentID:   "4aawyAB9vmqN3uQ7FjRGTy",
			expectURI:   "x-rincon-cpcontainer:1004206c4aawyAB9vmqN3uQ7FjRGTy?sid=12&flags=8300&sn=1",
		},
		{
			name:        "spotify track",
			contentType: "track",
			contentID:   "11dFghVXANMlKmJXsNCbNl",
			expectURI:   "x-sonos-spotify:spotify:track:11dFghVXANMlKmJXsNCbNl?sid=12&flags=8224&sn=1",
		},
		{
			name:        "spotify station",
			contentType: "station",
			contentID:   "artist:1vCWHaC5f2uS3yhpwWbIA6",
			expectURI:   "x-sonosapi-radio:spotify:station:artist:1vCWHaC5f2uS3yhpwWbIA6?sid=12&flags=8300&sn=1",
		},
		{
			name:        "unsupported content type",
			contentType: "podcast",
			contentID:   "123",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := builder.BuildURI("spotify", tt.contentType, tt.contentID, creds)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectURI, uri)
			}
		})
	}
}

func TestURIBuilder_BuildAppleMusicURI(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	builder := NewURIBuilder(logger)

	creds := &ServiceCredentials{
		Service:   "apple_music",
		SID:       "204",
		AccountID: "SA_RINCON204",
	}

	tests := []struct {
		name        string
		contentType string
		contentID   string
		expectURI   string
		expectError bool
	}{
		{
			name:        "apple music playlist",
			contentType: "playlist",
			contentID:   "pl.u-aZb0kMBIqqJv0L",
			expectURI:   "x-rincon-cpcontainer:1006006cplaylist:pl.u-aZb0kMBIqqJv0L?sid=204",
		},
		{
			name:        "apple music album",
			contentType: "album",
			contentID:   "1234567890",
			expectURI:   "x-rincon-cpcontainer:1004006calbum:1234567890?sid=204",
		},
		{
			name:        "apple music track",
			contentType: "track",
			contentID:   "1234567890",
			expectURI:   "x-sonos-http:song%3a1234567890.mp4?sid=204",
		},
		{
			name:        "apple music station",
			contentType: "station",
			contentID:   "ra.u-abc123",
			expectURI:   "x-sonosapi-radio:station:ra.u-abc123?sid=204",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := builder.BuildURI("apple_music", tt.contentType, tt.contentID, creds)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectURI, uri)
			}
		})
	}
}

func TestURIBuilder_UnsupportedService(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	builder := NewURIBuilder(logger)

	creds := &ServiceCredentials{
		Service: "deezer",
		SID:     "100",
	}

	_, err := builder.BuildURI("deezer", "playlist", "123", creds)
	require.Error(t, err)
	var serviceErr *ServiceNotSupportedError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "deezer", serviceErr.Service)
}

func TestURIBuilder_BuildMetadata(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	builder := NewURIBuilder(logger)

	creds := &ServiceCredentials{
		Service:   "spotify",
		SID:       "12",
		AccountID: "SA_RINCON12345",
	}

	metadata, err := builder.BuildMetadata("spotify", "playlist", "abc123", "My Playlist", creds)
	require.NoError(t, err)
	require.Contains(t, metadata, "DIDL-Lite")
	require.Contains(t, metadata, "My Playlist")
	require.Contains(t, metadata, "object.container.playlistContainer")
	require.Contains(t, metadata, "SA_RINCON12345")
}

func TestCredentialExtractor_ExtractFromItem(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	extractor := NewCredentialExtractor(soapClient, 5*time.Second, logger)

	tests := []struct {
		name        string
		item        soap.FavoriteItem
		expectCreds bool
		expectSID   string
		expectSN    string
		service     string
	}{
		{
			name: "spotify favorite with sid and sn",
			item: soap.FavoriteItem{
				Resource:         "x-rincon-cpcontainer:1006206cabc123?sid=12&sn=5&flags=8300",
				ResourceMetaData: "SA_RINCON12345_X",
			},
			expectCreds: true,
			expectSID:   "12",
			expectSN:    "5",
			service:     ServiceSpotify,
		},
		{
			name: "apple music favorite",
			item: soap.FavoriteItem{
				Resource:         "x-sonos-http:song%3a123.mp4?sid=204&sn=8",
				ResourceMetaData: "SA_RINCON52231",
			},
			expectCreds: true,
			expectSID:   "204",
			expectSN:    "8",
			service:     ServiceAppleMusic,
		},
		{
			name: "tunein favorite (no extractable creds)",
			item: soap.FavoriteItem{
				Resource:         "x-sonosapi-stream:s123?sid=254",
				ResourceMetaData: "SA_RINCON254",
			},
			expectCreds: false,
		},
		{
			name: "empty resource",
			item: soap.FavoriteItem{
				Resource:         "",
				ResourceMetaData: "",
			},
			expectCreds: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := extractor.extractFromItem(tt.item)
			if tt.expectCreds {
				require.NotNil(t, creds)
				require.Equal(t, tt.expectSID, creds.SID)
				require.Equal(t, tt.expectSN, creds.SN)
				require.Equal(t, tt.service, creds.Service)
			} else {
				require.Nil(t, creds)
			}
		})
	}
}

func TestCredentialExtractor_DetectServiceFromItem(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	extractor := NewCredentialExtractor(soapClient, 5*time.Second, logger)

	tests := []struct {
		name     string
		item     soap.FavoriteItem
		expected string
	}{
		{
			name: "spotify by keyword",
			item: soap.FavoriteItem{
				Resource: "x-sonos-spotify:spotify:track:abc",
			},
			expected: ServiceSpotify,
		},
		{
			name: "spotify by sid",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:abc?sid=12&sn=1",
			},
			expected: ServiceSpotify,
		},
		{
			name: "apple music by keyword in resource",
			item: soap.FavoriteItem{
				Resource: "x-sonos-http:apple-music:song%3aabc.mp4",
			},
			expected: ServiceAppleMusic,
		},
		{
			name: "apple music by sid",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:abc?sid=204&sn=1",
			},
			expected: ServiceAppleMusic,
		},
		{
			name: "amazon music by keyword",
			item: soap.FavoriteItem{
				Resource: "x-sonosapi-hls-static:catalog/tracks/amazon/abc",
			},
			expected: ServiceAmazonMusic,
		},
		{
			name: "amazon music by sid",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:abc?sid=201&sn=1",
			},
			expected: ServiceAmazonMusic,
		},
		{
			name: "unknown service",
			item: soap.FavoriteItem{
				Resource: "x-sonosapi-stream:s12345?sid=254",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.detectServiceFromItem(tt.item)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCredentialExtractor_CacheOperations(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	extractor := NewCredentialExtractor(soapClient, 5*time.Second, logger)

	// Set short TTL for testing
	extractor.SetCacheTTL(100 * time.Millisecond)

	// Cache some credentials
	testCreds := &ServiceCredentials{
		Service: ServiceSpotify,
		SID:     "12",
		SN:      "5",
	}
	extractor.cacheCredentials(ServiceSpotify, testCreds)

	// Should be valid immediately
	require.True(t, extractor.IsCacheValid(ServiceSpotify))
	require.False(t, extractor.IsCacheValid(ServiceAppleMusic))

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)
	require.False(t, extractor.IsCacheValid(ServiceSpotify))

	// Test ClearCache
	extractor.cacheCredentials(ServiceSpotify, testCreds)
	require.True(t, extractor.IsCacheValid(ServiceSpotify))
	extractor.ClearCache()
	require.False(t, extractor.IsCacheValid(ServiceSpotify))
}

func TestCredentialExtractor_GetServiceStatus(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	extractor := NewCredentialExtractor(soapClient, 5*time.Second, logger)

	// Amazon Music is always not supported
	require.Equal(t, StatusNotSupported, extractor.GetServiceStatus(ServiceAmazonMusic))

	// Uncached service needs bootstrap
	require.Equal(t, StatusNeedsBootstrap, extractor.GetServiceStatus(ServiceSpotify))

	// Cache credentials and check status
	extractor.cacheCredentials(ServiceSpotify, &ServiceCredentials{
		Service: ServiceSpotify,
		SID:     "12",
	})
	require.Equal(t, StatusReady, extractor.GetServiceStatus(ServiceSpotify))
}

func TestResolveContent_ValidationErrors(t *testing.T) {
	h := setupTest(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		content     MusicContent
		expectError string
	}{
		{
			name: "sonos_favorite without favorite_id",
			content: MusicContent{
				Type: "sonos_favorite",
			},
			expectError: "favorite_id is required",
		},
		{
			name: "sonos_favorite with empty favorite_id",
			content: MusicContent{
				Type:       "sonos_favorite",
				FavoriteID: strPtr(""),
			},
			expectError: "favorite_id is required",
		},
		{
			name: "direct without service",
			content: MusicContent{
				Type: "direct",
			},
			expectError: "service is required",
		},
		{
			name: "direct without content_type",
			content: MusicContent{
				Type:    "direct",
				Service: strPtr("spotify"),
			},
			expectError: "content_type is required",
		},
		{
			name: "direct without content_id",
			content: MusicContent{
				Type:        "direct",
				Service:     strPtr("spotify"),
				ContentType: strPtr("playlist"),
			},
			expectError: "content_id is required",
		},
		{
			name: "unknown content type",
			content: MusicContent{
				Type: "unknown_type",
			},
			expectError: "unknown content type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.resolver.ResolveContent(ctx, tt.content, "192.168.1.100")
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestPlayableContent_Fields(t *testing.T) {
	content := PlayableContent{
		URI:         "x-rincon-cpcontainer:abc123",
		Metadata:    "<DIDL-Lite>...</DIDL-Lite>",
		Title:       "My Playlist",
		ContentType: "playlist",
		Service:     "Spotify",
		UsesQueue:   true,
	}

	require.Equal(t, "x-rincon-cpcontainer:abc123", content.URI)
	require.Contains(t, content.Metadata, "DIDL-Lite")
	require.Equal(t, "My Playlist", content.Title)
	require.Equal(t, "playlist", content.ContentType)
	require.Equal(t, "Spotify", content.Service)
	require.True(t, content.UsesQueue)
}

func TestValidationResult_Fields(t *testing.T) {
	result := ValidationResult{
		Valid:           true,
		ContentType:     "playlist",
		CanBeQueued:     true,
		Service:         "spotify",
		ServiceReady:    true,
		DeviceAvailable: true,
	}

	require.True(t, result.Valid)
	require.Equal(t, "playlist", result.ContentType)
	require.True(t, result.CanBeQueued)
	require.Equal(t, "spotify", result.Service)
	require.True(t, result.ServiceReady)
	require.True(t, result.DeviceAvailable)

	// Test error case
	errorResult := ValidationResult{
		Valid:       false,
		Error:       "device not reachable",
		Remediation: "Check device power",
	}

	require.False(t, errorResult.Valid)
	require.Equal(t, "device not reachable", errorResult.Error)
	require.Equal(t, "Check device power", errorResult.Remediation)
}

func TestServiceStatus_Fields(t *testing.T) {
	status := ServiceStatus{
		Service:               ServiceSpotify,
		DisplayName:           "Spotify",
		Status:                StatusReady,
		Ready:                 true,
		HasCredential:         true,
		SupportedContentTypes: []string{"track", "album", "playlist"},
		LogoURL:               "/v1/assets/service-logos/spotify.png",
	}

	require.Equal(t, ServiceSpotify, status.Service)
	require.Equal(t, "Spotify", status.DisplayName)
	require.Equal(t, StatusReady, status.Status)
	require.True(t, status.Ready)
	require.True(t, status.HasCredential)
	require.Contains(t, status.SupportedContentTypes, "playlist")
	require.Contains(t, status.LogoURL, "spotify")

	// Test not ready case
	notReady := ServiceStatus{
		Service:     ServiceAmazonMusic,
		DisplayName: "Amazon Music",
		Status:      StatusNotSupported,
		Ready:       false,
		Remediation: "This service does not support direct playback",
	}

	require.Equal(t, ServiceAmazonMusic, notReady.Service)
	require.False(t, notReady.Ready)
	require.Equal(t, StatusNotSupported, notReady.Status)
	require.Contains(t, notReady.Remediation, "not support")
}

func TestServiceCredentials_Fields(t *testing.T) {
	creds := ServiceCredentials{
		Service:       ServiceSpotify,
		AccountID:     "SA_RINCON12345_X",
		SID:           "12",
		SN:            "5",
		Token:         "12345",
		SessionSuffix: "abc123",
		ExtractedAt:   time.Now(),
	}

	require.Equal(t, ServiceSpotify, creds.Service)
	require.Equal(t, "SA_RINCON12345_X", creds.AccountID)
	require.Equal(t, "12", creds.SID)
	require.Equal(t, "5", creds.SN)
	require.Equal(t, "12345", creds.Token)
	require.Equal(t, "abc123", creds.SessionSuffix)
	require.False(t, creds.ExtractedAt.IsZero())
}

func TestErrorTypes(t *testing.T) {
	t.Run("FavoriteNotFoundError", func(t *testing.T) {
		err := &FavoriteNotFoundError{FavoriteID: "FV:2/99"}
		require.Contains(t, err.Error(), "FV:2/99")
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("ServiceNotSupportedError", func(t *testing.T) {
		err := &ServiceNotSupportedError{Service: "deezer"}
		require.Contains(t, err.Error(), "deezer")
		require.Contains(t, err.Error(), "not support")
	})

	t.Run("ServiceNeedsBootstrapError", func(t *testing.T) {
		err := &ServiceNeedsBootstrapError{Service: "spotify"}
		require.Contains(t, err.Error(), "spotify")
		require.Contains(t, err.Error(), "credentials")
		require.Contains(t, err.Error(), "favorites")
	})

	t.Run("ContentUnavailableError", func(t *testing.T) {
		err := &ContentUnavailableError{Reason: "service offline"}
		require.Contains(t, err.Error(), "service offline")
		require.Contains(t, err.Error(), "unavailable")
	})
}

func TestIsServiceSupported(t *testing.T) {
	h := setupTest(t)

	tests := []struct {
		service   string
		supported bool
	}{
		{"spotify", true},
		{"apple_music", true},
		{"amazon_music", false},
		{"deezer", false},
		{"tidal", false},
		{"tunein", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			result := h.resolver.isServiceSupported(tt.service)
			require.Equal(t, tt.supported, result)
		})
	}
}

func TestDetermineContentType(t *testing.T) {
	h := setupTest(t)

	tests := []struct {
		upnpClass   string
		contentType string
	}{
		{"object.item.audioItem.audioBroadcast", "station"},
		{"object.container.playlistContainer", "playlist"},
		{"object.container.album.musicAlbum", "album"},
		{"object.item.audioItem.musicTrack", "track"},
		{"object.container.unknown", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.upnpClass, func(t *testing.T) {
			result := h.resolver.determineContentType(tt.upnpClass)
			require.Equal(t, tt.contentType, result)
		})
	}
}

func TestBuildDidlMetadata(t *testing.T) {
	metadata := buildDidlMetadata("item123", "Test Title", "object.container.playlistContainer", "SA_RINCON12345")

	require.Contains(t, metadata, "DIDL-Lite")
	require.Contains(t, metadata, "item123")
	require.Contains(t, metadata, "Test Title")
	require.Contains(t, metadata, "object.container.playlistContainer")
	require.Contains(t, metadata, "SA_RINCON12345")
	require.Contains(t, metadata, "xmlns:dc")
	require.Contains(t, metadata, "xmlns:upnp")
}

func TestBuildDidlMetadata_EmptyTitle(t *testing.T) {
	metadata := buildDidlMetadata("item123", "", "object.container.playlistContainer", "SA_RINCON12345")

	require.Contains(t, metadata, "Unknown")
}

func TestConstants(t *testing.T) {
	// Test service ID constants
	require.Equal(t, "12", SIDSpotify)
	require.Equal(t, "204", SIDAppleMusic)
	require.Equal(t, "201", SIDAmazonMusic)

	// Test service name constants
	require.Equal(t, "spotify", ServiceSpotify)
	require.Equal(t, "apple_music", ServiceAppleMusic)
	require.Equal(t, "amazon_music", ServiceAmazonMusic)

	// Test status constants
	require.Equal(t, "ready", StatusReady)
	require.Equal(t, "needs_bootstrap", StatusNeedsBootstrap)
	require.Equal(t, "not_supported", StatusNotSupported)
}

func TestServiceMaps(t *testing.T) {
	// Test service logos
	require.Contains(t, serviceLogos[ServiceSpotify], "spotify")
	require.Contains(t, serviceLogos[ServiceAppleMusic], "apple")
	require.Contains(t, serviceLogos[ServiceAmazonMusic], "amazon")

	// Test display names
	require.Equal(t, "Spotify", serviceDisplayNames[ServiceSpotify])
	require.Equal(t, "Apple Music", serviceDisplayNames[ServiceAppleMusic])
	require.Equal(t, "Amazon Music", serviceDisplayNames[ServiceAmazonMusic])

	// Test supported content types
	require.Contains(t, serviceSupportedContentTypes[ServiceSpotify], "track")
	require.Contains(t, serviceSupportedContentTypes[ServiceSpotify], "playlist")
	require.Contains(t, serviceSupportedContentTypes[ServiceAppleMusic], "album")
	require.Empty(t, serviceSupportedContentTypes[ServiceAmazonMusic]) // Amazon not supported
}

func TestCredentialExtractor_HasSID(t *testing.T) {
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	soapClient := soap.NewClient(5 * time.Second)
	extractor := NewCredentialExtractor(soapClient, 5*time.Second, logger)

	tests := []struct {
		name        string
		uri         string
		expectedSID string
		expected    bool
	}{
		{
			name:        "matching sid",
			uri:         "x-rincon-cpcontainer:abc?sid=12&sn=1",
			expectedSID: "12",
			expected:    true,
		},
		{
			name:        "non-matching sid",
			uri:         "x-rincon-cpcontainer:abc?sid=12&sn=1",
			expectedSID: "204",
			expected:    false,
		},
		{
			name:        "no sid in uri",
			uri:         "x-rincon-cpcontainer:abc",
			expectedSID: "12",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.hasSID(tt.uri, tt.expectedSID)
			require.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
func strPtr(s string) *string {
	return &s
}
