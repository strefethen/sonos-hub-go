package sonos

import (
	"context"
	"testing"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

func TestExtractFromItem_SpotifyCredentials(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	tests := []struct {
		name     string
		item     soap.FavoriteItem
		wantSvc  string
		wantSID  string
		wantSN   string
		wantTok  string
		wantSess string
	}{
		{
			name: "Spotify favorite with full credentials",
			item: soap.FavoriteItem{
				ID:               "FV:2/123",
				Title:            "My Spotify Playlist",
				Resource:         "x-rincon-cpcontainer:1006206cspotify%3aplaylist%3a37i9dQZF1DXcBWIGoYBM5M?sid=12&flags=8300&sn=5",
				ResourceMetaData: "SA_RINCON2311_X_#Svc2311-6a54dae0-Token",
			},
			wantSvc:  ServiceSpotify,
			wantSID:  "12",
			wantSN:   "5",
			wantTok:  "2311",
			wantSess: "6a54dae0",
		},
		{
			name: "Spotify favorite detected by URI content",
			item: soap.FavoriteItem{
				ID:       "FV:2/124",
				Title:    "Another Playlist",
				Resource: "x-rincon-cpcontainer:1006206cspotify%3aplaylist%3aabcdef?sid=12&flags=8300&sn=10",
			},
			wantSvc: ServiceSpotify,
			wantSID: "12",
			wantSN:  "10",
		},
		{
			name: "Spotify detected by metadata content",
			item: soap.FavoriteItem{
				ID:               "FV:2/125",
				Title:            "Spotify Song",
				Resource:         "x-rincon-cpcontainer:something?sid=12&sn=3",
				ResourceMetaData: "RINCON_spotify_something",
			},
			wantSvc: ServiceSpotify,
			wantSID: "12",
			wantSN:  "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := extractor.extractFromItem(tt.item)

			if tt.wantSvc == "" {
				if creds != nil {
					t.Errorf("expected nil credentials, got %+v", creds)
				}
				return
			}

			if creds == nil {
				t.Fatal("expected credentials, got nil")
			}

			if creds.Service != tt.wantSvc {
				t.Errorf("Service = %q, want %q", creds.Service, tt.wantSvc)
			}
			if creds.SID != tt.wantSID {
				t.Errorf("SID = %q, want %q", creds.SID, tt.wantSID)
			}
			if creds.SN != tt.wantSN {
				t.Errorf("SN = %q, want %q", creds.SN, tt.wantSN)
			}
			if tt.wantTok != "" && creds.Token != tt.wantTok {
				t.Errorf("Token = %q, want %q", creds.Token, tt.wantTok)
			}
			if tt.wantSess != "" && creds.SessionSuffix != tt.wantSess {
				t.Errorf("SessionSuffix = %q, want %q", creds.SessionSuffix, tt.wantSess)
			}
		})
	}
}

func TestExtractFromItem_AppleMusicCredentials(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	tests := []struct {
		name    string
		item    soap.FavoriteItem
		wantSvc string
		wantSID string
	}{
		{
			name: "Apple Music favorite with SID",
			item: soap.FavoriteItem{
				ID:       "FV:2/200",
				Title:    "Apple Music Playlist",
				Resource: "x-rincon-cpcontainer:1006206capple%3aplaylist%3aabc?sid=204&flags=8300&sn=15",
			},
			wantSvc: ServiceAppleMusic,
			wantSID: "204",
		},
		{
			name: "Apple Music detected by metadata sa_rincon52231",
			item: soap.FavoriteItem{
				ID:               "FV:2/201",
				Title:            "My Apple Playlist",
				Resource:         "x-rincon-cpcontainer:something?sid=204&sn=7",
				ResourceMetaData: "SA_RINCON52231_something",
			},
			wantSvc: ServiceAppleMusic,
			wantSID: "204",
		},
		{
			name: "Apple Music detected by URI containing apple",
			item: soap.FavoriteItem{
				ID:       "FV:2/202",
				Title:    "Apple Favorites",
				Resource: "x-sonos-http:song%3aapple-music-track?sid=204&sn=8",
			},
			wantSvc: ServiceAppleMusic,
			wantSID: "204",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := extractor.extractFromItem(tt.item)

			if tt.wantSvc == "" {
				if creds != nil {
					t.Errorf("expected nil credentials, got %+v", creds)
				}
				return
			}

			if creds == nil {
				t.Fatal("expected credentials, got nil")
			}

			if creds.Service != tt.wantSvc {
				t.Errorf("Service = %q, want %q", creds.Service, tt.wantSvc)
			}
			if creds.SID != tt.wantSID {
				t.Errorf("SID = %q, want %q", creds.SID, tt.wantSID)
			}
		})
	}
}

func TestExtractFromItem_AmazonMusicCredentials(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	tests := []struct {
		name    string
		item    soap.FavoriteItem
		wantSvc string
		wantSID string
	}{
		{
			name: "Amazon Music favorite with SID",
			item: soap.FavoriteItem{
				ID:       "FV:2/300",
				Title:    "Amazon Music Playlist",
				Resource: "x-rincon-cpcontainer:1006206camazon%3aplaylist%3aabc?sid=201&flags=8300&sn=20",
			},
			wantSvc: ServiceAmazonMusic,
			wantSID: "201",
		},
		{
			name: "Amazon Music detected by amzn in URI",
			item: soap.FavoriteItem{
				ID:       "FV:2/301",
				Title:    "Prime Playlist",
				Resource: "x-rincon-cpcontainer:amzn%3aplaylist%3axyz?sid=201&sn=21",
			},
			wantSvc: ServiceAmazonMusic,
			wantSID: "201",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := extractor.extractFromItem(tt.item)

			if tt.wantSvc == "" {
				if creds != nil {
					t.Errorf("expected nil credentials, got %+v", creds)
				}
				return
			}

			if creds == nil {
				t.Fatal("expected credentials, got nil")
			}

			if creds.Service != tt.wantSvc {
				t.Errorf("Service = %q, want %q", creds.Service, tt.wantSvc)
			}
			if creds.SID != tt.wantSID {
				t.Errorf("SID = %q, want %q", creds.SID, tt.wantSID)
			}
		})
	}
}

func TestExtractFromItem_NonServiceFavorite(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	// Test items that should not match any service
	tests := []struct {
		name string
		item soap.FavoriteItem
	}{
		{
			name: "TuneIn radio station",
			item: soap.FavoriteItem{
				ID:       "FV:2/400",
				Title:    "NPR News",
				Resource: "x-sonosapi-stream:s12345?sid=254&flags=8224&sn=0",
			},
		},
		{
			name: "Local music",
			item: soap.FavoriteItem{
				ID:       "FV:2/401",
				Title:    "My Local Song",
				Resource: "x-file-cifs://server/music/song.mp3",
			},
		},
		{
			name: "Empty resource",
			item: soap.FavoriteItem{
				ID:    "FV:2/402",
				Title: "Empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := extractor.extractFromItem(tt.item)
			if creds != nil {
				t.Errorf("expected nil credentials for non-service favorite, got %+v", creds)
			}
		})
	}
}

func TestDetectServiceFromItem(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	tests := []struct {
		name    string
		item    soap.FavoriteItem
		wantSvc string
	}{
		{
			name: "Spotify by URI keyword",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:spotify%3aplaylist%3aabc",
			},
			wantSvc: ServiceSpotify,
		},
		{
			name: "Spotify by SID",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:something?sid=12&sn=1",
			},
			wantSvc: ServiceSpotify,
		},
		{
			name: "Apple Music by URI keyword",
			item: soap.FavoriteItem{
				Resource: "x-sonos-http:apple%3aplaylist",
			},
			wantSvc: ServiceAppleMusic,
		},
		{
			name: "Apple Music by sa_rincon52231 in metadata",
			item: soap.FavoriteItem{
				Resource:         "x-rincon-cpcontainer:something",
				ResourceMetaData: "SA_RINCON52231_something",
			},
			wantSvc: ServiceAppleMusic,
		},
		{
			name: "Apple Music by SID",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:something?sid=204&sn=1",
			},
			wantSvc: ServiceAppleMusic,
		},
		{
			name: "Amazon Music by URI keyword",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:amazon%3aplaylist",
			},
			wantSvc: ServiceAmazonMusic,
		},
		{
			name: "Amazon Music by amzn keyword",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:amzn%3aplaylist",
			},
			wantSvc: ServiceAmazonMusic,
		},
		{
			name: "Amazon Music by SID",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:something?sid=201&sn=1",
			},
			wantSvc: ServiceAmazonMusic,
		},
		{
			name: "Unknown service",
			item: soap.FavoriteItem{
				Resource: "x-rincon-cpcontainer:unknown?sid=999&sn=1",
			},
			wantSvc: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractor.detectServiceFromItem(tt.item)
			if got != tt.wantSvc {
				t.Errorf("detectServiceFromItem() = %q, want %q", got, tt.wantSvc)
			}
		})
	}
}

func TestGetServiceStatus(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	// Amazon Music should always be not supported
	status := extractor.GetServiceStatus(ServiceAmazonMusic)
	if status != StatusNotSupported {
		t.Errorf("GetServiceStatus(amazon_music) = %q, want %q", status, StatusNotSupported)
	}

	// Unknown service should need bootstrap
	status = extractor.GetServiceStatus(ServiceSpotify)
	if status != StatusNeedsBootstrap {
		t.Errorf("GetServiceStatus(spotify) without cache = %q, want %q", status, StatusNeedsBootstrap)
	}

	// Cache some credentials
	extractor.cacheCredentials(ServiceSpotify, &ServiceCredentials{
		Service: ServiceSpotify,
		SID:     "12",
		SN:      "5",
	})

	// Now it should be ready
	status = extractor.GetServiceStatus(ServiceSpotify)
	if status != StatusReady {
		t.Errorf("GetServiceStatus(spotify) with cache = %q, want %q", status, StatusReady)
	}
}

func TestCacheBehavior(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)
	extractor.SetCacheTTL(100 * time.Millisecond)

	// Cache credentials
	creds := &ServiceCredentials{
		Service: ServiceSpotify,
		SID:     "12",
		SN:      "5",
	}
	extractor.cacheCredentials(ServiceSpotify, creds)

	// Should be valid initially
	if !extractor.IsCacheValid(ServiceSpotify) {
		t.Error("Cache should be valid immediately after caching")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be invalid after TTL
	if extractor.IsCacheValid(ServiceSpotify) {
		t.Error("Cache should be invalid after TTL expires")
	}

	// Status should reflect expired cache
	status := extractor.GetServiceStatus(ServiceSpotify)
	if status != StatusNeedsBootstrap {
		t.Errorf("GetServiceStatus after TTL = %q, want %q", status, StatusNeedsBootstrap)
	}
}

func TestClearCache(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	// Cache credentials for multiple services
	extractor.cacheCredentials(ServiceSpotify, &ServiceCredentials{Service: ServiceSpotify})
	extractor.cacheCredentials(ServiceAppleMusic, &ServiceCredentials{Service: ServiceAppleMusic})

	// Verify cached
	if !extractor.IsCacheValid(ServiceSpotify) {
		t.Error("Spotify should be cached")
	}
	if !extractor.IsCacheValid(ServiceAppleMusic) {
		t.Error("Apple Music should be cached")
	}

	// Clear cache
	extractor.ClearCache()

	// Verify cleared
	if extractor.IsCacheValid(ServiceSpotify) {
		t.Error("Spotify should not be cached after clear")
	}
	if extractor.IsCacheValid(ServiceAppleMusic) {
		t.Error("Apple Music should not be cached after clear")
	}
}

func TestRegexPatterns(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		wantVal  string
		wantFind bool
	}{
		{
			name:     "SID pattern match",
			pattern:  "sid",
			input:    "x-rincon-cpcontainer:something?sid=12&flags=8300&sn=5",
			wantVal:  "12",
			wantFind: true,
		},
		{
			name:     "SID pattern no match",
			pattern:  "sid",
			input:    "x-rincon-cpcontainer:something?flags=8300",
			wantFind: false,
		},
		{
			name:     "SN pattern match",
			pattern:  "sn",
			input:    "x-rincon-cpcontainer:something?sid=12&sn=5",
			wantVal:  "5",
			wantFind: true,
		},
		{
			name:     "Token pattern match",
			pattern:  "token",
			input:    "SA_RINCON2311_X_#Svc2311-6a54dae0-Token",
			wantVal:  "2311",
			wantFind: true,
		},
		{
			name:     "Token pattern for Apple Music",
			pattern:  "token",
			input:    "SA_RINCON52231_X_#Svc52231-abcdef12-Token",
			wantVal:  "52231",
			wantFind: true,
		},
		{
			name:     "Session pattern match",
			pattern:  "session",
			input:    "SA_RINCON2311_X_#Svc2311-6a54dae0-Token",
			wantVal:  "6a54dae0",
			wantFind: true,
		},
		{
			name:     "Session pattern longer hex",
			pattern:  "session",
			input:    "#Svc12345-abcdef0123456789-Token",
			wantVal:  "abcdef0123456789",
			wantFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var matches []string
			switch tt.pattern {
			case "sid":
				matches = sidPattern.FindStringSubmatch(tt.input)
			case "sn":
				matches = snPattern.FindStringSubmatch(tt.input)
			case "token":
				matches = tokenPattern.FindStringSubmatch(tt.input)
			case "session":
				matches = sessionPattern.FindStringSubmatch(tt.input)
			}

			found := len(matches) > 1

			if found != tt.wantFind {
				t.Errorf("pattern match = %v, want %v", found, tt.wantFind)
			}

			if found && matches[1] != tt.wantVal {
				t.Errorf("captured value = %q, want %q", matches[1], tt.wantVal)
			}
		})
	}
}

func TestGetAllServiceStatuses(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)
	ctx := context.Background()

	// Test GetAllServiceStatuses returns proper structure
	// Note: This will return needs_bootstrap for Spotify/Apple since we don't have a real SOAP client
	// and not_supported for Amazon Music
	statuses := extractor.GetAllServiceStatuses(ctx, "192.168.1.100")

	// Check that all known services are present
	expectedServices := []string{ServiceSpotify, ServiceAppleMusic, ServiceAmazonMusic}
	for _, svc := range expectedServices {
		status, ok := statuses[svc]
		if !ok {
			t.Errorf("missing status for service %q", svc)
			continue
		}

		// Verify display name
		if status.DisplayName != serviceDisplayNames[svc] {
			t.Errorf("DisplayName for %s = %q, want %q", svc, status.DisplayName, serviceDisplayNames[svc])
		}

		// Verify logo URL
		if status.LogoURL != serviceLogos[svc] {
			t.Errorf("LogoURL for %s = %q, want %q", svc, status.LogoURL, serviceLogos[svc])
		}

		// Amazon should be not_supported
		if svc == ServiceAmazonMusic && status.Status != StatusNotSupported {
			t.Errorf("Amazon Music status = %q, want %q", status.Status, StatusNotSupported)
		}

		// Others should need bootstrap (no credentials cached)
		if svc != ServiceAmazonMusic && status.Status != StatusNeedsBootstrap {
			t.Errorf("%s status = %q, want %q", svc, status.Status, StatusNeedsBootstrap)
		}
	}
}

func TestHasSID(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	tests := []struct {
		uri      string
		sid      string
		expected bool
	}{
		{"x-rincon-cpcontainer:something?sid=12&sn=5", "12", true},
		{"x-rincon-cpcontainer:something?sid=12&sn=5", "204", false},
		{"x-rincon-cpcontainer:something?sid=204&sn=5", "204", true},
		{"x-rincon-cpcontainer:something?sn=5", "12", false},
		{"x-rincon-cpcontainer:something", "12", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := extractor.hasSID(tt.uri, tt.sid)
			if got != tt.expected {
				t.Errorf("hasSID(%q, %q) = %v, want %v", tt.uri, tt.sid, got, tt.expected)
			}
		})
	}
}

func TestServiceCredentialsExtraction_FullDIDLExample(t *testing.T) {
	extractor := NewCredentialExtractor(nil, 5*time.Second, nil)

	// Test with a realistic DIDL-Lite style metadata
	item := soap.FavoriteItem{
		ID:       "FV:2/42",
		Title:    "Today's Top Hits",
		Resource: "x-rincon-cpcontainer:1006206cspotify%3aplaylist%3a37i9dQZF1DXcBWIGoYBM5M?sid=12&flags=8300&sn=7",
		ResourceMetaData: `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="1006206cspotify%3aplaylist%3a37i9dQZF1DXcBWIGoYBM5M" parentID="0" restricted="true"><dc:title>Today&apos;s Top Hits</dc:title><upnp:class>object.container.playlistContainer</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON2311_X_#Svc2311-abc123de-Token</desc></item></DIDL-Lite>`,
	}

	creds := extractor.extractFromItem(item)
	if creds == nil {
		t.Fatal("expected credentials, got nil")
	}

	if creds.Service != ServiceSpotify {
		t.Errorf("Service = %q, want %q", creds.Service, ServiceSpotify)
	}
	if creds.SID != "12" {
		t.Errorf("SID = %q, want %q", creds.SID, "12")
	}
	if creds.SN != "7" {
		t.Errorf("SN = %q, want %q", creds.SN, "7")
	}
	if creds.Token != "2311" {
		t.Errorf("Token = %q, want %q", creds.Token, "2311")
	}
	if creds.SessionSuffix != "abc123de" {
		t.Errorf("SessionSuffix = %q, want %q", creds.SessionSuffix, "abc123de")
	}
}

func TestServiceConstants(t *testing.T) {
	// Verify service ID constants match expected values
	if SIDSpotify != "12" {
		t.Errorf("SIDSpotify = %q, want %q", SIDSpotify, "12")
	}
	if SIDAppleMusic != "204" {
		t.Errorf("SIDAppleMusic = %q, want %q", SIDAppleMusic, "204")
	}
	if SIDAmazonMusic != "201" {
		t.Errorf("SIDAmazonMusic = %q, want %q", SIDAmazonMusic, "201")
	}

	// Verify service name constants
	if ServiceSpotify != "spotify" {
		t.Errorf("ServiceSpotify = %q, want %q", ServiceSpotify, "spotify")
	}
	if ServiceAppleMusic != "apple_music" {
		t.Errorf("ServiceAppleMusic = %q, want %q", ServiceAppleMusic, "apple_music")
	}
	if ServiceAmazonMusic != "amazon_music" {
		t.Errorf("ServiceAmazonMusic = %q, want %q", ServiceAmazonMusic, "amazon_music")
	}

	// Verify status constants
	if StatusReady != "ready" {
		t.Errorf("StatusReady = %q, want %q", StatusReady, "ready")
	}
	if StatusNeedsBootstrap != "needs_bootstrap" {
		t.Errorf("StatusNeedsBootstrap = %q, want %q", StatusNeedsBootstrap, "needs_bootstrap")
	}
	if StatusNotSupported != "not_supported" {
		t.Errorf("StatusNotSupported = %q, want %q", StatusNotSupported, "not_supported")
	}
}
