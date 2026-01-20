package sonos

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewMusicURIBuilder(t *testing.T) {
	builder := NewMusicURIBuilder()
	require.NotNil(t, builder)
}

// Test service configuration lookups
func TestGetServiceConfig(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify service exists", func(t *testing.T) {
		config, ok := builder.GetServiceConfig("spotify")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, 12, config.SID)
		require.Equal(t, "SA_RINCON", config.TokenPrefix)
	})

	t.Run("apple_music service exists", func(t *testing.T) {
		config, ok := builder.GetServiceConfig("apple_music")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, 204, config.SID)
		require.Equal(t, "SA_RINCON", config.TokenPrefix)
	})

	t.Run("case insensitive lookup", func(t *testing.T) {
		config, ok := builder.GetServiceConfig("SPOTIFY")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, 12, config.SID)
	})

	t.Run("handles whitespace", func(t *testing.T) {
		config, ok := builder.GetServiceConfig("  spotify  ")
		require.True(t, ok)
		require.NotNil(t, config)
	})

	t.Run("unknown service returns false", func(t *testing.T) {
		config, ok := builder.GetServiceConfig("unknown_service")
		require.False(t, ok)
		require.Nil(t, config)
	})
}

func TestGetContentTypeConfig(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify playlist config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "playlist")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-rincon-cpcontainer", config.URIScheme)
		require.Equal(t, "1006206c", config.ItemIDPrefix)
		require.Equal(t, "spotify:playlist:", config.IDPrefix)
		require.Equal(t, 8300, config.DefaultFlags)
	})

	t.Run("spotify album config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "album")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-rincon-cpcontainer", config.URIScheme)
		require.Equal(t, "1004006c", config.ItemIDPrefix)
		require.Equal(t, "spotify:album:", config.IDPrefix)
		require.Equal(t, 108, config.DefaultFlags)
	})

	t.Run("spotify station config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "station")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-sonosapi-radio", config.URIScheme)
		require.Equal(t, "100c206c", config.ItemIDPrefix)
		require.Equal(t, "spotify:artistRadio:", config.IDPrefix)
		require.Equal(t, 8300, config.DefaultFlags)
	})

	t.Run("spotify track config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "track")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-sonos-http", config.URIScheme)
		require.Equal(t, "00032020", config.ItemIDPrefix)
		require.Equal(t, "spotify:track:", config.IDPrefix)
		require.Equal(t, 8224, config.DefaultFlags)
	})

	t.Run("apple_music track config with suffix", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("apple_music", "track")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-sonos-http", config.URIScheme)
		require.Equal(t, "10032028", config.ItemIDPrefix)
		require.Equal(t, "song:", config.IDPrefix)
		require.Equal(t, ".mp4", config.IDSuffix)
		require.Equal(t, 8232, config.DefaultFlags)
	})

	t.Run("apple_music playlist config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("apple_music", "playlist")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-rincon-cpcontainer", config.URIScheme)
		require.Equal(t, "1006206c", config.ItemIDPrefix)
		require.Equal(t, "playlist:", config.IDPrefix)
		require.Equal(t, 8300, config.DefaultFlags)
	})

	t.Run("apple_music album config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("apple_music", "album")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-rincon-cpcontainer", config.URIScheme)
		require.Equal(t, "1004206c", config.ItemIDPrefix)
		require.Equal(t, "libraryalbum:l.", config.IDPrefix)
		require.Equal(t, 8300, config.DefaultFlags)
	})

	t.Run("apple_music station config", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("apple_music", "station")
		require.True(t, ok)
		require.NotNil(t, config)
		require.Equal(t, "x-sonosapi-radio", config.URIScheme)
		require.Equal(t, "100c706c", config.ItemIDPrefix)
		require.Equal(t, "radio:", config.IDPrefix)
		require.Equal(t, 28780, config.DefaultFlags)
	})

	t.Run("case insensitive content type", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "PLAYLIST")
		require.True(t, ok)
		require.NotNil(t, config)
	})

	t.Run("unknown content type returns false", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "unknown")
		require.False(t, ok)
		require.Nil(t, config)
	})

	t.Run("unknown service returns false", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("unknown", "playlist")
		require.False(t, ok)
		require.Nil(t, config)
	})
}

func TestMusicURIBuilderIsServiceSupported(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify is supported", func(t *testing.T) {
		require.True(t, builder.IsServiceSupported("spotify"))
	})

	t.Run("apple_music is supported", func(t *testing.T) {
		require.True(t, builder.IsServiceSupported("apple_music"))
	})

	t.Run("unknown service is not supported", func(t *testing.T) {
		require.False(t, builder.IsServiceSupported("pandora"))
	})
}

func TestGetSupportedContentTypes(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify content types", func(t *testing.T) {
		types := builder.GetSupportedContentTypes("spotify")
		require.Len(t, types, 6)
		require.Contains(t, types, "playlist")
		require.Contains(t, types, "album")
		require.Contains(t, types, "station")
		require.Contains(t, types, "track")
		require.Contains(t, types, "podcast")
		require.Contains(t, types, "episode")
	})

	t.Run("apple_music content types", func(t *testing.T) {
		types := builder.GetSupportedContentTypes("apple_music")
		require.Len(t, types, 4)
		require.Contains(t, types, "playlist")
		require.Contains(t, types, "album")
		require.Contains(t, types, "station")
		require.Contains(t, types, "track")
	})

	t.Run("unknown service returns nil", func(t *testing.T) {
		types := builder.GetSupportedContentTypes("unknown")
		require.Nil(t, types)
	})
}

func TestGetSupportedServices(t *testing.T) {
	builder := NewMusicURIBuilder()

	services := builder.GetSupportedServices()
	require.Len(t, services, 2)
	require.Contains(t, services, "spotify")
	require.Contains(t, services, "apple_music")
}

// Test individual URI building methods
func TestBuildContainerURI(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("builds container URI correctly", func(t *testing.T) {
		uri := builder.BuildContainerURI("1006206cspotify%3aplaylist%3a37i9dQZF1DXcBWIGoYBM5M", 12, 5, 8300)
		require.Equal(t, "x-rincon-cpcontainer:1006206cspotify%3aplaylist%3a37i9dQZF1DXcBWIGoYBM5M?sid=12&flags=8300&sn=5", uri)
	})

	t.Run("different flags", func(t *testing.T) {
		uri := builder.BuildContainerURI("testitem", 204, 3, 108)
		require.Equal(t, "x-rincon-cpcontainer:testitem?sid=204&flags=108&sn=3", uri)
	})
}

func TestBuildStreamURI(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("builds stream URI correctly", func(t *testing.T) {
		uri := builder.BuildStreamURI("10032028song%3a1234567890.mp4", 204, 3, 8232)
		require.Equal(t, "x-sonos-http:10032028song%3a1234567890.mp4?sid=204&flags=8232&sn=3", uri)
	})

	t.Run("spotify track", func(t *testing.T) {
		uri := builder.BuildStreamURI("00032020spotify%3atrack%3aabc123", 12, 5, 8224)
		require.Equal(t, "x-sonos-http:00032020spotify%3atrack%3aabc123?sid=12&flags=8224&sn=5", uri)
	})
}

func TestBuildRadioURI(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("builds radio URI correctly", func(t *testing.T) {
		uri := builder.BuildRadioURI("100c206cspotify%3aartistRadio%3a123", 12, 5, 8300)
		require.Equal(t, "x-sonosapi-radio:100c206cspotify%3aartistRadio%3a123?sid=12&flags=8300&sn=5", uri)
	})

	t.Run("apple music station", func(t *testing.T) {
		uri := builder.BuildRadioURI("100c706cradio%3astation123", 204, 3, 28780)
		require.Equal(t, "x-sonosapi-radio:100c706cradio%3astation123?sid=204&flags=28780&sn=3", uri)
	})
}

// Test full URI building with BuildURI
func TestBuildURI(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify playlist without credentials", func(t *testing.T) {
		uri, metadata, err := builder.BuildURI("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Today's Top Hits", nil)
		require.NoError(t, err)
		require.NotEmpty(t, uri)
		require.NotEmpty(t, metadata)

		// Check URI structure
		require.True(t, strings.HasPrefix(uri, "x-rincon-cpcontainer:"))
		require.Contains(t, uri, "sid=12")
		require.Contains(t, uri, "flags=8300")
		require.Contains(t, uri, "sn=1")

		// Check that playlist ID is URL encoded
		require.Contains(t, uri, url.QueryEscape("spotify:playlist:37i9dQZF1DXcBWIGoYBM5M"))
	})

	t.Run("spotify playlist with credentials", func(t *testing.T) {
		creds := &MusicServiceCredentials{
			SID:           12,
			SN:            5,
			Token:         12345,
			SessionSuffix: "abc123",
			ExtractedAt:   time.Now(),
		}

		uri, metadata, err := builder.BuildURI("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Today's Top Hits", creds)
		require.NoError(t, err)
		require.NotEmpty(t, uri)
		require.NotEmpty(t, metadata)

		// Check credentials are used
		require.Contains(t, uri, "sid=12")
		require.Contains(t, uri, "sn=5")
	})

	t.Run("spotify album", func(t *testing.T) {
		uri, _, err := builder.BuildURI("spotify", "album", "1HrMmB5useeZ0F5lHrMvl0", "Thriller", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-rincon-cpcontainer:"))
		require.Contains(t, uri, "1004006c")
		require.Contains(t, uri, "flags=108")
	})

	t.Run("spotify station", func(t *testing.T) {
		uri, _, err := builder.BuildURI("spotify", "station", "abc123", "Artist Radio", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-sonosapi-radio:"))
		require.Contains(t, uri, "100c206c")
		require.Contains(t, uri, "flags=8300")
	})

	t.Run("spotify track", func(t *testing.T) {
		uri, _, err := builder.BuildURI("spotify", "track", "6rqhFgbbKwnb9MLmUQDhG6", "Billie Jean", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-sonos-http:"))
		require.Contains(t, uri, "00032020")
		require.Contains(t, uri, "flags=8224")
	})

	t.Run("apple_music track", func(t *testing.T) {
		uri, _, err := builder.BuildURI("apple_music", "track", "1234567890", "Song Name", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-sonos-http:"))
		require.Contains(t, uri, "10032028")
		require.Contains(t, uri, "sid=204")
		require.Contains(t, uri, "flags=8232")
		// Check .mp4 suffix is included
		require.Contains(t, uri, url.QueryEscape("song:1234567890.mp4"))
	})

	t.Run("apple_music playlist", func(t *testing.T) {
		uri, _, err := builder.BuildURI("apple_music", "playlist", "pl.abc123", "My Playlist", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-rincon-cpcontainer:"))
		require.Contains(t, uri, "1006206c")
		require.Contains(t, uri, "sid=204")
	})

	t.Run("apple_music album", func(t *testing.T) {
		uri, _, err := builder.BuildURI("apple_music", "album", "album123", "Album Name", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-rincon-cpcontainer:"))
		require.Contains(t, uri, "1004206c")
		// Check libraryalbum prefix is used
		require.Contains(t, uri, url.QueryEscape("libraryalbum:l.album123"))
	})

	t.Run("apple_music station", func(t *testing.T) {
		uri, _, err := builder.BuildURI("apple_music", "station", "station123", "Radio Station", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-sonosapi-radio:"))
		require.Contains(t, uri, "100c706c")
		require.Contains(t, uri, "flags=28780")
	})

	t.Run("unsupported service returns error", func(t *testing.T) {
		_, _, err := builder.BuildURI("pandora", "playlist", "123", "Test", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported service")
	})

	t.Run("spotify podcast", func(t *testing.T) {
		uri, _, err := builder.BuildURI("spotify", "podcast", "2mTUnDkuKUkhiueKcVWoP0", "Up First from NPR", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-rincon-cpcontainer:"))
		require.Contains(t, uri, "1006206c")
		require.Contains(t, uri, "spotify%3Ashow%3A")
		require.Contains(t, uri, "flags=8300")
	})

	t.Run("spotify episode", func(t *testing.T) {
		uri, _, err := builder.BuildURI("spotify", "episode", "4VptiCRV73h8c9cLLQmLhW", "Episode Title", nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(uri, "x-sonos-http:"))
		require.Contains(t, uri, "00032020")
		require.Contains(t, uri, "spotify%3Aepisode%3A")
		require.Contains(t, uri, "flags=8224")
	})

	t.Run("unsupported content type returns error", func(t *testing.T) {
		_, _, err := builder.BuildURI("spotify", "audiobook", "123", "Test", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported content type")
	})
}

// Test DIDL metadata building
func TestBuildDIDLMetadata(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify playlist metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Today's Top Hits", nil)
		require.NotEmpty(t, metadata)

		// Check XML structure
		require.Contains(t, metadata, "<DIDL-Lite")
		require.Contains(t, metadata, "</DIDL-Lite>")
		require.Contains(t, metadata, "<item")
		require.Contains(t, metadata, "</item>")
		// Apostrophe is XML-escaped to &apos;
		require.Contains(t, metadata, "<dc:title>Today&apos;s Top Hits</dc:title>")
		require.Contains(t, metadata, "<upnp:class>object.container.playlistContainer</upnp:class>")
		require.Contains(t, metadata, "SA_RINCON12")
		require.Contains(t, metadata, `id="cdudn"`)
	})

	t.Run("spotify album metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "album", "abc123", "My Album", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "<upnp:class>object.container.album.musicAlbum</upnp:class>")
	})

	t.Run("spotify track metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "track", "abc123", "My Song", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "<upnp:class>object.item.audioItem.musicTrack</upnp:class>")
	})

	t.Run("spotify station metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "station", "abc123", "Artist Radio", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "<upnp:class>object.item.audioItem.audioBroadcast</upnp:class>")
	})

	t.Run("spotify podcast metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "podcast", "2mTUnDkuKUkhiueKcVWoP0", "Up First from NPR", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "<upnp:class>object.container.playlistContainer</upnp:class>")
		require.Contains(t, metadata, "1006206cspotify%3Ashow%3A")
	})

	t.Run("spotify episode metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "episode", "4VptiCRV73h8c9cLLQmLhW", "Episode Title", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "<upnp:class>object.item.audioItem.musicTrack</upnp:class>")
		require.Contains(t, metadata, "00032020spotify%3Aepisode%3A")
	})

	t.Run("metadata with credentials", func(t *testing.T) {
		creds := &MusicServiceCredentials{
			SID:           12,
			SN:            5,
			Token:         12345,
			SessionSuffix: "abc123",
			ExtractedAt:   time.Now(),
		}

		metadata := builder.BuildDIDLMetadata("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Test", creds)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "SA_RINCON12_5_X_#Svc12-abc123-Token")
	})

	t.Run("metadata escapes XML special characters", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "playlist", "123", "Rock & Roll <Classics>", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "Rock &amp; Roll &lt;Classics&gt;")
		require.NotContains(t, metadata, "Rock & Roll <Classics>")
	})

	t.Run("apple_music track metadata", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("apple_music", "track", "123", "Test Song", nil)
		require.NotEmpty(t, metadata)
		require.Contains(t, metadata, "SA_RINCON204")
		require.Contains(t, metadata, "<upnp:class>object.item.audioItem.musicTrack</upnp:class>")
	})

	t.Run("unknown service returns empty string", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("unknown", "playlist", "123", "Test", nil)
		require.Empty(t, metadata)
	})

	t.Run("unknown content type returns empty string", func(t *testing.T) {
		metadata := builder.BuildDIDLMetadata("spotify", "unknown", "123", "Test", nil)
		require.Empty(t, metadata)
	})
}

// Test URL encoding of content IDs
func TestURLEncodingInURIs(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("encodes special characters in content ID", func(t *testing.T) {
		// The content ID itself gets URL encoded
		uri, _, err := builder.BuildURI("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Test", nil)
		require.NoError(t, err)

		// spotify:playlist:37i9dQZF1DXcBWIGoYBM5M should be URL encoded
		// Colons become %3a
		require.Contains(t, uri, "spotify%3Aplaylist%3A37i9dQZF1DXcBWIGoYBM5M")
	})

	t.Run("handles content IDs with special chars", func(t *testing.T) {
		// Test with a content ID that has special characters
		uri, _, err := builder.BuildURI("apple_music", "playlist", "pl.test/playlist", "Test", nil)
		require.NoError(t, err)

		// The slash should be encoded
		require.Contains(t, uri, url.QueryEscape("playlist:pl.test/playlist"))
	})
}

// Test MusicServiceCredentials struct
func TestMusicServiceCredentials(t *testing.T) {
	t.Run("creates credentials with all fields", func(t *testing.T) {
		now := time.Now()
		creds := MusicServiceCredentials{
			SID:           12,
			SN:            5,
			Token:         12345,
			SessionSuffix: "abc123",
			ExtractedAt:   now,
		}

		require.Equal(t, 12, creds.SID)
		require.Equal(t, 5, creds.SN)
		require.Equal(t, 12345, creds.Token)
		require.Equal(t, "abc123", creds.SessionSuffix)
		require.Equal(t, now, creds.ExtractedAt)
	})
}

// Test MusicContentTypeConfig struct
func TestMusicContentTypeConfigFields(t *testing.T) {
	builder := NewMusicURIBuilder()

	t.Run("spotify playlist has all required fields", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("spotify", "playlist")
		require.True(t, ok)
		require.NotEmpty(t, config.URIScheme)
		require.NotEmpty(t, config.ItemIDPrefix)
		require.NotEmpty(t, config.IDPrefix)
		require.NotZero(t, config.DefaultFlags)
		require.NotEmpty(t, config.UpnpClass)
	})

	t.Run("apple_music track has suffix field", func(t *testing.T) {
		config, ok := builder.GetContentTypeConfig("apple_music", "track")
		require.True(t, ok)
		require.NotEmpty(t, config.IDSuffix)
		require.Equal(t, ".mp4", config.IDSuffix)
	})
}

// Benchmark tests
func BenchmarkBuildURI(b *testing.B) {
	builder := NewMusicURIBuilder()
	creds := &MusicServiceCredentials{
		SID:           12,
		SN:            5,
		Token:         12345,
		SessionSuffix: "abc123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = builder.BuildURI("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Today's Top Hits", creds)
	}
}

func BenchmarkBuildDIDLMetadata(b *testing.B) {
	builder := NewMusicURIBuilder()
	creds := &MusicServiceCredentials{
		SID:           12,
		SN:            5,
		Token:         12345,
		SessionSuffix: "abc123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = builder.BuildDIDLMetadata("spotify", "playlist", "37i9dQZF1DXcBWIGoYBM5M", "Today's Top Hits", creds)
	}
}
