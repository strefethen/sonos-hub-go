# CLAUDE.md — Sonos Hub Go (API Backend)

Generated: 2025-01-15

This is the **Go rewrite** of the Sonos Hub backend, migrated from Node.js/TypeScript for improved performance, simplified deployment, and a single-binary distribution.

## Project Overview

- **Purpose**: LAN-only Sonos routines hub backend with REST API
- **Runtime**: Go 1.25+ with Chi router
- **Database**: SQLite with WAL mode
- **Architecture**: Monolith with internal packages (not microservices)

Primary goals:

- **1:1 API parity** with Node.js version (same endpoints, same response shapes)
- **Single binary deployment**: `go build` produces one executable
- **Production-ready**: proper error handling, graceful shutdown, concurrent-safe
- **Test-first**: comprehensive integration tests against real (or mocked) services

## Development Setup

```bash
# Load environment and run with hot reload (preferred)
set -a && source .env && set +a && air

# Or without hot reload
set -a && source .env && set +a && go run ./cmd/sonos-hub

# Or export individually
export JWT_SECRET=your-secret-key
export SQLITE_DB_PATH=./data/sonos-hub.db
go run ./cmd/sonos-hub
```

**Hot Reloading**: The project uses [air](https://github.com/cosmtrek/air) for hot reloading. Code changes are automatically detected and the server restarts. No manual restart needed for most changes.

**IMPORTANT**: Always ask the user to start/restart the server manually. They want servers running in a terminal where they can view the logs.

## Project Structure

```
sonos-hub-go/
├── cmd/sonos-hub/          # Main entry point
│   └── main.go
├── internal/               # Private application packages
│   ├── api/                # HTTP utilities (handler wrapper, request ID, responses)
│   ├── apperrors/          # Centralized error types
│   ├── audit/              # Audit log service
│   ├── auth/               # JWT auth, pairing, middleware
│   ├── config/             # Environment configuration
│   ├── db/                 # SQLite initialization and schema
│   ├── devices/            # Device registry and discovery
│   ├── discovery/          # SSDP discovery
│   ├── music/              # Music catalog (sets, items, providers)
│   ├── scene/              # Scene engine (CRUD, execution, volume ramp)
│   ├── scheduler/          # Routine scheduling (cron, jobs, holidays)
│   ├── server/             # HTTP server wiring
│   ├── settings/           # User settings (TV routing)
│   ├── sonos/              # Sonos control (SOAP client, play commands)
│   │   └── soap/           # UPnP SOAP protocol implementation
│   ├── sonoscloud/         # Sonos Cloud OAuth integration
│   ├── system/             # System info and dashboard
│   └── templates/          # Routine templates
├── tests/                  # Integration tests by phase
│   ├── phase0/             # Basic connectivity
│   ├── phase1/             # Auth & device tests
│   ├── phase2/             # Device registry
│   ├── phase3/             # Sonos control
│   ├── phase4/             # Music catalog
│   ├── phase5/             # Scheduler
│   └── phase6/             # Scene engine
├── data/                   # SQLite database files (gitignored)
└── assets/                 # Static assets (templates, logos)
```

## Coding Standards

### Go Conventions

- **Package naming**: lowercase, single word (e.g., `scene`, `scheduler`, `devices`)
- **Error handling**: always check and return errors, use `fmt.Errorf("context: %w", err)` for wrapping
- **No panics in production code**: panics are for programmer errors only
- **Interface segregation**: small interfaces, accept interfaces return structs
- **Context propagation**: pass `context.Context` as first parameter

### HTTP Patterns (Chi Router)

```go
// Route registration pattern
func RegisterRoutes(router chi.Router, service *Service) {
    router.Route("/v1/resource", func(r chi.Router) {
        r.Method(http.MethodGet, "/", api.Handler(listHandler(service)))
        r.Method(http.MethodPost, "/", api.Handler(createHandler(service)))
        r.Method(http.MethodGet, "/{id}", api.Handler(getHandler(service)))
        r.Method(http.MethodPut, "/{id}", api.Handler(updateHandler(service)))
        r.Method(http.MethodDelete, "/{id}", api.Handler(deleteHandler(service)))
    })
}

// Handler wrapper returns errors instead of writing directly
func getHandler(svc *Service) api.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) error {
        id := chi.URLParam(r, "id")
        result, err := svc.Get(r.Context(), id)
        if err != nil {
            return err // api.Handler converts to HTTP response
        }
        return api.WriteJSON(w, http.StatusOK, api.DataResponse{
            RequestID: api.GetRequestID(r.Context()),
            Data:      result,
        })
    }
}
```

### Error Handling Pattern

```go
// internal/apperrors/errors.go defines app-wide error types
type AppError struct {
    Code       string
    Message    string
    StatusCode int
}

// Return specific errors from services
if scene == nil {
    return nil, &apperrors.AppError{
        Code:       "SCENE_NOT_FOUND",
        Message:    "Scene not found",
        StatusCode: http.StatusNotFound,
    }
}

// api.Handler automatically converts AppError to JSON response
```

### Database Pattern (SQLite)

```go
// Repository pattern with explicit SQL
type Repository struct {
    db *sql.DB
}

func (r *Repository) GetByID(ctx context.Context, id string) (*Model, error) {
    row := r.db.QueryRowContext(ctx, `
        SELECT id, name, created_at, updated_at
        FROM table_name
        WHERE id = ?
    `, id)

    var m Model
    if err := row.Scan(&m.ID, &m.Name, &m.CreatedAt, &m.UpdatedAt); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, nil // Not found returns nil, nil
        }
        return nil, fmt.Errorf("query: %w", err)
    }
    return &m, nil
}
```

### JSON Serialization

```go
// Use struct tags for API responses
type Scene struct {
    SceneID               string    `json:"scene_id"`
    Name                  string    `json:"name"`
    Description           *string   `json:"description,omitempty"`
    CoordinatorPreference string    `json:"coordinator_preference"`
    CreatedAt             time.Time `json:"created_at"`
}

// Parse time as RFC3339
createdAt, _ := time.Parse(time.RFC3339, row.CreatedAt)
```

## Testing

### Running Tests

```bash
# All tests
go test ./...

# Specific phase
go test ./tests/phase6/...

# With verbose output
go test -v ./tests/phase6/...

# With coverage
go test -cover ./internal/...
```

### Integration Test Pattern

```go
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
    t.Helper()
    t.Setenv("JWT_SECRET", "test-secret-32-characters-long!!")
    t.Setenv("NODE_ENV", "development")
    t.Setenv("ALLOW_TEST_MODE", "true")

    tempDir := t.TempDir()
    dbPath := filepath.Join(tempDir, "test.db")
    t.Setenv("SQLITE_DB_PATH", dbPath)

    cfg, err := config.Load()
    require.NoError(t, err)

    handler, shutdown, err := server.NewHandler(cfg, server.Options{DisableDiscovery: true})
    require.NoError(t, err)

    ts := httptest.NewServer(handler)
    return ts, func() {
        ts.Close()
        require.NoError(t, shutdown(nil))
    }
}

func doRequest(t *testing.T, method, url string, body any) *http.Response {
    t.Helper()
    var buf *bytes.Buffer
    if body != nil {
        payload, _ := json.Marshal(body)
        buf = bytes.NewBuffer(payload)
    } else {
        buf = bytes.NewBuffer(nil)
    }

    req, _ := http.NewRequest(method, url, buf)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Test-Mode", "true")  // Bypass JWT auth

    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    return resp
}
```

### Test Mode Header

For integration testing without JWT authentication:

```bash
curl -H "X-Test-Mode: true" http://localhost:9000/v1/devices
```

**Requirements:**
- `ALLOW_TEST_MODE=true` in `.env`
- `NODE_ENV=development`
- Header: `X-Test-Mode: true`

## API Response Format

All endpoints follow Stripe API conventions:

```json
// Single resource - returned directly with object type
{
    "id": "rtn_abc123",
    "object": "routine",
    "name": "Morning Music",
    "enabled": true,
    ...
}

// Collection/List - uses generic "data" array (path defines resource)
{
    "object": "list",
    "data": [ ... ],
    "has_more": true,
    "url": "/v1/routines"
}
// Pagination: use ?limit=N&starting_after=id or ?ending_before=id

// Create/Update - returns the resource directly
{
    "id": "rtn_abc123",
    "object": "routine",
    ...
}

// Action result - returns relevant data flat
{
    "object": "execution",
    "id": "exec_xyz789",
    "status": "started"
}

// Error - wrapped in error object
{
    "error": {
        "type": "invalid_request_error",
        "code": "resource_not_found",
        "message": "Routine not found"
    }
}
```

### Object Types

Each resource includes an `object` field identifying its type:

| Resource | Object Type |
|----------|-------------|
| Routine | `routine` |
| Scene | `scene` |
| Device | `device` |
| Music Set | `music_set` |
| Execution | `execution` |
| Favorite | `favorite` |
| List response | `list` |

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | HTTP router with middleware support |
| `github.com/golang-jwt/jwt/v5` | JWT token parsing and validation |
| `github.com/mattn/go-sqlite3` | SQLite driver (requires CGO) |
| `github.com/google/uuid` | UUID generation |
| `github.com/stretchr/testify` | Test assertions |
| `github.com/robfig/cron/v3` | Cron expression parsing |

## SOAP/UPnP Communication

Sonos devices use UPnP SOAP for control. The implementation is in `internal/sonos/soap/`:

```go
// SOAP action call pattern
client := soap.NewClient(5 * time.Second)
result, err := client.Play(ctx, deviceIP)

// Available actions (internal/sonos/soap/actions.go):
// - Play, Pause, Stop, Next, Previous
// - SetVolume, GetVolume, SetMute, GetMute
// - SetAVTransportURI, GetPositionInfo
// - Browse (ContentDirectory)
// - AddURIToQueue, RemoveAllTracksFromQueue
```

## Content Resolution (Music Playback)

Playing content from streaming services requires URI construction:

```go
// internal/sonos/content_resolver.go
type MusicContent struct {
    Type        string  `json:"type"`         // "sonos_favorite" or "direct"
    FavoriteID  *string `json:"favorite_id"`  // For favorites
    Service     *string `json:"service"`      // "spotify", "apple_music"
    ContentType *string `json:"content_type"` // "playlist", "album", "track", "station"
    ContentID   *string `json:"content_id"`   // Service-specific ID
}

// Service configurations (internal/sonos/uri_builder.go)
// Spotify SID=12, Apple Music SID=204
// URI schemes: x-rincon-cpcontainer, x-sonos-http, x-sonosapi-radio
```

## Configuration

Environment variables (see `.env`):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 9000 | HTTP listen port |
| `HOST` | 0.0.0.0 | Bind address |
| `JWT_SECRET` | (required) | JWT signing key (32+ chars) |
| `SQLITE_DB_PATH` | ./data/sonos-hub.db | Database location |
| `SONOS_TIMEOUT_MS` | 5000 | SOAP request timeout |
| `ALLOW_TEST_MODE` | false | Enable X-Test-Mode header |
| `SONOS_CLIENT_ID` | (optional) | Sonos Cloud OAuth client ID |
| `SONOS_CLIENT_SECRET` | (optional) | Sonos Cloud OAuth client secret |

## Data Constraints

**CRITICAL Redis Rules:**
- **NEVER** use `FLUSHALL` on Redis
- **NEVER** delete Redis keys without a use-case specific pattern

## Graceful Shutdown

The server handles SIGINT/SIGTERM:

```go
// cmd/sonos-hub/main.go
shutdownCh := make(chan os.Signal, 1)
signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

go func() {
    <-shutdownCh
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    shutdownHandler(ctx)  // Close DB, stop schedulers
    srv.Shutdown(ctx)     // Stop accepting requests
}()
```

## Common Operations

### Adding a New Endpoint

1. Add types to `internal/<package>/types.go`
2. Add repository methods to `internal/<package>/repository.go`
3. Add service methods to `internal/<package>/service.go`
4. Add handlers to `internal/<package>/routes.go`
5. Wire into `internal/server/server.go` if new package
6. Add integration tests to `tests/phase<N>/`

### Adding Database Migrations

Schema is in `internal/db/schema.go`:

```go
var schema = `
CREATE TABLE IF NOT EXISTS new_table (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_new_table_name ON new_table(name);
`
```

### Building for Production

```bash
# Build binary
CGO_ENABLED=1 go build -o sonos-hub ./cmd/sonos-hub

# Run with environment
./sonos-hub
```

Note: `CGO_ENABLED=1` is required for SQLite driver.

## Troubleshooting

### .env Parsing Issues

If you see errors like `export: not valid in this context`:

```bash
# Use this pattern instead of direct export
set -a && source .env && set +a && go run ./cmd/sonos-hub
```

### SQLite "database is locked"

Ensure WAL mode is enabled (done automatically in `db.Init`):

```go
db.Exec("PRAGMA journal_mode=WAL")
```

### Wrong Database Path (Go server using Node.js database)

**Symptoms:**
- Database endpoints hang (`/v1/dashboard`, `/v1/routines`, `/v1/scenes`)
- Non-database endpoints work (`/v1/health`, `/v1/devices`)
- Server logs show: `WARNING: SQLITE_DB_PATH appears to point to Node.js project`

**Cause:** Shell has `SQLITE_DB_PATH` exported with an absolute path to the Node.js project database. This overrides the `.env` file setting.

**Diagnosis:**
```bash
# Check which database the server is using
lsof -p $(pgrep -f "tmp/main") | grep ".db"

# Expected: /Users/.../sonos-hub-go/data/sonos-hub.db
# Wrong:    /Users/.../sonos-hub/data/sonos-hub.db
```

**Fix:**
```bash
# Option A: Unset and re-source (recommended)
unset SQLITE_DB_PATH
set -a && source .env && set +a && air

# Option B: Explicit override
SQLITE_DB_PATH=./data/sonos-hub.db air
```

**Prevention:** Always start the server from a fresh terminal or use `unset SQLITE_DB_PATH` before starting if you've been working on the Node.js project.

### Test Failures

- Check `ALLOW_TEST_MODE=true` is set
- Ensure temp directory is writable
- Run with `-v` for verbose output

## Development Environment Device Topology

The development network has the following Sonos devices for testing:

### Home Theater (192.168.1.10)
- **Master**: Sonos Arc SL (soundbar)
- **Surrounds**: 2x Sonos Play:1 (rear left/right at .17 and .29)
- **Subwoofer**: Sonos Sub Mini (at .76)
- **Logical representation**: 1 device with `physical_device_count: 4`, role `HOME_THEATER_MASTER`

### Kitchen Stereo Pair (192.168.1.12 or .13)
- **Left**: Sonos Play:1 (192.168.1.13)
- **Right**: Sonos Play:1 (192.168.1.12)
- **Coordinator**: Either speaker can be coordinator; IP may change
- **Logical representation**: 1 device with `physical_device_count: 2`, model suffix "(Stereo Pair)"

### Living Room (192.168.1.15)
- **Device**: Sonos Playbase (standalone)
- **Logical representation**: 1 device with `physical_device_count: 1`

### Device Detection Notes

- Device description XML contains 3 UDNs per device: root (`RINCON_xxx`), MediaServer (`_MS`), MediaRenderer (`_MR`). Always use the **root UDN** (first one) to match Node.js behavior.
- Zone topology uses short-form UDNs without suffixes for member matching.
- Stereo pair coordinator determination uses `channelMapSet` from zone topology.
- Home theater groups are identified by `isSatellite` and `isSubwoofer` flags on zone members.
- Static IPs in `.env` (`STATIC_DEVICE_IPS`) provide fallback for wired devices that don't respond to SSDP multicast.
