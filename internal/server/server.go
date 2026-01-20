package server

import (
	"bufio"
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/applemusic"
	"github.com/strefethen/sonos-hub-go/internal/audit"
	"github.com/strefethen/sonos-hub-go/internal/auth"
	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/db"
	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/music"
	"github.com/strefethen/sonos-hub-go/internal/openapi"
	"github.com/strefethen/sonos-hub-go/internal/scene"
	"github.com/strefethen/sonos-hub-go/internal/scheduler"
	"github.com/strefethen/sonos-hub-go/internal/settings"
	"github.com/strefethen/sonos-hub-go/internal/sonos"
	"github.com/strefethen/sonos-hub-go/internal/sonos/events"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
	"github.com/strefethen/sonos-hub-go/internal/sonoscloud"
	"github.com/strefethen/sonos-hub-go/internal/spotifysearch"
	"github.com/strefethen/sonos-hub-go/internal/system"
	"github.com/strefethen/sonos-hub-go/internal/templates"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker for WebSocket support
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// requestLoggerMiddleware logs all incoming HTTP requests
func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), wrapped.status, time.Since(start).Round(time.Millisecond))
	})
}

// Options controls server wiring.
type Options struct {
	DisableDiscovery bool
}

// NewHandler builds the HTTP handler and returns a shutdown function.
func NewHandler(cfg config.Config, options Options) (http.Handler, func(context.Context) error, error) {
	log.Printf("Using database: %s", cfg.SQLiteDBPath)
	dbPair, err := db.Init(cfg.SQLiteDBPath)
	if err != nil {
		return nil, nil, err
	}

	router := chi.NewRouter()
	router.Use(middleware.StripSlashes) // Handle trailing slashes like Node.js
	router.Use(requestLoggerMiddleware)
	router.Use(api.RequestIDMiddleware)
	router.Use(api.RecovererMiddleware)
	router.Use(auth.Middleware(cfg))

	registerHealthRoutes(router)
	openapi.RegisterRoutes(router)

	pairingStore := auth.NewPairingStore(5 * time.Minute)
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	pairingStore.StartCleanup(shutdownCtx, time.Minute)
	auth.RegisterRoutes(router, pairingStore, cfg)

	soapClient := soap.NewClient(time.Duration(cfg.SonosTimeoutMs) * time.Millisecond)
	deviceService := devices.NewService(cfg, nil, soapClient)

	// Create zone cache for sharing between sonos service and event manager
	zoneCache := sonos.NewZoneGroupCache(time.Duration(cfg.ZoneCacheTTLSeconds) * time.Second)

	// Create UPnP event manager for real-time device state updates
	// Must be created before device discovery starts so we can subscribe to devices
	port, _ := strconv.Atoi(cfg.Port)
	eventConfig := events.ManagerConfig{
		Enabled:             cfg.UPnPEventsEnabled,
		SubscriptionTimeout: cfg.UPnPSubscriptionTimeoutSec,
		RenewalBuffer:       60,
		StateCacheTTL:       time.Duration(cfg.UPnPStateCacheTTLSeconds) * time.Second,
		Services: []events.ServiceType{
			events.ServiceAVTransport,
			events.ServiceRenderingControl,
			events.ServiceZoneGroupTopology,
		},
	}
	eventManager := events.NewManager(eventConfig, port, zoneCache)

	// Set up device discovery callback to subscribe to UPnP events when devices are found
	if cfg.UPnPEventsEnabled && !options.DisableDiscovery {
		deviceService.SetDiscoveryCallback(func(discovered []devices.DeviceInfo) {
			for _, device := range discovered {
				go func(ip, udn string) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if err := eventManager.SubscribeDevice(ctx, ip, udn); err != nil {
						log.Printf("UPNP: Failed to subscribe to %s: %v", ip, err)
					}
				}(device.IP, device.UDN)
			}
		})
	}

	// Only disable discovery if explicitly requested via options (for tests)
	// AllowTestMode is for auth bypass only, not for skipping device discovery
	if options.DisableDiscovery {
		deviceService.SetTestMode(true)
	} else {
		deviceService.StartPeriodicDiscovery()
	}
	devices.RegisterRoutes(router, deviceService)

	// Create state provider adapter to break import cycle
	var stateProvider sonos.StateProvider
	if cfg.UPnPEventsEnabled {
		stateProvider = NewStateCacheAdapter(eventManager.GetStateCache())
	}

	// Create sonos service with state provider for hybrid data layer
	sonosService := sonos.NewServiceWithStateProvider(deviceService, soapClient, cfg.DefaultSonosIP, time.Duration(cfg.SonosTimeoutMs)*time.Millisecond, time.Duration(cfg.ZoneCacheTTLSeconds)*time.Second, stateProvider)
	sonosService.ZoneCache = zoneCache // Use the shared zone cache
	sonos.RegisterRoutes(router, sonosService)

	// UPnP callback handler - will be wired up outside Chi to bypass method restrictions
	var upnpHandler http.Handler
	if cfg.UPnPEventsEnabled && !options.DisableDiscovery {
		callbackHandler := events.NewCallbackHandler(eventManager)
		upnpMux := http.NewServeMux()
		upnpMux.Handle("/upnp/notify", callbackHandler)
		upnpMux.Handle("/upnp/notify/avtransport", callbackHandler)
		upnpMux.Handle("/upnp/notify/renderingcontrol", callbackHandler)
		upnpMux.Handle("/upnp/notify/topology", callbackHandler)
		upnpHandler = upnpMux

		// Start event manager
		if err := eventManager.Start(); err != nil {
			log.Printf("Warning: Failed to start UPnP event manager: %v", err)
		}
	}

	playService := sonos.NewPlayService(soapClient, deviceService, time.Duration(cfg.SonosTimeoutMs)*time.Millisecond, nil)
	sonos.RegisterPlayRoutes(router, playService)

	sceneService := scene.NewService(cfg, dbPair, nil, deviceService, soapClient)
	scene.RegisterRoutes(router, sceneService)

	// Create Spotify search connection manager (for Chrome extension WebSocket)
	spotifySearchManager := spotifysearch.NewConnectionManager()
	spotifysearch.RegisterRoutes(router, spotifySearchManager)

	// Create Apple Music client if configured
	var appleClient *applemusic.Client
	if cfg.AppleTeamID != "" && cfg.AppleKeyID != "" && cfg.ApplePrivateKeyPath != "" {
		tokenManager, err := applemusic.NewTokenManager(applemusic.TokenManagerConfig{
			TeamID:         cfg.AppleTeamID,
			KeyID:          cfg.AppleKeyID,
			PrivateKeyPath: cfg.ApplePrivateKeyPath,
			Expiry:         time.Duration(cfg.AppleTokenExpirySec) * time.Second,
		})
		if err != nil {
			log.Printf("Warning: Failed to create Apple Music token manager: %v", err)
		} else {
			appleClient = applemusic.NewClient(applemusic.ClientConfig{
				TokenManager: tokenManager,
				BaseURL:      cfg.AppleMusicAPIURL,
				Storefront:   cfg.DefaultStorefront,
				Timeout:      time.Duration(cfg.SonosTimeoutMs) * time.Millisecond,
			})
			log.Printf("Apple Music client initialized (storefront: %s)", cfg.DefaultStorefront)
		}
	}

	// Create music service (needed for scheduler routes)
	musicService := music.NewService(cfg, dbPair, nil)
	music.RegisterRoutes(router, musicService, spotifySearchManager, appleClient, soapClient, deviceService)

	// Create content resolver for routine execution (handles direct service playback)
	contentResolver := sonos.NewContentResolver(
		soapClient,
		deviceService,
		time.Duration(cfg.SonosTimeoutMs)*time.Millisecond,
		nil,
	)

	// Create scene adapter for the routine executor
	sceneAdapter := scheduler.NewSceneServiceAdapter(sceneService)

	// Create routine executor adapter that handles music resolution before scene execution
	routineExecutor := scheduler.NewRoutineExecutorAdapter(
		sceneAdapter,       // SceneExecutor for scene execution
		musicService,       // For SelectItem from music sets
		contentResolver,    // For ResolveFavorite/ResolveDirectContent to get URI/metadata
		deviceService,      // For ResolveDeviceIP
		time.Duration(cfg.SonosTimeoutMs)*time.Millisecond,
	)

	// Create scheduler service with routine executor
	schedulerService := scheduler.NewService(cfg, dbPair, nil, routineExecutor)
	scheduler.RegisterRoutes(router,
		scheduler.NewRoutinesRepository(dbPair),
		scheduler.NewJobsRepository(dbPair),
		scheduler.NewHolidaysRepository(dbPair),
		sceneService,
		deviceService,
		musicService,
	)
	schedulerService.Start()

	// Create audit service
	auditService := audit.NewService(cfg, dbPair, nil)
	audit.RegisterRoutes(router, auditService)
	auditService.StartPruneJob()

	// Create system service (with scheduler for status reporting)
	systemService := system.NewService(cfg, dbPair, nil, deviceService, schedulerService)
	system.RegisterRoutes(router, systemService)

	// Create templates service
	templatesService := templates.NewService(dbPair)
	templates.RegisterRoutes(router, templatesService)

	// Create settings service
	settingsService := settings.NewService(dbPair, nil)
	settings.RegisterRoutes(router, settingsService)

	// Create Sonos Cloud service (only if configured)
	if cfg.SonosClientID != "" && cfg.SonosClientSecret != "" {
		sonosCloudRepo := sonoscloud.NewRepository(dbPair)
		sonosCloudClient := sonoscloud.NewClient(cfg.SonosClientID, cfg.SonosClientSecret, cfg.SonosRedirectURI, sonosCloudRepo)
		sonoscloud.RegisterRoutes(router, sonosCloudClient)
	}

	// Register webhook route for Sonos Cloud events (always available, doesn't require OAuth)
	// The webhook endpoint receives events from Sonos servers, not user browsers
	if cfg.UPnPEventsEnabled {
		stateCache := eventManager.GetStateCache()
		// For now, pass nil resolver - cloud webhooks will log but not update cache
		// until we implement group ID to IP resolution
		sonoscloud.RegisterWebhookRoute(router, stateCache, nil)
	}

	// Serve static files with caching headers (matching Node.js behavior)
	fileServer := http.FileServer(http.Dir("./assets"))
	router.Handle("/v1/assets/*", http.StripPrefix("/v1/assets/", staticFileHandler(fileServer)))

	shutdown := func(ctx context.Context) error {
		shutdownCancel()
		schedulerService.Stop()
		auditService.StopPruneJob()
		deviceService.StopPeriodicDiscovery()
		spotifySearchManager.Close()
		// Stop UPnP event manager (unsubscribes from all devices)
		if eventManager != nil && eventManager.IsEnabled() {
			eventManager.Stop(ctx)
		}
		if ctx == nil {
			ctx = context.Background()
		}
		return dbPair.Close()
	}

	// Wrap router to intercept /upnp paths before Chi (NOTIFY is not a standard HTTP method)
	var handler http.Handler = router
	if upnpHandler != nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/upnp/") {
				upnpHandler.ServeHTTP(w, r)
				return
			}
			router.ServeHTTP(w, r)
		})
	}

	return handler, shutdown, nil
}

// staticFileHandler wraps a file server with caching headers matching Node.js behavior
func staticFileHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set caching headers for static assets (1 year cache)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func registerHealthRoutes(router chi.Router) {
	router.Method(http.MethodGet, "/v1/health", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		response := map[string]any{
			"status":    "healthy",
			"service":   "sonos-hub",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		return api.WriteJSON(w, http.StatusOK, response)
	}))
	router.Method(http.MethodGet, "/v1/health/live", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		return api.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}))
	router.Method(http.MethodGet, "/v1/health/ready", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		return api.WriteJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	}))
}
