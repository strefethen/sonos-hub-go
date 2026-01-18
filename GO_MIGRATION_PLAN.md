# Go Migration Plan (Production-Ready, No New Features)

Date: 2025-02-14

## Scope and Constraints

- **Goal**: 1:1 conversion of the existing Node monolith to Go with production-grade quality.
- **No new features or APIs**. No feature expansion during migration.
- **Strict parity preferred**. Any necessary compromises must be documented before implementation.
- **Platforms**: macOS (arm64 + amd64), Linux (arm64 + amd64), Windows (amd64).
- **Observability**: keep existing health endpoints and metrics behavior. No new monitoring requirements.

## Key Technical Decisions (Locked)

### SQLite Driver

- **Chosen driver**: `github.com/mattn/go-sqlite3`.
- **Rationale**: most widely used Go SQLite driver, longest track record, strongest community support, best compatibility with SQLite features and WAL mode.
- **Implications**:
  - Requires **CGO**.
  - Shipping will be **per-platform binaries** (standard for Go + CGO). This is consistent with the platform list above.

### HTTP Server Stack

- **Recommended**: `net/http` + `chi`.
- **Rationale**: stable, minimal, transparent, low risk.

### Metrics

- **Prometheus**: `github.com/prometheus/client_golang`.
- **Keep**: `/metrics` route and semantics used in the Node service.

### OpenAPI

- **Source of truth**: `packages/openapi/openapi/sonos-hub.v1.yaml`.
- **Go validation**: `kin-openapi` for request validation in dev/test; can be toggled off in production for perf if needed.
- **Types**: optionally generate Go types from the OpenAPI spec to enforce compile-time correctness.

## Migration Phases (Production Mindset)

### Phase 0: Parity Definition (Required Before Coding)

- **Inventory** all HTTP routes in `apps/sonos-hub/src/routes/*` and confirm they map to OpenAPI.
- **Document** any endpoints not present in OpenAPI and decide whether to keep or deprecate.
- **Behavioral parity matrix** for each route:
  - status codes
  - response shapes
  - error payloads
  - timeouts and retry behavior
  - idempotency semantics

Deliverable: Parity matrix + endpoint map.

### Phase 1: Go Monolith Skeleton

- Create new Go module under `server-go/` or `apps/sonos-hub-go/` (final name TBD).
- Implement:
  - config loading (env with defaults, validation)
  - logger + correlation IDs
  - Prometheus registry + `/metrics`
  - health endpoints (`/v1/health`, `/v1/health/live`, `/v1/health/ready`)

Deliverable: Minimal Go server that boots and exposes health + metrics.

### Phase 2: Database Layer (Schema + Migrations)

- Port schema from `apps/sonos-hub/src/db/schema.ts`.
- Apply WAL mode and foreign keys.
- Migrations:
  - `retry_after`, `claimed_at`, `idempotency_key` on `jobs`
  - `speakers_json` on `routines`
  - backfill logic for `speakers_json`

Deliverable: DB parity with Node including migrations and backfills.

### Phase 3: Auth (JWT)

- Port `apps/sonos-hub/src/lib/jwt.ts` behavior:
  - HS256
  - issuer/audience
  - access + refresh
  - payload validation

Deliverable: Auth endpoints parity `/v1/auth/*`.

### Phase 4: SOAP/UPnP Core (Highest Risk)

- Implement SOAP client with:
  - timeouts
  - error mapping based on Sonos error codes
  - XML parsing tolerant of namespaces and optional fields
- Port functionality from:
  - `apps/sonos-hub/src/services/sonos/soap-client.ts`
  - `apps/sonos-hub/src/services/sonos/metadata-parser.ts`
  - `apps/sonos-hub/src/services/sonos/uri-builder.ts`

Deliverable: SOAP client verified against recorded XML responses and real devices.

### Phase 5: Discovery (SSDP + HTTP Probe)

- Implement SSDP search with multi-pass M-SEARCH.
- Dedupe by USN and handle late responses.
- Implement fallback probing of static IPs.
- Port XML parsing from `apps/sonos-hub/src/discovery/parser.ts`.

Deliverable: Discovery parity with device topology parsing.

### Phase 6: Core Services

#### Device Registry

- Port `apps/sonos-hub/src/services/device-service.ts`:
  - device health tracking
  - topology merging
  - known IP caching

#### Scene Engine

- Port `apps/sonos-hub/src/services/scene/*`:
  - coordinator lock
  - orchestration steps
  - verification logic
  - preflight checks

#### Scheduler

- Port `apps/sonos-hub/src/services/scheduler/*`:
  - job generation
  - polling job runner
  - retries/backoff
  - holiday logic

#### Music Catalog

- Port `apps/sonos-hub/src/services/music/*`:
  - sets, items, play history
  - rotation/shuffle logic

#### Audit Log

- Port `apps/sonos-hub/src/services/audit/*`:
  - retention policy
  - prune job

Deliverable: Internal service layer parity.

### Phase 7: Apple Music Integration

- Port `apps/sonos-hub/src/services/apple-music/*`:
  - ES256 developer token generation
  - token caching and refresh
  - Apple Music API calls

Deliverable: Apple Music API parity.

### Phase 8: HTTP Routes + OpenAPI Compliance

- Implement route handlers for **all** paths in the OpenAPI spec.
- Add request validation in dev/test using OpenAPI schema.
- Confirm response shapes and status codes match parity matrix.

Deliverable: Full HTTP surface parity.

### Phase 9: Static Assets

- Port `apps/sonos-hub/src/routes/assets.ts`:
  - cache-control logic per asset type
  - static file serving under `/v1/assets/*`

Deliverable: Asset serving parity.

### Phase 10: Testing and Parity Verification

- Black-box API tests against Node and Go.
- SOAP parsing tests using recorded XML fixtures.
- Real-device smoke tests for SSDP + SOAP + scene execution.

Deliverable: Parity confidence.

## Known High-Risk Areas

- SOAP XML parsing: Sonos responses vary by model/firmware.
- SSDP behavior: multicast can be OS/network dependent.
- Time zones: DST and timezone shifts must match Luxon behavior.
- SQLite concurrency: WAL + background jobs must be stable.

## Foreseeable Compromises (Must Be Approved in Advance)

These should be considered only if they materially reduce risk or complexity:

- **SOAP parsing tolerance**: strict parsing may be replaced with token-based parsing to match Node’s leniency.
- **Timezone and holiday calculation differences**: if Go libraries differ from `luxon` + `date-holidays`, output must be validated and documented.
- **Binary distribution**: CGO-based SQLite driver implies per-platform builds rather than a single universal binary.

Any deviation from 1:1 behavior requires explicit approval.

## Deliverable Checklist

- [ ] Parity matrix and endpoint map finalized
- [ ] Go module skeleton with health + metrics
- [ ] SQLite schema and migrations complete
- [ ] Auth parity confirmed
- [ ] SOAP/UPnP core parity confirmed
- [ ] SSDP discovery parity confirmed
- [ ] Device registry + scene + scheduler parity confirmed
- [ ] Music catalog parity confirmed
- [ ] Apple Music parity confirmed
- [ ] OpenAPI validation passes for all routes
- [ ] Static asset serving parity confirmed
- [ ] Black-box tests vs Node baseline passing
