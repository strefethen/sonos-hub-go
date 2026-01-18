# API Parity Matrix (Phase 0)

Date: 2025-02-14

This document compares the current Node monolith routes (`apps/sonos-hub/src/routes/*`) with the OpenAPI spec (`packages/openapi/openapi/sonos-hub.v1.yaml`). It identifies gaps that must be resolved before the Go migration proceeds.

## Summary

- OpenAPI spec is **incomplete** relative to the running Node monolith.
- Several implemented endpoints are **not represented** in OpenAPI.
- A few OpenAPI endpoints are **not implemented** in Node.
- Parameter naming in the OpenAPI spec does not always match current route params.

This must be reconciled before Go implementation to ensure strict parity.

## OpenAPI Paths (Spec Source of Truth Today)

From `packages/openapi/openapi/sonos-hub.v1.yaml`:

- `/v1/auth/pair/start`
- `/v1/auth/pair/complete`
- `/v1/auth/refresh`
- `/v1/dashboard`
- `/v1/devices`
- `/v1/devices/rescan`
- `/v1/sonos/favorites`
- `/v1/sonos/play/content`
- `/v1/sonos/play/validate`
- `/v1/sonos/services`
- `/v1/sonos/services/{service}/health`
- `/v1/scenes`
- `/v1/scenes/{scene_id}/execute`
- `/v1/routines`
- `/v1/jobs`
- `/v1/music/handles`
- `/v1/music/sets`
- `/v1/music/sets/{set_id}`
- `/v1/music/sets/{set_id}/items`
- `/v1/music/sets/{set_id}/items/{item_id}`
- `/v1/music/sets/{set_id}/select`
- `/v1/music/sets/backfill-logos`
- `/v1/apple/search`
- `/v1/audit/events`

## Node Monolith Paths (Implementation)

Extracted from `apps/sonos-hub/src/routes/*`:

### Auth

- `/v1/auth/pair/start`
- `/v1/auth/pair/complete`
- `/v1/auth/refresh`

### Dashboard

- `/v1/dashboard`

### Devices

- `/v1/devices`
- `/v1/devices/:device_id`
- `/v1/devices/rescan`
- `/v1/devices/topology`
- `/v1/devices/stats`

### Sonos Control

- `/v1/sonos/playback/stop`
- `/v1/sonos/playback/pause`
- `/v1/sonos/playback/play`
- `/v1/sonos/playback/next`
- `/v1/sonos/playback/previous`
- `/v1/sonos/playback/state`
- `/v1/sonos/playback/now-playing`
- `/v1/sonos/play`
- `/v1/sonos/play/favorite`
- `/v1/sonos/play/content`
- `/v1/sonos/play/validate`
- `/v1/sonos/volume`
- `/v1/sonos/volume/set`
- `/v1/sonos/volume/ramp`
- `/v1/sonos/alarms`
- `/v1/sonos/favorites`
- `/v1/sonos/groups` (GET)
- `/v1/sonos/groups` (POST)
- `/v1/sonos/groups/ungroup` (POST)
- `/v1/sonos/players` (GET)
- `/v1/sonos/players/:device_id/state`
- `/v1/sonos/players/:device_id/tv-status`
- `/v1/sonos/services`
- `/v1/sonos/services/:service/health`
- `/v1/sonos/verify/playback`

### Scenes + Executions

- `/v1/scenes` (POST)
- `/v1/scenes` (GET)
- `/v1/scenes/:scene_id` (GET)
- `/v1/scenes/:scene_id` (PUT)
- `/v1/scenes/:scene_id` (DELETE)
- `/v1/scenes/:scene_id/start` (POST)
- `/v1/scenes/:scene_id/stop` (POST)
- `/v1/scenes/:scene_id/execute` (POST)
- `/v1/scenes/:scene_id/executions` (GET)
- `/v1/scene-executions/:scene_execution_id` (GET)

### Routines

- `/v1/routines` (POST)
- `/v1/routines` (GET)
- `/v1/routines/:routine_id` (GET)
- `/v1/routines/:routine_id` (PUT)
- `/v1/routines/:routine_id` (DELETE)
- `/v1/routines/:routine_id/run` (POST)
- `/v1/routines/:routine_id/skip` (POST)
- `/v1/routines/:routine_id/unskip` (POST)
- `/v1/routines/test` (POST)

### Scheduler Executions

- `/v1/executions` (GET)
- `/v1/executions/:execution_id/retry` (POST)

### Music Catalog

- `/v1/music/sets` (GET)
- `/v1/music/sets/:setId` (GET)
- `/v1/music/sets` (POST)
- `/v1/music/sets/:setId` (PATCH)
- `/v1/music/sets/:setId` (DELETE)
- `/v1/music/sets/:setId/items` (POST)
- `/v1/music/sets/:setId/items/:sonosFavoriteId` (DELETE)
- `/v1/music/sets/:setId/items/reorder` (POST)
- `/v1/music/sets/:setId/play` (POST)
- `/v1/music/sets/:setId/select` (POST)
- `/v1/music/sets/:setId/content` (POST)
- `/v1/music/sets/:setId/content/:position` (DELETE)
- `/v1/music/sets/backfill-logos` (POST)

### Music Search

- `/v1/music/search` (GET)
- `/v1/music/suggestions` (GET)
- `/v1/music/providers` (GET)

### Apple Music

- `/v1/apple/search` (GET)
- `/v1/apple/item/:type/:id` (GET)
- `/v1/apple/browse/charts` (GET)
- `/v1/apple/recommendations` (GET)
- `/v1/apple/suggestions` (GET)
- `/v1/apple/import` (POST)
- `/v1/apple/token/status` (GET)
- `/v1/apple/token/refresh` (POST)

### Audit

- `/v1/audit/events` (GET)
- `/v1/audit/events/:event_id` (GET)
- `/v1/audit/events` (POST)

### Settings

- `/v1/settings/tv-routing` (GET)
- `/v1/settings/tv-routing` (PUT)

### Templates

- `/v1/routine-templates` (GET)
- `/v1/routine-templates/:template_id` (GET)

### Assets

- `/v1/assets/*` (static)

## Gaps and Mismatches

### Implemented in Node but Missing in OpenAPI

- `/v1/devices/:device_id`
- `/v1/devices/topology`
- `/v1/devices/stats`
- All `/v1/sonos/playback/*`, `/v1/sonos/volume/*`, `/v1/sonos/groups/*`, `/v1/sonos/players/*`, `/v1/sonos/alarms`, `/v1/sonos/verify/playback`
- `/v1/scenes/:scene_id` (GET/PUT/DELETE)
- `/v1/scenes/:scene_id/start`, `/v1/scenes/:scene_id/stop`, `/v1/scenes/:scene_id/executions`
- `/v1/scene-executions/:scene_execution_id`
- `/v1/routines/:routine_id` (GET/PUT/DELETE)
- `/v1/routines/:routine_id/run`, `/v1/routines/:routine_id/skip`, `/v1/routines/:routine_id/unskip`
- `/v1/routines/test`
- `/v1/executions` and `/v1/executions/:execution_id/retry`
- `/v1/music/search`, `/v1/music/suggestions`, `/v1/music/providers`
- Multiple `/v1/music/sets/:setId/*` endpoints not in spec
- `/v1/apple/*` endpoints other than `/v1/apple/search`
- `/v1/audit/events/:event_id` and `POST /v1/audit/events`
- `/v1/settings/tv-routing`
- `/v1/routine-templates/*`
- `/v1/assets/*`

### Present in OpenAPI but Not Implemented in Node

- `/v1/jobs`
- `/v1/music/handles`

### Parameter Naming Mismatches

- OpenAPI uses `{set_id}` and `{item_id}`; Node uses `:setId` and `:sonosFavoriteId`.
- OpenAPI uses `{scene_id}`; Node uses `:scene_id` in routes (compatible), but mixed `:id` is present in scene route definitions.

## Required Decisions Before Go Implementation

1. **OpenAPI must be updated** to reflect the Node monolith as the actual contract, or the Go implementation must follow an incomplete spec. Strict parity implies updating OpenAPI first.
2. **Unimplemented OpenAPI routes** (`/v1/jobs`, `/v1/music/handles`) must be either removed from spec or added to Node before porting.
3. **Parameter naming** should be normalized in the spec to match actual route parameters to avoid codegen conflicts.

## Next Action (Phase 0 Completion)

- Decide whether OpenAPI is the authoritative contract or the Node implementation is the authoritative contract.
- Update OpenAPI accordingly before proceeding with Go route implementation.
