# Sonos Hub Go

A high-performance LAN-only backend for Sonos smart home automation, written in Go. This is a complete rewrite of the original Node.js/TypeScript implementation, designed for simplified deployment as a single binary.

## Features

- **Device Discovery** — SSDP multicast discovery with static IP fallback for wired devices
- **Zone Topology** — Automatic detection of stereo pairs and home theater groups
- **Scene Engine** — Create and execute multi-device scenes with volume ramping
- **Routine Scheduler** — Cron-based scheduling with US federal holiday awareness
- **Music Catalog** — Organize music into sets with no-repeat tracking
- **Sonos Cloud Integration** — OAuth flow for cloud API access
- **Apple Music** — Native integration for playlist and album playback
- **Audit Logging** — Track all system events with configurable retention
- **JWT Authentication** — Secure device pairing with access/refresh tokens

## Requirements

- Go 1.21+ (CGO enabled for SQLite)
- SQLite 3
- Network access to Sonos devices (same LAN segment)

## Quick Start

### 1. Clone and configure

```bash
git clone https://github.com/strefethen/sonos-hub-go.git
cd sonos-hub-go

# Create environment file
cp .env.example .env

# Edit .env with your settings (at minimum, set JWT_SECRET)
```

### 2. Build and run

```bash
# Build the binary
CGO_ENABLED=1 go build -o sonos-hub ./cmd/sonos-hub

# Run
./sonos-hub
```

### 3. With hot reload (development)

```bash
# Install air if you haven't
go install github.com/cosmtrek/air@latest

# Run with hot reload
set -a && source .env && set +a && air
```

The server starts on `http://localhost:9000` by default.

## Configuration

All configuration is via environment variables. Create a `.env` file in the project root:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `9000` | HTTP server port |
| `HOST` | `0.0.0.0` | Bind address |
| `JWT_SECRET` | (required) | JWT signing key (32+ characters) |
| `SQLITE_DB_PATH` | `./data/sonos-hub.db` | SQLite database path |
| `NODE_ENV` | `development` | Environment mode |
| `LOG_LEVEL` | `info` | Log level (trace, debug, info, warn, error) |

### Device Discovery

| Variable | Default | Description |
|----------|---------|-------------|
| `SSDP_DISCOVERY_TIMEOUT_MS` | `15000` | Full discovery cycle timeout |
| `SSDP_DISCOVERY_PASSES` | `3` | Number of SSDP passes |
| `SSDP_RESCAN_INTERVAL_MS` | `60000` | Periodic rescan interval (0 to disable) |
| `STATIC_DEVICE_IPS` | | Comma-separated IPs for wired devices |
| `DEVICE_OFFLINE_THRESHOLD_MS` | `120000` | Mark device offline after this period |

### Sonos Control

| Variable | Default | Description |
|----------|---------|-------------|
| `SONOS_TIMEOUT_MS` | `5000` | UPnP SOAP request timeout |
| `SONOS_CLIENT_ID` | | Sonos Cloud OAuth client ID |
| `SONOS_CLIENT_SECRET` | | Sonos Cloud OAuth client secret |
| `SONOS_REDIRECT_URI` | | OAuth callback URL |

### Scheduler

| Variable | Default | Description |
|----------|---------|-------------|
| `DEFAULT_TIMEZONE` | `America/New_York` | Default timezone for routines |
| `JOB_WINDOW_DAYS` | `7` | Days ahead to generate jobs |
| `JOB_POLL_INTERVAL_MS` | `15000` | Job runner poll interval |

### Apple Music (optional)

| Variable | Description |
|----------|-------------|
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `APPLE_KEY_ID` | MusicKit key ID |
| `APPLE_PRIVATE_KEY_PATH` | Path to `.p8` private key file |

## API Overview

All endpoints return JSON with a consistent response format:

```json
// Success (single resource)
{
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "scene": { ... }
}

// Success (collection)
{
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "scenes": [ ... ],
  "pagination": { "total": 10, "limit": 20, "offset": 0, "has_more": false }
}

// Error
{
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "error": { "code": "SCENE_NOT_FOUND", "message": "Scene not found" }
}
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| **Authentication** |||
| POST | `/v1/auth/pair/start` | Start device pairing |
| POST | `/v1/auth/pair/complete` | Complete pairing with code |
| POST | `/v1/auth/refresh` | Refresh access token |
| **Devices** |||
| GET | `/v1/devices` | List discovered devices |
| GET | `/v1/devices/{id}` | Get device details |
| POST | `/v1/devices/discover` | Trigger discovery |
| **Scenes** |||
| GET | `/v1/scenes` | List scenes |
| POST | `/v1/scenes` | Create scene |
| GET | `/v1/scenes/{id}` | Get scene |
| PUT | `/v1/scenes/{id}` | Update scene |
| DELETE | `/v1/scenes/{id}` | Delete scene |
| POST | `/v1/scenes/{id}/execute` | Execute scene |
| **Routines** |||
| GET | `/v1/routines` | List routines |
| POST | `/v1/routines` | Create routine |
| GET | `/v1/routines/{id}` | Get routine |
| PUT | `/v1/routines/{id}` | Update routine |
| DELETE | `/v1/routines/{id}` | Delete routine |
| **Music** |||
| GET | `/v1/music/sets` | List music sets |
| POST | `/v1/music/sets` | Create music set |
| GET | `/v1/music/sets/{id}` | Get music set |
| PUT | `/v1/music/sets/{id}` | Update music set |
| DELETE | `/v1/music/sets/{id}` | Delete music set |
| **Sonos Control** |||
| GET | `/v1/sonos/favorites` | List Sonos favorites |
| POST | `/v1/sonos/{deviceId}/play` | Start playback |
| POST | `/v1/sonos/{deviceId}/pause` | Pause playback |
| POST | `/v1/sonos/{deviceId}/volume` | Set volume |
| **System** |||
| GET | `/v1/health` | Health check |
| GET | `/v1/system/info` | System information |
| GET | `/v1/dashboard` | Dashboard data |

Full API documentation is available via OpenAPI spec at `assets/openapi/sonos-hub.v1.yaml`.

## Project Structure

```
sonos-hub-go/
├── cmd/sonos-hub/          # Application entry point
│   └── main.go
├── internal/               # Private packages
│   ├── api/                # HTTP utilities, response helpers
│   ├── apperrors/          # Centralized error types
│   ├── audit/              # Audit logging service
│   ├── auth/               # JWT auth, pairing, middleware
│   ├── config/             # Environment configuration
│   ├── db/                 # SQLite setup and schema
│   ├── devices/            # Device registry and discovery
│   ├── discovery/          # SSDP and HTTP probe
│   ├── music/              # Music catalog management
│   ├── scene/              # Scene CRUD and execution
│   ├── scheduler/          # Cron scheduling and job runner
│   ├── server/             # HTTP server wiring
│   ├── settings/           # User settings
│   ├── sonos/              # Sonos SOAP client and control
│   │   └── soap/           # UPnP SOAP protocol
│   ├── sonoscloud/         # Sonos Cloud OAuth
│   ├── system/             # System info endpoints
│   └── templates/          # Routine templates
├── tests/                  # Integration tests
│   ├── phase0/             # Basic connectivity
│   ├── phase1/             # Authentication
│   └── phase6/             # Full integration
├── assets/                 # Static assets
│   └── openapi/            # OpenAPI specification
└── data/                   # SQLite database (gitignored)
```

## Testing

```bash
# Run all tests
go test ./...

# Run specific phase
go test -v ./tests/phase6/...

# Run with coverage
go test -cover ./internal/...
```

### Test Mode

For integration testing without JWT authentication:

```bash
# In .env
ALLOW_TEST_MODE=true
NODE_ENV=development

# In requests
curl -H "X-Test-Mode: true" http://localhost:9000/v1/devices
```

## Building for Production

```bash
# Build optimized binary
CGO_ENABLED=1 go build -ldflags="-s -w" -o sonos-hub ./cmd/sonos-hub

# The binary is self-contained (except for SQLite which requires CGO)
./sonos-hub
```

## Architecture

### UPnP/SOAP Communication

Sonos devices use UPnP for control. The SOAP client in `internal/sonos/soap/` implements:

- Transport controls (Play, Pause, Stop, Next, Previous)
- Volume controls (GetVolume, SetVolume, SetMute)
- Queue management (AddURIToQueue, RemoveAllTracksFromQueue)
- Content browsing (Browse ContentDirectory)

### Device Discovery

1. **SSDP Multicast** — Discovers devices responding to `urn:schemas-upnp-org:device:ZonePlayer:1`
2. **HTTP Probe** — Fetches device description XML from each discovered IP
3. **Zone Topology** — Parses `/status/topology` for group membership
4. **Static Fallback** — Probes `STATIC_DEVICE_IPS` for wired devices

### Scene Execution

Scenes execute in phases:
1. **Preflight** — Validate all devices are reachable
2. **Volume Ramp** — Gradually adjust volume to target
3. **Content** — Set transport URI and start playback
4. **Grouping** — Join devices to coordinator

## License

MIT

## Contributing

Contributions are welcome. Please ensure:

1. All tests pass (`go test ./...`)
2. Code follows Go conventions (`go fmt`, `go vet`)
3. New features include tests
