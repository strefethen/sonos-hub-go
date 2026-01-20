package db

const schemaSQL = `
-- ===========================================================================
-- SCENES (from scene-engine)
-- ===========================================================================

CREATE TABLE IF NOT EXISTS scenes (
  scene_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  coordinator_preference TEXT NOT NULL DEFAULT 'ARC_FIRST',
  fallback_policy TEXT NOT NULL DEFAULT 'PLAYBASE_IF_ARC_TV_ACTIVE',
  members TEXT NOT NULL DEFAULT '[]',
  volume_ramp TEXT,
  teardown TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS scene_executions (
  scene_execution_id TEXT PRIMARY KEY,
  scene_id TEXT NOT NULL,
  idempotency_key TEXT,
  coordinator_used_udn TEXT,
  status TEXT NOT NULL DEFAULT 'STARTING',
  started_at TEXT NOT NULL DEFAULT (datetime('now')),
  ended_at TEXT,
  steps TEXT NOT NULL DEFAULT '[]',
  verification TEXT,
  error TEXT,
  FOREIGN KEY (scene_id) REFERENCES scenes(scene_id)
);

CREATE INDEX IF NOT EXISTS idx_executions_scene_id ON scene_executions(scene_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_executions_idempotency ON scene_executions(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_executions_status ON scene_executions(status);

-- ==========================================================================
-- ROUTINES (from scheduler)
-- ==========================================================================

CREATE TABLE IF NOT EXISTS routines (
  routine_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  timezone TEXT NOT NULL,
  schedule_type TEXT NOT NULL DEFAULT 'weekly',
  schedule_weekdays TEXT,
  schedule_month INTEGER,
  schedule_day INTEGER,
  schedule_time TEXT NOT NULL,
  holiday_behavior TEXT NOT NULL DEFAULT 'SKIP',
  scene_id TEXT NOT NULL,
  music_mode TEXT NOT NULL DEFAULT 'FIXED',
  music_policy_type TEXT NOT NULL DEFAULT 'FIXED',
  music_set_id TEXT,
  music_sonos_favorite_id TEXT,
  music_content_type TEXT,
  music_content_json TEXT,
  music_no_repeat_window INTEGER,
  music_no_repeat_window_minutes INTEGER,
  music_fallback_behavior TEXT,
  music_sonos_favorite_name TEXT,
  music_sonos_favorite_artwork_url TEXT,
  music_sonos_favorite_service_logo_url TEXT,
  music_sonos_favorite_service_name TEXT,
  arc_tv_policy TEXT,
  skip_next INTEGER NOT NULL DEFAULT 0,
  snooze_until TEXT,
  template_id TEXT,
  occasions_enabled INTEGER NOT NULL DEFAULT 1,
  speakers_json TEXT,
  last_run_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (scene_id) REFERENCES scenes(scene_id)
);

CREATE TABLE IF NOT EXISTS jobs (
  job_id TEXT PRIMARY KEY,
  routine_id TEXT NOT NULL,
  scheduled_for TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  scene_execution_id TEXT,
  retry_after TEXT,
  claimed_at TEXT,
  idempotency_key TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (routine_id) REFERENCES routines(routine_id) ON DELETE CASCADE,
  FOREIGN KEY (scene_execution_id) REFERENCES scene_executions(scene_execution_id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_routine_id ON jobs(routine_id);
CREATE INDEX IF NOT EXISTS idx_jobs_scheduled_for ON jobs(scheduled_for);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_scheduled_status ON jobs(scheduled_for, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_routine_scheduled ON jobs(routine_id, scheduled_for);
-- Note: idx_jobs_idempotency index is created in migrations after column is added

CREATE TABLE IF NOT EXISTS holidays (
  date TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  is_custom INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS routine_templates (
  template_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  category TEXT NOT NULL,
  sort_order INTEGER DEFAULT 0,
  icon TEXT,
  image_name TEXT,
  gradient_color_1 TEXT,
  gradient_color_2 TEXT,
  accent_color TEXT,
  timezone TEXT DEFAULT 'America/Los_Angeles',
  schedule_type TEXT DEFAULT 'weekly',
  schedule_weekdays TEXT,
  schedule_month INTEGER,
  schedule_day INTEGER,
  schedule_time TEXT NOT NULL,
  suggested_speakers TEXT,
  music_policy_type TEXT DEFAULT 'ROTATION',
  music_set_id TEXT,
  music_sonos_favorite_id TEXT,
  music_no_repeat_window_minutes INTEGER,
  music_fallback_behavior TEXT,
  holiday_behavior TEXT DEFAULT 'SKIP',
  arc_tv_policy TEXT DEFAULT 'SKIP',
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_routine_templates_category ON routine_templates(category, sort_order);

-- ==========================================================================
-- MUSIC CATALOG (from music-catalog)
-- ==========================================================================

CREATE TABLE IF NOT EXISTS music_sets (
  set_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  selection_policy TEXT NOT NULL CHECK (selection_policy IN ('ROTATION', 'SHUFFLE')),
  current_index INTEGER DEFAULT 0,
  occasion_start TEXT,
  occasion_end TEXT,
  artwork_url TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS set_items (
  set_id TEXT NOT NULL,
  sonos_favorite_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  added_at TEXT NOT NULL,
  service_logo_url TEXT,
  service_name TEXT,
  artwork_url TEXT,
  display_name TEXT,
  content_type TEXT DEFAULT 'sonos_favorite',
  content_json TEXT,
  PRIMARY KEY (set_id, sonos_favorite_id),
  FOREIGN KEY (set_id) REFERENCES music_sets(set_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_set_items_position ON set_items(set_id, position);

CREATE TABLE IF NOT EXISTS play_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sonos_favorite_id TEXT NOT NULL,
  set_id TEXT,
  routine_id TEXT,
  played_at TEXT NOT NULL,
  FOREIGN KEY (set_id) REFERENCES music_sets(set_id),
  FOREIGN KEY (routine_id) REFERENCES routines(routine_id)
);

CREATE INDEX IF NOT EXISTS idx_play_history_favorite ON play_history(sonos_favorite_id, played_at);
CREATE INDEX IF NOT EXISTS idx_play_history_set ON play_history(set_id, played_at);

-- ==========================================================================
-- AUDIT LOG (from audit-log)
-- ==========================================================================

CREATE TABLE IF NOT EXISTS audit_events (
  event_id TEXT PRIMARY KEY,
  timestamp TEXT NOT NULL,
  type TEXT NOT NULL,
  level TEXT NOT NULL,
  request_id TEXT,
  routine_id TEXT,
  job_id TEXT,
  scene_execution_id TEXT,
  udn TEXT,
  message TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_type ON audit_events(type);
CREATE INDEX IF NOT EXISTS idx_audit_events_level ON audit_events(level);
CREATE INDEX IF NOT EXISTS idx_audit_events_job_id ON audit_events(job_id) WHERE job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_events_routine_id ON audit_events(routine_id) WHERE routine_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_events_scene_execution_id ON audit_events(scene_execution_id) WHERE scene_execution_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_events_udn ON audit_events(udn) WHERE udn IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp_level ON audit_events(timestamp DESC, level);

-- ==========================================================================
-- SETTINGS (global app configuration)
-- ==========================================================================

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Insert default TV routing settings
INSERT OR IGNORE INTO settings (key, value) VALUES
  ('tv_routing_enabled', 'true'),
  ('tv_default_fallback_udn', ''),
  ('tv_default_policy', 'USE_FALLBACK');

-- ==========================================================================
-- SONOS CLOUD TOKENS (OAuth tokens for Sonos Cloud API)
-- ==========================================================================

CREATE TABLE IF NOT EXISTS sonos_cloud_tokens (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    scope TEXT NOT NULL,
    household_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
`
