# Test Automation Plan (Phased, Production-Quality)

This plan establishes automated testing that runs continuously during migration. It prioritizes fast feedback, stable signal, and parity with the Node contract.

## Principles

- **Phased gates**: Each phase has a small, reliable test suite that must pass before moving on.
- **Go-native tooling**: Use `testing`, `httptest`, and `testify/require`.
- **No shell-only tests**: API tests are written in Go for deterministic behavior.
- **No new features**: Tests validate the existing contract only.

## Tooling Baseline

- `testing` + `httptest` for in-process HTTP tests.
- `github.com/stretchr/testify/require` for clear assertions.
- Integration tests use **build tags** and environment-gated execution.

## Phase Map

### Phase 0: Boot + Auth + Health (No devices)

**Goal**: Ensure server boots, health endpoints respond, and auth flows work.

Tests:
- `GET /v1/health`, `/v1/health/live`, `/v1/health/ready`
- `POST /v1/auth/pair/start`
- `POST /v1/auth/pair/complete`
- `POST /v1/auth/refresh`

Run:
```bash
go test ./tests/phase0 -run TestPhase0
```

### Phase 1: Discovery + Devices (Real devices)

**Goal**: Validate SSDP discovery and `/v1/devices*` behavior.

Prereqs:
- Sonos system on LAN
- `SONOS_TEST_DEVICE_ID` set to a known device in your environment (default: "Home Theater")

Run:
```bash
go test ./tests/phase1 -run TestPhase1 -tags=integration
```

### Phase 2: Sonos Playback + Volume + Groups + Players (Real devices)

**Goal**: Verify core control endpoints.

Run:
```bash
go test ./tests/phase2 -run TestPhase2 -tags=integration
```

### Phase 3: Scenes + Scheduler + Music Catalog

**Goal**: Verify job generation, scene execution, and catalog policies.

Run:
```bash
go test ./tests/phase3 -run TestPhase3
```

### Phase 4: Apple Music + Audit + Settings + Dashboard

**Goal**: Validate external integration and management endpoints.

Run:
```bash
go test ./tests/phase4 -run TestPhase4 -tags=integration
```

### Phase 5: Parity + Regression Suite

**Goal**: Side-by-side Node vs Go contract checks for all endpoints.

Run:
```bash
go test ./tests/parity -run TestParity -tags=integration
```

## Environment Conventions

Phase 0 tests use an in-process server with:
- `ALLOW_TEST_MODE=true`
- `NODE_ENV=development`
- `SQLITE_DB_PATH` pointing to a temp file

Integration phases require:
- `JWT_SECRET`
- `SONOS_TEST_DEVICE_ID`
- `DEFAULT_SONOS_IP` (if needed)

## Test Gates During Migration

- **Every implementation step must add/extend tests** in the active phase.
- No phase advances without a green suite.
- Failures must block feature work until fixed.

## Current Status

- Phase 0 tests implemented in `tests/phase0`.
- Phase 1+ to be added as we port device and Sonos integrations.
