// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

// CurrentSchemaVersion is the current database schema version.
const CurrentSchemaVersion = 2

// Schema contains the SQL statements to create the database schema.
const Schema = `
-- ============================================================
-- RUNS: Top-level recipe executions
-- ============================================================
CREATE TABLE IF NOT EXISTS runs (
    id              TEXT PRIMARY KEY,           -- UUID
    recipe_name     TEXT NOT NULL,
    recipe_version  TEXT,
    recipe_hash     TEXT,                       -- Hash of recipe content (for cache invalidation)
    project_path    TEXT NOT NULL,

    -- Timing
    started_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at    TIMESTAMP,

    -- Status (includes 'retrying' for better UX - inspired by Prefect)
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK(status IN ('pending', 'running', 'retrying', 'completed',
                                     'failed', 'interrupted', 'cancelled')),

    -- Progress
    total_files     INTEGER DEFAULT 0,
    processed_files INTEGER DEFAULT 0,
    current_step    TEXT,
    current_file    TEXT,

    -- Retry tracking (inspired by Prefect)
    retry_count     INTEGER DEFAULT 0,

    -- Context
    git_commit      TEXT,                       -- HEAD at start
    git_branch      TEXT,
    bazel_version   TEXT,
    dry_run         BOOLEAN DEFAULT FALSE,

    -- Configuration
    config_json     TEXT,                       -- Recipe options

    -- Context outputs at run level (inspired by Airflow XCom fix)
    context_outputs TEXT,                       -- JSON: preserved across retries

    -- Result
    error_message   TEXT,
    changes_count   INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_recipe ON runs(recipe_name);
CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project_path);

-- ============================================================
-- CHECKPOINTS: Savepoints within runs
-- ============================================================
CREATE TABLE IF NOT EXISTS checkpoints (
    id              TEXT PRIMARY KEY,           -- UUID
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,

    -- Position
    step_id         TEXT NOT NULL,
    step_index      INTEGER NOT NULL,
    file_index      INTEGER NOT NULL,

    -- Timing
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Type
    checkpoint_type TEXT NOT NULL DEFAULT 'auto'
                    CHECK(checkpoint_type IN ('auto', 'manual', 'step_boundary')),

    -- State snapshot (with size tracking)
    state_json      TEXT,                       -- Serialized execution state
    state_size      INTEGER,                    -- Size in bytes for monitoring

    -- Metadata
    description     TEXT
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_run ON checkpoints(run_id);
CREATE INDEX IF NOT EXISTS idx_checkpoints_created ON checkpoints(created_at DESC);

-- ============================================================
-- FILE_STATES: Per-file processing status
-- ============================================================
CREATE TABLE IF NOT EXISTS file_states (
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    file_path       TEXT NOT NULL,

    -- Status (includes 'staged' for atomic updates)
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK(status IN ('pending', 'skipped', 'processing',
                                     'staged', 'completed', 'failed')),

    -- Change detection
    content_hash    TEXT,                       -- xxHash of file content before
    after_hash      TEXT,                       -- Hash after modification
    mtime           INTEGER,                    -- File modification time
    file_exists     BOOLEAN DEFAULT TRUE,       -- Track if file still exists

    -- Timing
    started_at      TIMESTAMP,
    completed_at    TIMESTAMP,

    -- Result
    changed         BOOLEAN DEFAULT FALSE,
    lines_added     INTEGER DEFAULT 0,
    lines_removed   INTEGER DEFAULT 0,
    error_message   TEXT,

    PRIMARY KEY (run_id, file_path)
);

CREATE INDEX IF NOT EXISTS idx_file_states_status ON file_states(run_id, status);

-- ============================================================
-- EVENT_LOG: Detailed event history (inspired by Temporal)
-- ============================================================
-- Separate from audit_log for better debugging and replay capability
CREATE TABLE IF NOT EXISTS event_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,

    -- Timing
    timestamp       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    sequence        INTEGER NOT NULL,           -- Monotonic sequence within run

    -- Event details
    event_type      TEXT NOT NULL,              -- Flexible: any event type
    file_path       TEXT,
    step_id         TEXT,

    -- Event data (JSON)
    event_data      TEXT
);

CREATE INDEX IF NOT EXISTS idx_event_log_run ON event_log(run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_event_log_timestamp ON event_log(timestamp DESC);

-- ============================================================
-- AUDIT_LOG: Human-readable change history
-- ============================================================
CREATE TABLE IF NOT EXISTS audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id          TEXT REFERENCES runs(id) ON DELETE CASCADE,

    -- Timing
    timestamp       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- What happened
    action          TEXT NOT NULL
                    CHECK(action IN ('file_modified', 'file_created', 'file_deleted',
                                     'checkpoint_created', 'checkpoint_restored',
                                     'run_started', 'run_completed', 'run_failed',
                                     'run_resumed', 'step_started', 'step_completed')),

    -- Details
    file_path       TEXT,
    step_id         TEXT,
    description     TEXT,
    diff_summary    TEXT,                       -- "+3 -1" style summary

    -- Before/after for debugging
    old_value       TEXT,
    new_value       TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_log_run ON audit_log(run_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_file ON audit_log(file_path);

-- ============================================================
-- SCHEMA_VERSION: For migrations
-- ============================================================
CREATE TABLE IF NOT EXISTS schema_version (
    version         INTEGER PRIMARY KEY,
    applied_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

// SchemaInitVersion is the SQL to check and initialize schema version.
const SchemaInitVersion = `
INSERT OR IGNORE INTO schema_version (version) VALUES (2);
`

// GetSchemaVersionSQL returns the query to get the current schema version.
const GetSchemaVersionSQL = `SELECT MAX(version) FROM schema_version;`

// SchemaMigrationV1ToV2 contains SQL statements to migrate from v1 to v2.
const SchemaMigrationV1ToV2 = `
-- Add new columns to runs table
ALTER TABLE runs ADD COLUMN recipe_hash TEXT;
ALTER TABLE runs ADD COLUMN retry_count INTEGER DEFAULT 0;
ALTER TABLE runs ADD COLUMN context_outputs TEXT;

-- Add new columns to checkpoints table
ALTER TABLE checkpoints ADD COLUMN state_size INTEGER;

-- Add new columns to file_states table
ALTER TABLE file_states ADD COLUMN after_hash TEXT;
ALTER TABLE file_states ADD COLUMN file_exists BOOLEAN DEFAULT TRUE;

-- Create event_log table
CREATE TABLE IF NOT EXISTS event_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    timestamp       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    sequence        INTEGER NOT NULL,
    event_type      TEXT NOT NULL,
    file_path       TEXT,
    step_id         TEXT,
    event_data      TEXT
);

CREATE INDEX IF NOT EXISTS idx_event_log_run ON event_log(run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_event_log_timestamp ON event_log(timestamp DESC);

-- Add new index for runs
CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project_path);

-- Add run_resumed action to audit_log (handled by CHECK constraint update)
-- Note: SQLite doesn't support modifying CHECK constraints, so we rely on
-- the application to enforce this. New databases get the updated constraint.

-- Update schema version
INSERT INTO schema_version (version) VALUES (2);
`
