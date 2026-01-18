package openapi

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// Default paths to search for the OpenAPI spec
var defaultSpecPaths = []string{
	// Relative to Go project root
	"assets/openapi/sonos-hub.v1.yaml",
	// Relative to monorepo root (when running from monorepo)
	"packages/openapi/openapi/sonos-hub.v1.yaml",
	// Absolute path from monorepo (development)
	"../sonos-hub/packages/openapi/openapi/sonos-hub.v1.yaml",
}

// RegisterRoutes wires OpenAPI routes to the router.
func RegisterRoutes(router chi.Router) {
	router.Method(http.MethodGet, "/v1/openapi", api.Handler(serveOpenAPIYAML()))
	router.Method(http.MethodGet, "/v1/openapi.json", api.Handler(serveOpenAPIJSON()))
}

// findSpecPath locates the OpenAPI spec file
func findSpecPath() string {
	// Check environment variable first
	if envPath := os.Getenv("OPENAPI_SPEC_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// Try default paths
	for _, path := range defaultSpecPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath
		}
	}

	return ""
}

func serveOpenAPIYAML() func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		specPath := findSpecPath()
		if specPath == "" {
			return apperrors.NewInternalError("OpenAPI specification file not found")
		}

		spec, err := os.ReadFile(specPath)
		if err != nil {
			return apperrors.NewInternalError("Failed to read OpenAPI specification")
		}

		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec)
		return nil
	}
}

func serveOpenAPIJSON() func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		specPath := findSpecPath()
		if specPath == "" {
			return apperrors.NewInternalError("OpenAPI specification file not found")
		}

		spec, err := os.ReadFile(specPath)
		if err != nil {
			return apperrors.NewInternalError("Failed to read OpenAPI specification")
		}

		// Parse YAML and convert to JSON
		var parsed any
		if err := yaml.Unmarshal(spec, &parsed); err != nil {
			return apperrors.NewInternalError("Failed to parse OpenAPI specification")
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		return api.WriteJSON(w, http.StatusOK, parsed)
	}
}
