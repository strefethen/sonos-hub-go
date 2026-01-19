package server

import (
	"context"
	"log"
	"net/http"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/strefethen/sonos-hub-go/internal/api"
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
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
	"github.com/strefethen/sonos-hub-go/internal/sonoscloud"
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

// requestLoggerMiddleware logs all incoming HTTP requests
func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, wrapped.status, time.Since(start).Round(time.Millisecond))
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
	// Only disable discovery if explicitly requested via options (for tests)
	// AllowTestMode is for auth bypass only, not for skipping device discovery
	if options.DisableDiscovery {
		deviceService.SetTestMode(true)
	} else {
		deviceService.StartPeriodicDiscovery()
	}
	devices.RegisterRoutes(router, deviceService)

	sonosService := sonos.NewService(deviceService, soapClient, cfg.DefaultSonosIP, time.Duration(cfg.SonosTimeoutMs)*time.Millisecond)
	sonos.RegisterRoutes(router, sonosService)

	playService := sonos.NewPlayService(soapClient, deviceService, time.Duration(cfg.SonosTimeoutMs)*time.Millisecond, nil)
	sonos.RegisterPlayRoutes(router, playService)

	sceneService := scene.NewService(cfg, dbPair, nil, deviceService, soapClient)
	scene.RegisterRoutes(router, sceneService)

	// Create music service (needed for scheduler routes)
	musicService := music.NewService(cfg, dbPair, nil)
	music.RegisterRoutes(router, musicService)

	// Create scheduler service with scene service adapter as dependency
	sceneAdapter := scheduler.NewSceneServiceAdapter(sceneService)
	schedulerService := scheduler.NewService(cfg, dbPair, nil, sceneAdapter)
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
		sonosCloudClient := sonoscloud.NewClient(cfg.SonosClientID, cfg.SonosClientSecret, "", sonosCloudRepo)
		sonoscloud.RegisterRoutes(router, sonosCloudClient)
	}

	// Serve static files
	router.Handle("/v1/assets/*", http.StripPrefix("/v1/assets/", http.FileServer(http.Dir("./assets"))))

	shutdown := func(ctx context.Context) error {
		shutdownCancel()
		schedulerService.Stop()
		auditService.StopPruneJob()
		deviceService.StopPeriodicDiscovery()
		if ctx == nil {
			ctx = context.Background()
		}
		return dbPair.Close()
	}

	return router, shutdown, nil
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
