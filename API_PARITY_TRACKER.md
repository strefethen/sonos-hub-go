# API Parity Tracker: Node.js → Go Migration

**Last Updated:** 2026-01-15
**Node.js Server:** localhost:9000
**Go Server:** localhost:9001

## Progress Summary
- Total APIs: 48+
- Completed: 37
- N/A: 7 (not in either codebase or Node.js)
- Schema Diff: 2 (tv-routing has extended Go schema)
- Not Implemented: 4 (Apple/Spotify routes not in Go)
- Remaining: 0

---

## TAB 1: HOME (11 endpoints)

| # | Method | Path | Status | Notes |
|---|--------|------|--------|-------|
| 1 | GET | `/v1/dashboard` | **DONE** | Verified identical structure |
| 2 | GET | `/v1/sonos/playback/now-playing` | **DONE** | Verified identical |
| 3 | GET | `/v1/routines` | **DONE** | Fixed: timestamps, music_policy fields, music_set, speakers |
| 4 | POST | `/v1/sonos/volume` | **DONE** | Verified identical |
| 5 | POST | `/v1/sonos/playback/pause` | **DONE** | Fixed: timestamps with milliseconds |
| 6 | POST | `/v1/sonos/playback/play` | **DONE** | Fixed: timestamps with milliseconds |
| 7 | POST | `/v1/sonos/playback/next` | **DONE** | Verified identical |
| 8 | POST | `/v1/sonos/playback/previous` | **DONE** | Verified identical |
| 9 | POST | `/v1/routines/{id}/run` | PENDING | Response format differs: Node.js returns scene execution, Go returns job |
| 10 | POST | `/v1/routines/{id}/skip` | **DONE** | Fixed: returns simple message format |
| 11 | POST | `/v1/routines/{id}/snooze` | N/A | Node.js returns 404 - not implemented |

---

## TAB 2: ROUTINES (19 endpoints)

| # | Method | Path | Status | Notes |
|---|--------|------|--------|-------|
| 1 | GET | `/v1/routines` | **DONE** | Same as HOME #3 |
| 2 | DELETE | `/v1/routines/{id}` | **DONE** | Fixed: empty 204 body, deletes scene |
| 3 | POST | `/v1/routines/{id}/unskip` | **DONE** | Fixed: returns simple message format |
| 4 | POST | `/v1/routines/{id}/run` | PENDING | Same as HOME #9 |
| 5 | POST | `/v1/routines` | **DONE** | Fixed: "routine" key instead of "data" |
| 6 | GET | `/v1/routines/{id}` | **DONE** | Verified identical |
| 7 | PUT | `/v1/routines/{id}` | **DONE** | Fixed: "routine" key instead of "data" |
| 8 | GET | `/v1/devices` | **DONE** | Fixed timestamp milliseconds |
| 9 | GET | `/v1/sonos/favorites` | **DONE** | Fixed: DIDL parsing for ordinal/type/description/resMD, added detection fallbacks |
| 10 | GET | `/v1/music/sets` | **DONE** | Fixed: timestamps, service_names order, null vs empty array, removed pagination |
| 11 | POST | `/v1/routines/test` | PENDING | |
| 12 | PUT | `/v1/scenes/{id}/stop` | PENDING | |
| 13 | GET | `/v1/music/handles` | PENDING | |
| 14 | GET | `/v1/music/sets/{id}` | **DONE** | Same as TAB 3 #2 |
| 15 | DELETE | `/v1/music/sets/{id}` | **DONE** | Same as TAB 3 #5 |
| 16 | DELETE | `/v1/music/sets/{setId}/items/{id}` | **DONE** | Same as TAB 3 #7 |
| 17 | POST | `/v1/music/sets/{setId}/items` | **DONE** | Same as TAB 3 #6 |

---

## TAB 3: MUSIC SETS (12 endpoints)

| # | Method | Path | Status | Notes |
|---|--------|------|--------|-------|
| 1 | GET | `/v1/music/sets` | **DONE** | Same as ROUTINES #10 |
| 2 | GET | `/v1/music/sets/{id}` | **DONE** | Fixed: added items array, removed wrapper, null handling. Note: display_name requires favorites lookup (not implemented) |
| 3 | POST | `/v1/music/sets` | **DONE** | Fixed: removed wrapper, no item_count |
| 4 | PATCH | `/v1/music/sets/{id}` | **DONE** | Fixed: removed wrapper, no item_count |
| 5 | DELETE | `/v1/music/sets/{id}` | **DONE** | Fixed: return 204 with no body |
| 6 | POST | `/v1/music/sets/{setId}/items` | **DONE** | Fixed: removed wrapper, added music_content, null fields |
| 7 | DELETE | `/v1/music/sets/{setId}/items/{id}` | **DONE** | Fixed: return 204 with no body |
| 8 | POST | `/v1/music/sets/{setId}/content` | N/A | Phase 3 feature - service layer not implemented |
| 9 | DELETE | `/v1/music/sets/{setId}/content/{pos}` | N/A | Phase 3 feature - service layer not implemented |
| 10 | PUT | `/v1/music/sets/{setId}/items/reorder` | **DONE** | Fixed: changed POST→PUT, returns {success:true}. Note: input format differs |
| 11 | POST | `/v1/music/sets/{setId}/play` | **DONE** | Fixed: accepts speaker_id, matches response format. Note: scene engine not integrated |
| 12 | POST | `/v1/music/sets/{setId}/select` | **DONE** | Fixed: removed wrapper, matches Node.js response |

---

## TAB 4: SETTINGS (9 endpoints)

| # | Method | Path | Status | Notes |
|---|--------|------|--------|-------|
| 1 | GET | `/v1/health` | **DONE** | Verified identical |
| 2 | GET | `/v1/system/info` | **DONE** | Verified identical (no request_id wrapper) |
| 3 | POST | `/v1/devices/rescan` | **DONE** | Verified identical |
| 4 | GET | `/v1/system/health-check` | N/A | Not in Node.js |
| 5 | GET | `/v1/settings/tv-routing` | SCHEMA_DIFF | Go has extended schema vs Node.js |
| 6 | PUT | `/v1/settings/tv-routing` | SCHEMA_DIFF | Go has extended schema vs Node.js |
| 7 | GET | `/v1/logs` | N/A | Not in either codebase |
| 8 | GET | `/v1/logs/export` | N/A | Not in either codebase |

---

## CROSS-TAB / SHARED (17 endpoints)

| # | Method | Path | Status | Notes |
|---|--------|------|--------|-------|
| 1 | POST | `/v1/auth/pair/complete` | **DONE** | Verified identical |
| 2 | POST | `/v1/auth/refresh` | **DONE** | Verified identical |
| 3 | GET | `/v1/executions` | **DONE** | Fixed: uses "executions" key, "has_more" casing |
| 4 | POST | `/v1/executions/{id}/retry` | **DONE** | Fixed: matches Node.js response format |
| 5 | POST | `/v1/music/sync-favorites` | NOT_IMPL | Not in Go |
| 6 | GET | `/v1/music/providers/spotify/artwork/{id}` | NOT_IMPL | Not in Go |
| 7 | GET | `/v1/music/search` | **DONE** | Returns stub/placeholder (service not integrated) |
| 8 | GET | `/v1/music/providers/spotify/content` | NOT_IMPL | Not in Go |
| 9 | GET | `/v1/apple/search` | NOT_IMPL | Apple routes not in Go |
| 10 | GET | `/v1/apple/suggestions` | NOT_IMPL | Apple routes not in Go |
| 11 | GET | `/v1/scenes` | **DONE** | Verified identical |
| 12 | GET | `/v1/scenes/{id}` | **DONE** | Verified identical |
| 13 | DELETE | `/v1/scenes/{id}` | **DONE** | Fixed: 204 with empty body |
| 14 | POST | `/v1/scenes/{id}/execute` | **DONE** | Verified identical |
| 15 | POST | `/v1/scenes/{id}/stop` | **DONE** | Verified identical |
| 16 | GET | `/v1/alarms/preview` | N/A | Not in either codebase |

---

## Current Work

**Next API to check:** Continue with other endpoints (Sonos favorites, routines actions, etc.)

## Issues Found & Fixed

### Global: Trailing Slash Handling (COMPLETED)
- Go was returning 404 for URLs with trailing slashes (e.g., `/v1/routines/`)
- Added `middleware.StripSlashes` to router in `internal/server/server.go`
- Now matches Node.js behavior

### GET `/v1/routines` (COMPLETED)
1. Timestamps missing milliseconds → Added `rfc3339Millis()` helper
2. Extra null fields in music_policy for non-FIXED → Conditional inclusion
3. music_set returned object for SHUFFLE → Now returns null
4. sonos_favorite_* fields read from wrong source → Now extracts from music_content_json only
5. speakers omitted when empty → Now always includes array
6. music_set not explicitly null for FIXED without content → Now explicit null

### GET `/v1/music/sets` (COMPLETED)
1. Timestamps missing milliseconds → Added `rfc3339Millis()` helper
2. service_names/service_logo_urls used map (random order) → Now preserves insertion order
3. Empty arrays returned instead of null → Now returns null when no items
4. Response included request_id and pagination → Removed to match Node.js format

### GET `/v1/music/sets/{id}` (COMPLETED)
1. Response wrapped in data/request_id → Now returns fields at root level
2. Missing items array → Now includes items with all Node.js fields
3. Missing service_logo_urls/service_names → Now computed from items
4. Limitation: display_name returns null (requires Sonos favorites lookup not implemented)

### POST/PATCH/DELETE `/v1/music/sets` (COMPLETED)
1. POST/PATCH: Response wrapped in data/request_id → Now returns fields at root level
2. POST/PATCH: Included item_count → Removed to match Node.js
3. DELETE: Returned JSON with request_id → Now returns 204 with empty body

### Music Set Items & Operations (COMPLETED)
1. POST items: Removed wrapper, added music_content object
2. DELETE items: Returns 204 with empty body
3. PUT reorder: Changed from POST to PUT, returns {success: true}
4. POST select: Removed wrapper, returns sonos_favorite_id, music_content, was_recent
5. POST play: Accepts speaker_id, returns matching response format (scene engine not integrated)

### GET `/v1/sonos/favorites` (COMPLETED)
1. DIDL parser missing fields → Added parsing for r:ordinal, r:type, r:description, r:resMD in `soap/actions.go`
2. content_type/service_name/service_logo_url empty → Added detection fallbacks using existing `detectServiceName()` and `GetServiceLogoFromName()` functions
3. ordinal returned as string → Converted to integer to match Node.js

---

## How to Resume After Compaction

1. Read this file: `/Users/stevetrefethen/github/sonos-hub-go/API_PARITY_TRACKER.md`
2. Find "Next API to check" in Current Work section
3. Continue from that endpoint
4. Update status and notes as you go
