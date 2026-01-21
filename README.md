# Sonos Hub Go

A high-performance LAN-only backend for Sonos smart home automation, written in Go. Designed for simplified deployment as a single binary with no external runtime dependencies.

## Features

### Core Capabilities
- **Device Discovery** — SSDP multicast discovery with static IP fallback for wired devices
- **Zone Topology** — Automatic detection of stereo pairs, home theater groups, and speaker capabilities
- **Device Health Tracking** — Real-time health status (OK/DEGRADED/OFFLINE) with missed scan counting
- **UPnP Event Subscriptions** — Real-time playback state tracking via Sonos event callbacks

### Automation
- **Scene Engine** — Multi-device scenes with volume ramping and coordinator selection
- **Routine Scheduler** — Multiple schedule types (weekly, monthly, yearly, one-time) with holiday awareness
- **Holiday Handling** — Skip, delay, or run routines on holidays with custom holiday support
- **Snooze & Skip** — Temporarily pause routines or skip the next occurrence
- **Routine Templates** — Pre-configured routines for quick setup with visual styling

### Music Management
- **Music Sets** — Curated playlists with rotation or shuffle selection policies
- **No-Repeat Algorithm** — Configurable time window to prevent recently played items from repeating
- **Occasion Sets** — Seasonal music tied to annual dates (birthdays, holidays, anniversaries)
- **Direct Content** — Play Apple Music and Spotify content without Sonos favorites (bypasses 70-favorite limit)

### Integrations
- **Sonos Cloud OAuth** — Cloud API access for favorites and household management
- **Apple Music** — Native MusicKit integration for playlist, album, and station playback
- **Arc TV Policy** — Smart handling when Arc soundbar is in TV mode (skip, fallback, or force play)

### Infrastructure
- **Stripe-Style API** — Consistent JSON responses with object types and cursor pagination
- **JWT Authentication** — Secure mobile app pairing with access/refresh token rotation
- **Audit Logging** — Track all system events with configurable retention
- **SQLite with WAL** — High-performance embedded database with write-ahead logging

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

The API follows [Stripe API conventions](https://stripe.com/docs/api) for consistent, predictable responses.

### Response Format

```json
// Single resource - returned directly with object type
{
  "object": "routine",
  "id": "rtn_abc123",
  "name": "Morning Music",
  "enabled": true,
  ...
}

// Collection - wrapped in list object with cursor pagination
{
  "object": "list",
  "data": [
    { "object": "routine", "id": "rtn_abc123", ... },
    { "object": "routine", "id": "rtn_def456", ... }
  ],
  "has_more": true,
  "url": "/v1/routines"
}

// Error - wrapped in error object with type classification
{
  "error": {
    "type": "invalid_request_error",
    "code": "resource_not_found",
    "message": "Routine not found"
  }
}
```

### Object Types

Every resource includes an `object` field for type identification:

| Object Type | Description |
|-------------|-------------|
| `routine` | Scheduled automation |
| `scene` | Multi-device playback configuration |
| `device` | Discovered Sonos speaker |
| `music_set` | Curated music collection |
| `music_set_item` | Item within a music set |
| `favorite` | Sonos favorite |
| `routine_template` | Pre-configured routine |
| `execution` | Scene execution result |
| `job` | Scheduled job instance |
| `now_playing` | Current playback state |
| `list` | Collection response |

### Pagination

List endpoints support cursor-based pagination:

```bash
# First page
GET /v1/routines?limit=20

# Next page (use id from last item)
GET /v1/routines?limit=20&starting_after=rtn_abc123

# Previous page
GET /v1/routines?limit=20&ending_before=rtn_def456
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
| GET | `/v1/devices/{udn}` | Get device details |
| POST | `/v1/devices/rescan` | Trigger device rescan |
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
| POST | `/v1/routines/{id}/snooze` | Snooze routine |
| POST | `/v1/routines/{id}/unsnooze` | Cancel snooze |
| POST | `/v1/routines/{id}/skip` | Skip next occurrence |
| POST | `/v1/routines/{id}/unskip` | Cancel skip |
| POST | `/v1/routines/{id}/restore` | Restore deleted routine |
| **Music** |||
| GET | `/v1/music/sets` | List music sets |
| POST | `/v1/music/sets` | Create music set |
| GET | `/v1/music/sets/{id}` | Get music set with items |
| PUT | `/v1/music/sets/{id}` | Update music set |
| DELETE | `/v1/music/sets/{id}` | Soft delete music set |
| POST | `/v1/music/sets/{id}/restore` | Restore deleted set |
| POST | `/v1/music/sets/{id}/items/sync` | Sync items (add/remove) |
| POST | `/v1/music/sets/{id}/items/reorder` | Reorder items |
| GET | `/v1/music/search` | Search music (Apple Music) |
| **Templates** |||
| GET | `/v1/routine-templates` | List routine templates |
| GET | `/v1/routine-templates/{id}` | Get template details |
| **Sonos Control** |||
| GET | `/v1/sonos/favorites` | List Sonos favorites |
| GET | `/v1/sonos/now-playing` | Get current playback state |
| POST | `/v1/sonos/{udn}/play` | Start playback |
| POST | `/v1/sonos/{udn}/pause` | Pause playback |
| POST | `/v1/sonos/{udn}/stop` | Stop playback |
| POST | `/v1/sonos/{udn}/volume` | Set volume |
| POST | `/v1/sonos/{udn}/play-favorite` | Play a Sonos favorite |
| **System** |||
| GET | `/v1/health` | Health check |
| GET | `/v1/system/info` | System information |
| GET | `/v1/dashboard` | Dashboard data |
| **Holidays** |||
| GET | `/v1/holidays` | List holidays for year |
| POST | `/v1/holidays` | Create custom holiday |
| DELETE | `/v1/holidays/{id}` | Delete custom holiday |

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

### Music Sets & Selection Algorithms

Music sets are curated collections of content that can be assigned to routines. Each set has a selection policy that determines how items are chosen during routine execution.

#### Selection Policies

| Policy | Behavior |
|--------|----------|
| `ROTATION` | Cycles through items sequentially (1 → 2 → 3 → 1...) |
| `SHUFFLE` | Random selection with optional no-repeat window |

#### No-Repeat Algorithm

When using shuffle mode, the no-repeat window prevents recently played items from being selected:

1. Query `play_history` for items played within the configured window (e.g., 24 hours)
2. Filter those items from the available selection pool
3. Randomly select from remaining items
4. If all items were recently played, fall back to full selection pool
5. Record the played item with timestamp for future filtering

```
Configuration: no_repeat_window_minutes = 1440 (24 hours)
Available items: [A, B, C, D, E]
Recently played: [B, D] (within last 24 hours)
Selection pool: [A, C, E]
Result: Random selection from [A, C, E]
```

#### Occasion Sets

Music sets can be tied to annual dates for seasonal content:

- `occasion_start`: Start date in MM-DD format (e.g., "12-01" for December 1st)
- `occasion_end`: End date in MM-DD format (e.g., "12-31" for December 31st)

During the occasion window, routines with `occasions_enabled: true` will prefer occasion-specific music sets over their default configuration.

### Routine Scheduler

The scheduler manages routine execution with sophisticated job generation and execution.

#### Schedule Types

| Type | Description | Example |
|------|-------------|---------|
| `WEEKLY` | Runs on specific weekdays | Mon, Wed, Fri at 7:00 AM |
| `MONTHLY` | Runs on a specific day of month | 15th of each month |
| `YEARLY` | Runs on a specific date annually | December 25th |
| `ONE_TIME` | Single execution | January 1, 2025 |

#### Holiday Handling

Each routine has a `holiday_behavior` setting:

| Behavior | Action |
|----------|--------|
| `SKIP` | Don't run on holidays |
| `DELAY` | Move to next non-holiday (searches up to 30 days) |
| `RUN` | Execute regardless of holiday |

Custom holidays can be added via the API alongside built-in US federal holidays.

#### Snooze & Skip

- **Snooze**: Temporarily pause a routine until a specific time (`snooze_until` timestamp)
- **Skip Next**: One-time flag to skip the next occurrence only (`skip_next` boolean)

#### Job Lifecycle

```
PENDING → SCHEDULED → CLAIMED → RUNNING → COMPLETED
                                       ↘ FAILED
                                       ↘ SKIPPED
                                       ↘ RETRYING
```

Jobs use idempotency keys (`routine_id:scheduled_for`) to prevent duplicate execution.

### Music Resolution Pipeline

When a routine executes, music content is resolved through a multi-step pipeline:

```
1. Check MusicPolicyType
   ├─ FIXED → Try DirectContent → Fall back to Sonos Favorite
   └─ ROTATION/SHUFFLE → Select from MusicSet → Try DirectContent → Fall back to Favorite

2. Build Sonos URI
   ├─ Sonos Favorite → Use stored URI directly
   └─ Direct Content → Construct x-rincon-cpcontainer or x-sonosapi-radio URI

3. Record play history (for no-repeat tracking)

4. Pass to Scene Executor with URI + metadata
```

This pipeline enables playing content from Apple Music or Spotify without requiring it to be saved as a Sonos favorite first.

### UPnP Event Subscriptions

The server maintains real-time subscriptions to Sonos device events for live playback state.

#### Subscribed Services

| Service | Events |
|---------|--------|
| AVTransport | Transport state, current track, position, URI |
| RenderingControl | Volume, mute state |
| ZoneGroupTopology | Group membership changes |

#### Subscription Management

- **Automatic Renewal**: Subscriptions renewed every 30 seconds (before 60-second expiry)
- **Exponential Backoff**: Failed subscriptions retry with increasing delays (30s → 60s → 120s → 240s → 480s → 600s max)
- **State Cache**: Device states cached with configurable TTL (default 30s) for fast dashboard queries

#### Event Types

```go
// AVTransportEvent
{
  "transport_state": "PLAYING",
  "current_track_uri": "x-rincon-cpcontainer:...",
  "track_duration": "0:03:45",
  "relative_time": "0:01:23"
}

// RenderingControlEvent
{
  "volume": 35,
  "muted": false
}
```

### Device Health Monitoring

Devices are continuously monitored for availability through periodic SSDP scans.

#### Health States

| State | Missed Scans | Description |
|-------|--------------|-------------|
| `OK` | 0 | Device responding normally |
| `DEGRADED` | 1-2 | Intermittent connectivity |
| `OFFLINE` | 3+ | Device unreachable |

Devices are automatically removed from the registry after 1440 missed scans (~24 hours).

#### Coordinator Capability

Not all Sonos devices can act as group coordinators. The system maintains a capability matrix:

- **Can Coordinate**: Arc, Beam, Playbar, Playbase, Play:5, Five, Era 100/300, Move, Roam, One/SL, Port, Amp
- **Cannot Coordinate**: Play:1, Play:3, Sub, Boost

### Arc TV Policy

When an Arc soundbar is in TV mode (HDMI input active), routines can be configured to handle this gracefully:

| Policy | Behavior |
|--------|----------|
| `SKIP` | Don't play music, skip this execution |
| `USE_FALLBACK` | Play on an alternate speaker |
| `ALWAYS_PLAY` | Force music playback (interrupts TV audio) |

### Dashboard

The dashboard endpoint (`GET /v1/dashboard`) provides a summary view optimized for mobile apps:

#### Next Up Routine

The soonest scheduled routine for today, including:
- Routine name and schedule time
- Target rooms (resolved from device topology)
- Music preview (artwork, title)
- Template information (if created from template)

#### Attention Items

Issues requiring user attention:

| Type | Description |
|------|-------------|
| `device_offline` | Number of offline devices |
| `failed_jobs` | Jobs that failed in the last 24 hours |
| `database_unhealthy` | SQLite connection issues |

Each attention item includes severity, message, and resolution hints.

### Routine Templates

Pre-configured routine templates enable quick setup of common use cases.

#### Template Properties

- **Category**: Grouping for UI display (morning, evening, workout, etc.)
- **Schedule**: Pre-filled schedule configuration
- **Music Policy**: Default music selection (FIXED, ROTATION, or SHUFFLE)
- **Visual Style**: Gradient colors, accent color, icon, and background image

#### Template Endpoint

```bash
# List templates by category
GET /v1/routine-templates?category=morning

# Get template images
GET /v1/assets/templates/{image_name}.jpg
```

## Design Principles

### Why Go?

- **Single Binary**: No runtime dependencies, just copy and run
- **Low Memory**: Typically uses 20-50MB RAM in production
- **Fast Startup**: Server ready in under 100ms
- **Concurrent by Default**: Goroutines handle parallel device communication
- **Strong Typing**: Compile-time safety for API contracts

### API Design

- **Stripe Conventions**: Predictable response shapes, cursor pagination, typed errors
- **Idempotent Operations**: Safe to retry requests (job creation, scene execution)
- **Soft Deletes**: Music sets support recovery via restore endpoint
- **Atomic Transactions**: Music selection index updates use SQLite transactions

### Device Communication

- **SOAP over HTTP**: UPnP control via XML-formatted requests
- **Event Subscriptions**: Real-time state via HTTP callbacks (SUBSCRIBE/NOTIFY)
- **Topology Awareness**: Automatic detection of stereo pairs, home theater groups

### Error Handling

- **Typed Errors**: All errors have `type`, `code`, and `message` fields
- **Graceful Degradation**: Non-critical failures logged but don't abort operations
- **Exponential Backoff**: Transient failures trigger automatic retries

## License

MIT

## Contributing

Contributions are welcome. Please ensure:

1. All tests pass (`go test ./...`)
2. Code follows Go conventions (`go fmt`, `go vet`)
3. New features include tests
