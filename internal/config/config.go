package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// Config holds the base server configuration.
type Config struct {
	Host                     string
	Port                     string
	SQLiteDBPath             string
	NodeEnv                  string
	AllowTestMode            bool
	JWTSecret                string
	JWTAccessTokenExpirySec  int
	JWTRefreshTokenExpirySec int
	SSDPDiscoveryTimeoutMs   int
	SSDPDiscoveryPasses      int
	SSDPPassIntervalMs       int
	SSDPRescanIntervalMs     int
	StaticDeviceIPs          []string
	SonosTimeoutMs           int
	DefaultSonosIP           string
	SonosClientID            string
	SonosClientSecret        string
	SonosRedirectURI         string
}

// Load reads configuration from environment variables with defaults.
func Load() (Config, error) {
	host := envString("HOST", "0.0.0.0")
	port := envString("PORT", "9000")
	sqlitePath := envString("SQLITE_DB_PATH", "./data/sonos-hub.db")

	// Warn if database path appears to point to the Node.js project instead of Go project
	// This happens when SQLITE_DB_PATH is exported in shell from another project
	if strings.Contains(sqlitePath, "/sonos-hub/") && !strings.Contains(sqlitePath, "/sonos-hub-go/") {
		log.Printf("WARNING: SQLITE_DB_PATH appears to point to Node.js project: %s", sqlitePath)
		log.Printf("WARNING: Expected Go project database in sonos-hub-go/data/")
		log.Printf("WARNING: Fix: unset SQLITE_DB_PATH && set -a && source .env && set +a && air")
	}

	nodeEnv := envString("NODE_ENV", "development")
	allowTestMode := envBool("ALLOW_TEST_MODE", false)
	jwtSecret := envString("JWT_SECRET", "")
	jwtAccessExpiry := envInt("JWT_ACCESS_TOKEN_EXPIRY", 3600)
	jwtRefreshExpiry := envInt("JWT_REFRESH_TOKEN_EXPIRY", 2592000)
	ssdpTimeout := envInt("SSDP_DISCOVERY_TIMEOUT_MS", 5000)
	ssdpPasses := envInt("SSDP_DISCOVERY_PASSES", 3)
	ssdpPassInterval := envInt("SSDP_PASS_INTERVAL_MS", 2000)
	ssdpRescanInterval := envInt("SSDP_RESCAN_INTERVAL_MS", 60000)
	staticIPs := envCSV("STATIC_DEVICE_IPS")
	sonosTimeout := envInt("SONOS_TIMEOUT_MS", 5000)
	defaultSonosIP := envString("DEFAULT_SONOS_IP", "192.168.1.10")
	sonosClientID := envString("SONOS_CLIENT_ID", "")
	sonosClientSecret := envString("SONOS_CLIENT_SECRET", "")
	sonosRedirectURI := envString("SONOS_REDIRECT_URI", "")

	if len(strings.TrimSpace(jwtSecret)) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}

	return Config{
		Host:                     host,
		Port:                     port,
		SQLiteDBPath:             sqlitePath,
		NodeEnv:                  nodeEnv,
		AllowTestMode:            allowTestMode,
		JWTSecret:                jwtSecret,
		JWTAccessTokenExpirySec:  jwtAccessExpiry,
		JWTRefreshTokenExpirySec: jwtRefreshExpiry,
		SSDPDiscoveryTimeoutMs:   ssdpTimeout,
		SSDPDiscoveryPasses:      ssdpPasses,
		SSDPPassIntervalMs:       ssdpPassInterval,
		SSDPRescanIntervalMs:     ssdpRescanInterval,
		StaticDeviceIPs:          staticIPs,
		SonosTimeoutMs:           sonosTimeout,
		DefaultSonosIP:           defaultSonosIP,
		SonosClientID:            sonosClientID,
		SonosClientSecret:        sonosClientSecret,
		SonosRedirectURI:         sonosRedirectURI,
	}, nil
}

func envString(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func envInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return strings.EqualFold(val, "true")
}

func envCSV(key string) []string {
	val := os.Getenv(key)
	if val == "" {
		return []string{}
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
