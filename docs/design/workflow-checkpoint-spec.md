# Workflow and Checkpointing System Specification

**Status:** Draft v2 (Updated with review findings)
**Author:** bazelbump contributors
**Created:** 2026-02-03
**Updated:** 2026-02-03
**Review:** See [Appendix A](#appendix-a-design-rationale-and-benchmarks) for comparison with Temporal, DBOS, Prefect, Airflow, Bazel

## Summary

This specification defines a workflow and checkpointing system for bazelbump that enables:
- **Progress tracking** with real-time feedback
- **Checkpointing** for resumable execution
- **Incremental execution** to skip already-processed files
- **Audit trail** for change history
- **State persistence** across runs

## Motivation

Testing on Envoy revealed the need for robust workflow management:
- **1,696 BUILD files** scanned
- **893 files** would be modified
- **967 lines** would be added

For projects this large, users need:
1. Progress feedback ("Processing 423/893 files...")
2. Resume capability if interrupted
3. Incremental re-runs that skip unchanged files
4. History of what changed and when

---

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        bazelbump CLI                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │   Recipe    │  │  Progress   │  │    Checkpoint           │  │
│  │  Executor   │──│  Reporter   │──│    Manager              │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│         │                │                    │                  │
│         │                │                    │                  │
│         ▼                ▼                    ▼                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    State Manager                         │    │
│  │  ┌─────────┐  ┌──────────┐  ┌──────────┐  ┌─────────┐  │    │
│  │  │  Runs   │  │Checkpoints│  │  Files   │  │  Audit  │  │    │
│  │  └─────────┘  └──────────┘  └──────────┘  └─────────┘  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                              │                                   │
│                              ▼                                   │
│                    ┌─────────────────┐                          │
│                    │ .bazelbump/     │                          │
│                    │   state.db      │                          │
│                    │   snapshots/    │                          │
│                    └─────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Responsibility |
|-----------|---------------|
| **Recipe Executor** | Runs recipe steps, emits progress events |
| **Progress Reporter** | Displays real-time progress to user |
| **Checkpoint Manager** | Creates/restores checkpoints |
| **State Manager** | Persists state to SQLite |

---

## 2. State Persistence

### 2.1 Storage Location

```
.bazelbump/
├── state.db          # SQLite database (runs, checkpoints, audit)
├── snapshots/        # File backups (existing pkg/track system)
└── locks/
    └── state.lock    # File lock for concurrent access
```

### 2.2 Why SQLite?

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| SQLite | ACID, queryable, single file | ~5MB binary size | ✅ Chosen |
| JSON files | Simple, no deps | No ACID, hard to query | ❌ |
| BoltDB | Pure Go, embedded | Less familiar, no SQL | ❌ |

See [Appendix A.2](#a2-storage-approach-comparison) for detailed comparison.

**IMPORTANT: No CGO Required**

We use [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) - a **pure Go** SQLite implementation:

- ❌ **No CGO** - Cross-compiles to all Go targets (darwin, linux, windows, arm64, etc.)
- ✅ **Battle-tested** - Used by Gogs in production for 2+ years
- ✅ **Actively maintained** - Updated to SQLite 3.49.0 (Feb 2025)
- ⚡ **Fast enough** - ~2x slower than CGO version, but <50ms for our checkpoint operations

```go
import (
    "database/sql"
    _ "modernc.org/sqlite"  // Pure Go, no CGO!
)

db, err := sql.Open("sqlite", ".bazelbump/state.db")
```

This aligns with our **airgap-first** philosophy - single binary, no system dependencies.

### 2.3 SQLite Configuration (Critical for Durability)

**CRITICAL**: Default SQLite settings can lose data on power failure. We MUST configure for durability:

```go
func configureSQLite(db *sql.DB) error {
    pragmas := []string{
        // CRITICAL: Full sync for durability (default NORMAL can lose data)
        "PRAGMA synchronous=FULL",

        // WAL mode for concurrent reads during writes
        "PRAGMA journal_mode=WAL",

        // Limit WAL file size to prevent unbounded growth
        "PRAGMA wal_autocheckpoint=100",      // Checkpoint every 100 pages
        "PRAGMA journal_size_limit=5242880",  // Limit WAL to 5MB

        // Wait up to 5s for locks (handles concurrent access)
        "PRAGMA busy_timeout=5000",

        // Foreign key enforcement
        "PRAGMA foreign_keys=ON",
    }

    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            return fmt.Errorf("failed to set %s: %w", pragma, err)
        }
    }

    return nil
}
```

**Rationale**: See [Appendix A.6](#a6-sqlite-wal-mode-analysis) for why `synchronous=FULL` is required.

### 2.4 Optional Feature (Build Tags)

The checkpointing system is **optional** and can be disabled at compile time using Go build tags (similar to Rust's feature flags):

```bash
# Full build (default) - includes SQLite checkpointing
go build ./cmd/bazelbump

# Minimal build - no SQLite, no checkpointing (~5-10MB smaller binary)
go build -tags nostate ./cmd/bazelbump
```

**Implementation Pattern:**

```go
// pkg/state/manager.go - Interface (always included)
package state

type Manager interface {
    CreateRun(recipe string, opts RunOptions) (*Run, error)
    UpdateProgress(runID string, processed, total int) error
    CreateCheckpoint(opts CheckpointOptions) (*Checkpoint, error)
    GetLatestCheckpoint(runID string) (*Checkpoint, error)
    // ... etc
}

// Feature detection
func Available() bool  // Returns true if state management is available
```

```go
// pkg/state/manager_sqlite.go
//go:build !nostate

package state

import _ "modernc.org/sqlite"

func New(dbPath string) (Manager, error) {
    return &sqliteManager{...}, nil
}

func Available() bool { return true }
```

```go
// pkg/state/manager_noop.go
//go:build nostate

package state

// No-op implementation - all operations succeed silently
type noopManager struct{}

func New(dbPath string) (Manager, error) {
    return &noopManager{}, nil
}

func Available() bool { return false }

func (m *noopManager) CreateCheckpoint(...) (*Checkpoint, error) {
    return nil, nil // No-op
}
```

**CLI Behavior:**

| Build | `--resume` flag | `bazelbump status` |
|-------|-----------------|-------------------|
| Default | Works normally | Shows run history |
| `nostate` | Warns "checkpointing disabled" | Shows "state management not available" |

**Use Cases for Minimal Build:**
- CI environments where checkpointing isn't needed
- Embedded/constrained environments
- Users who prefer smaller binaries
- Simple one-off migrations

### 2.5 Database Schema

```sql
-- ============================================================
-- RUNS: Top-level recipe executions
-- ============================================================
CREATE TABLE runs (
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
    changed_files   INTEGER DEFAULT 0,
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
    error_message   TEXT
);

CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_started ON runs(started_at DESC);
CREATE INDEX idx_runs_project ON runs(project_path);

-- ============================================================
-- CHECKPOINTS: Savepoints within runs
-- ============================================================
CREATE TABLE checkpoints (
    id              TEXT PRIMARY KEY,           -- UUID
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,

    -- Position
    step_id         TEXT NOT NULL,
    step_index      INTEGER NOT NULL,
    files_processed INTEGER NOT NULL,

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

CREATE INDEX idx_checkpoints_run ON checkpoints(run_id);
CREATE INDEX idx_checkpoints_created ON checkpoints(created_at DESC);

-- ============================================================
-- FILE_STATES: Per-file processing status
-- ============================================================
CREATE TABLE file_states (
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    file_path       TEXT NOT NULL,

    -- Status
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK(status IN ('pending', 'skipped', 'in_progress',
                                     'staged', 'completed', 'failed')),

    -- Change detection
    content_hash    TEXT,                       -- xxHash of file content
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

CREATE INDEX idx_file_states_status ON file_states(run_id, status);

-- ============================================================
-- EVENT_LOG: Detailed event history (inspired by Temporal)
-- ============================================================
-- Separate from audit_log for better debugging and replay capability
CREATE TABLE event_log (
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

CREATE INDEX idx_event_log_run ON event_log(run_id, sequence);
CREATE INDEX idx_event_log_timestamp ON event_log(timestamp DESC);

-- ============================================================
-- AUDIT_LOG: Human-readable change history
-- ============================================================
CREATE TABLE audit_log (
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
    details_json    TEXT                        -- Flexible JSON details
);

CREATE INDEX idx_audit_log_run ON audit_log(run_id);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_log_file ON audit_log(file_path);

-- ============================================================
-- SCHEMA_VERSION: For migrations
-- ============================================================
CREATE TABLE schema_version (
    version         INTEGER PRIMARY KEY,
    applied_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO schema_version (version) VALUES (2);
```

---

## 3. State Machine

### 3.1 Run States

```
                         User: --restart
                              │
                              ▼
┌──────────┐    start    ┌─────────┐    complete    ┌───────────┐
│ pending  │────────────▶│ running │───────────────▶│ completed │
└──────────┘             └─────────┘                └───────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
              ▼               ▼               ▼
        ┌──────────┐   ┌─────────────┐   ┌────────────┐
        │  failed  │   │ interrupted │   │  retrying  │
        └──────────┘   └─────────────┘   └────────────┘
                              │               │
                              │    User: --resume
                              │               │
                              ▼               ▼
                         ┌─────────────────────┐
                         │       running       │
                         └─────────────────────┘
```

### 3.2 File States

```
                    ┌───────────────────────────────────────┐
                    │          Atomic File Update           │
                    │  (write temp → staged → rename)       │
                    └───────────────────────────────────────┘
                                      │
┌─────────┐   scan    ┌─────────────┐ │ write    ┌────────┐  rename   ┌───────────┐
│ pending │──────────▶│ in_progress │─┴─────────▶│ staged │──────────▶│ completed │
└─────────┘           └─────────────┘             └────────┘           └───────────┘
     │                      │                          │
     │ unchanged            │ error                    │ error
     ▼                      ▼                          ▼
┌─────────┐           ┌────────┐                 ┌────────┐
│ skipped │           │ failed │                 │ failed │
└─────────┘           └────────┘                 └────────┘
```

**Key Addition**: The `staged` state enables atomic file updates. See [Section 10.4](#104-atomic-file-updates).

---

## 4. Checkpoint System

### 4.1 Checkpoint Types

| Type | Trigger | Use Case |
|------|---------|----------|
| **auto** | Every N files (default: 100) | Crash recovery |
| **step_boundary** | Between recipe steps | Step-level resume |
| **manual** | User command | Explicit savepoint |

### 4.2 Checkpoint Content

```json
{
  "checkpoint_id": "cp-2026-02-03-143022-abc123",
  "run_id": "run-2026-02-03-142500-xyz789",
  "created_at": "2026-02-03T14:30:22Z",
  "checkpoint_type": "auto",

  "position": {
    "step_id": "detect-native-rules",
    "step_index": 0,
    "files_processed": 423,
    "total_files": 893
  },

  "execution_state": {
    "scan_results": { /* partial results */ },
    "pending_operations": [ /* remaining ops */ ],
    "context_outputs": { /* step outputs so far */ }
  },

  "statistics": {
    "files_processed": 423,
    "files_modified": 187,
    "lines_added": 412,
    "elapsed_ms": 34500
  }
}
```

### 4.3 Checkpoint Size Management

**Problem**: For large projects, checkpoint state can grow to several MB, causing storage bloat.

**Solution**: Implement size limits and cleanup:

```go
const (
    MaxCheckpointSize  = 1 * 1024 * 1024  // 1MB warning threshold
    MaxCheckpointsPerRun = 10             // Keep last 10 checkpoints
)

func (m *sqliteManager) CreateCheckpoint(opts CheckpointOptions) (*Checkpoint, error) {
    stateJSON, err := json.Marshal(opts.State)
    if err != nil {
        return nil, err
    }

    // Warn if state is too large
    if len(stateJSON) > MaxCheckpointSize {
        log.Warnf("Checkpoint state is large (%d bytes), consider reducing scan scope", len(stateJSON))
    }

    // Create checkpoint with size tracking
    cp := &Checkpoint{
        ID:        generateID("cp"),
        StateJSON: string(stateJSON),
        StateSize: len(stateJSON),
        // ...
    }

    // ... insert checkpoint ...

    // Cleanup old checkpoints (keep last N)
    if err := m.cleanupOldCheckpoints(opts.RunID, MaxCheckpointsPerRun); err != nil {
        log.Warnf("Failed to cleanup old checkpoints: %v", err)
    }

    return cp, nil
}

func (m *sqliteManager) cleanupOldCheckpoints(runID string, keepLast int) error {
    _, err := m.db.Exec(`
        DELETE FROM checkpoints
        WHERE run_id = ?
        AND id NOT IN (
            SELECT id FROM checkpoints
            WHERE run_id = ?
            ORDER BY created_at DESC
            LIMIT ?
        )
    `, runID, runID, keepLast)
    return err
}
```

### 4.4 Auto-Checkpoint Configuration

```bash
# Default: checkpoint every 100 files
bazelbump recipe run autoload-migration

# Custom interval
bazelbump recipe run autoload-migration --checkpoint-interval 50

# Disable auto-checkpoints
bazelbump recipe run autoload-migration --checkpoint-interval 0

# Time-based (every 5 minutes)
bazelbump recipe run autoload-migration --checkpoint-time 5m
```

---

## 5. Progress Tracking

### 5.1 Progress Reporter Interface

```go
// ProgressReporter receives execution events
type ProgressReporter interface {
    // Run lifecycle
    OnRunStart(run *Run)
    OnRunComplete(run *Run, result *Result)
    OnRunError(run *Run, err error)

    // Step lifecycle
    OnStepStart(step Step)
    OnStepProgress(step Step, processed, total int)
    OnStepComplete(step Step, result *StepResult)

    // File lifecycle
    OnFileStart(path string)
    OnFileComplete(path string, change *FileChange)
    OnFileSkipped(path string, reason string)
    OnFileError(path string, err error)

    // Checkpoints
    OnCheckpointCreated(cp *Checkpoint)
    OnCheckpointRestored(cp *Checkpoint)
}
```

### 5.2 Built-in Reporters

```go
// Terminal progress (default)
type TerminalReporter struct {
    spinner    *spinner.Spinner
    progressBar *progressbar.ProgressBar
    verbose    bool
}

// JSON streaming reporter (for CI/tooling)
type JSONStreamReporter struct {
    writer io.Writer
    encoder *json.Encoder
}

// Silent reporter (for scripting)
type SilentReporter struct{}

// Composite reporter (multiple outputs)
type CompositeReporter struct {
    reporters []ProgressReporter
}
```

### 5.3 Terminal Output

```
==> Running recipe: autoload-migration (resuming from checkpoint)

Step 1/3: Detect native rules
├─ Scanning BUILD files...
│  Progress: 423/893 files (47%)
│  [████████████░░░░░░░░░░░░░░] ETA: 2m 34s
│  Current: source/server/config/BUILD.bazel
│
│  Found: 1,823 cc_library | 456 cc_test | 89 proto_library
│
├─ ✓ Checkpoint saved: step-1-file-400 (auto)
│
└─ ✓ Completed in 4m 12s (893 files scanned, 2,368 rules found)

Step 2/3: Add module dependencies
├─ Updating MODULE.bazel...
└─ ✓ Completed in 0.2s (3 deps added)

Step 3/3: Add load statements
├─ Processing BUILD files...
│  Progress: 234/893 files (26%)
│  [██████░░░░░░░░░░░░░░░░░░░░] ETA: 1m 45s
│  Current: test/extensions/filters/http/BUILD
^C
│
└─ ⚠ Interrupted at file 234/893
   Run 'bazelbump recipe run autoload-migration --resume' to continue
```

---

## 6. Incremental Execution

### 6.1 Change Detection with Recipe Hash

**Key Insight from Bazel**: Cache keys must include both content hash AND action definition. If the recipe changes, cached results are invalid.

```go
// ChangeDetector determines if a file needs reprocessing
type ChangeDetector interface {
    // HasChanged returns true if file needs reprocessing
    HasChanged(path string, lastRun *Run) (bool, error)

    // ComputeHash computes content hash for a file
    ComputeHash(path string) (string, error)

    // ComputeRecipeHash computes hash of recipe definition
    ComputeRecipeHash(recipe *Recipe) (string, error)
}

// CacheKey for determining if processing is needed
type CacheKey struct {
    FilePath      string
    ContentHash   string
    RecipeHash    string  // Hash of recipe content
    RecipeVersion string
}

func (d *XXHashDetector) HasChanged(path string, lastRun *Run) (bool, error) {
    // Check if recipe changed (invalidates all cache)
    if lastRun.RecipeHash != d.currentRecipeHash {
        return true, nil  // Recipe changed, must reprocess
    }

    // Check if file changed
    lastState, err := d.stateManager.GetFileState(lastRun.ID, path)
    if err != nil || lastState == nil {
        return true, nil // Never processed
    }

    // Quick check: mtime (avoids hash computation)
    stat, err := os.Stat(path)
    if err != nil {
        return true, nil // File may have been deleted
    }
    if stat.ModTime().Unix() == lastState.Mtime {
        return false, nil // Fast path: unchanged
    }

    // Full check: content hash
    currentHash, err := d.ComputeHash(path)
    if err != nil {
        return true, err
    }

    return currentHash != lastState.ContentHash, nil
}
```

### 6.2 Incremental Modes

```bash
# Default: Skip unchanged files from last successful run
bazelbump recipe run autoload-migration

# Force full re-run (ignore previous state)
bazelbump recipe run autoload-migration --restart

# Continue from last checkpoint (same run)
bazelbump recipe run autoload-migration --resume

# Incremental from specific run
bazelbump recipe run autoload-migration --since run-abc123
```

---

## 7. CLI Commands

### 7.1 Recipe Run (Enhanced)

```bash
bazelbump recipe run <recipe> [flags]

Flags:
  --resume              Resume from last checkpoint (default if interrupted run exists)
  --restart             Start fresh, ignore existing state
  --checkpoint-interval Checkpoint every N files (default: 100, 0 to disable)
  --checkpoint-time     Checkpoint every duration (e.g., "5m")
  --incremental         Skip unchanged files (default: true)
  --since <run-id>      Use specific run as baseline for incremental
  --progress            Show detailed progress (default: true for TTY)
  --no-progress         Disable progress output
```

### 7.2 Status Command (Enhanced)

```bash
bazelbump status [flags]

Flags:
  --run <id>      Show specific run
  --progress      Live progress for running execution
  --json          JSON output
  --export        Export run state to JSON file

Examples:
  bazelbump status                    # Current/last run summary
  bazelbump status --progress         # Live progress display
  bazelbump status --run run-abc123   # Specific run details
  bazelbump status --export run.json  # Export for debugging
```

### 7.3 History Command

```bash
bazelbump history [flags]

Flags:
  --run <id>       Details of specific run
  --since <time>   Runs since time (e.g., "24h", "7d")
  --recipe <name>  Filter by recipe
  --status <s>     Filter by status
  --limit <n>      Limit results (default: 10)
  --json           JSON output
```

### 7.4 Checkpoint Commands

```bash
bazelbump checkpoint list [flags]
bazelbump checkpoint create [flags]
bazelbump checkpoint restore <id> [flags]
bazelbump checkpoint export <id> [flags]    # Export checkpoint to JSON
bazelbump checkpoint delete <id> [flags]

Flags:
  --run <id>      Target run (default: current/last)
  --json          JSON output
  --force         Skip confirmation for destructive ops
```

---

## 8. Error Handling

### 8.1 Sentinel Errors

**Rationale**: Consistent error handling improves API usability. See [Appendix A.5](#a5-api-design-insights).

```go
package state

import "errors"

// Sentinel errors for consistent error handling
var (
    ErrNotFound         = errors.New("not found")
    ErrRunNotFound      = errors.New("run not found")
    ErrCheckpointNotFound = errors.New("checkpoint not found")
    ErrAlreadyCompleted = errors.New("run already completed")
    ErrLockHeld         = errors.New("another instance is running")
    ErrStateCorrupted   = errors.New("state database corrupted")
)

// Example usage
func (m *sqliteManager) GetRun(id string) (*Run, error) {
    row := m.db.QueryRow("SELECT ... FROM runs WHERE id = ?", id)

    var run Run
    err := row.Scan(...)
    if err == sql.ErrNoRows {
        return nil, ErrRunNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return &run, nil
}

// Caller can check with errors.Is
run, err := manager.GetRun(id)
if errors.Is(err, state.ErrRunNotFound) {
    // Handle not found case
}
```

### 8.2 Resume Semantics Decision Tree

```
bazelbump recipe run <recipe> [flags]
                │
                ▼
    ┌───────────────────────┐
    │ Check for --restart   │
    └───────────────────────┘
                │
        ┌───────┴───────┐
        │ yes           │ no
        ▼               ▼
    ┌───────┐   ┌───────────────────────┐
    │ Fresh │   │ Check for --resume    │
    │ Start │   └───────────────────────┘
    └───────┘               │
                    ┌───────┴───────┐
                    │ yes           │ no
                    ▼               ▼
            ┌───────────────┐   ┌───────────────────────┐
            │ Find last     │   │ Find last completed   │
            │ interrupted   │   │ run for incremental   │
            │ run           │   └───────────────────────┘
            └───────────────┘               │
                    │               ┌───────┴───────┐
            ┌───────┴───────┐       │ found         │ not found
            │ found         │ not   ▼               ▼
            ▼               │ found ┌───────────┐   ┌───────┐
    ┌───────────────┐       │       │Incremental│   │ Fresh │
    │ Resume from   │       │       │   Run     │   │ Start │
    │ checkpoint    │       │       └───────────┘   └───────┘
    └───────────────┘       │
                            ▼
                    ┌───────────────┐
                    │ Error: no run │
                    │ to resume     │
                    └───────────────┘
```

---

## 9. Performance Optimizations

### 9.1 Batched Database Writes

**Problem**: Individual writes per file cause 2,679+ writes for Envoy (893 files × 3 writes each).

**Solution**: Batch writes and flush periodically:

```go
type BatchedStateManager struct {
    Manager

    pendingFileStates []FileState
    pendingEvents     []EventLogEntry

    flushThreshold    int           // Flush every N operations
    flushInterval     time.Duration // Flush every N seconds
    lastFlush         time.Time

    mu sync.Mutex
}

func (m *BatchedStateManager) SetFileState(runID, path string, status FileStatus, hash string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.pendingFileStates = append(m.pendingFileStates, FileState{
        RunID:       runID,
        FilePath:    path,
        Status:      status,
        ContentHash: hash,
    })

    if len(m.pendingFileStates) >= m.flushThreshold ||
       time.Since(m.lastFlush) >= m.flushInterval {
        return m.flush()
    }

    return nil
}

func (m *BatchedStateManager) flush() error {
    if len(m.pendingFileStates) == 0 && len(m.pendingEvents) == 0 {
        return nil
    }

    tx, err := m.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Bulk insert file states using upsert (idempotent - inspired by DBOS)
    stmt, err := tx.Prepare(`
        INSERT INTO file_states (run_id, file_path, status, content_hash)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(run_id, file_path) DO UPDATE SET
            status = excluded.status,
            content_hash = COALESCE(excluded.content_hash, file_states.content_hash),
            completed_at = CASE WHEN excluded.status = 'completed' THEN CURRENT_TIMESTAMP ELSE file_states.completed_at END
        WHERE file_states.status != 'completed'
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, fs := range m.pendingFileStates {
        if _, err := stmt.Exec(fs.RunID, fs.FilePath, fs.Status, fs.ContentHash); err != nil {
            return err
        }
    }

    // Bulk insert events
    // ... similar pattern ...

    if err := tx.Commit(); err != nil {
        return err
    }

    m.pendingFileStates = m.pendingFileStates[:0]
    m.pendingEvents = m.pendingEvents[:0]
    m.lastFlush = time.Now()

    return nil
}
```

### 9.2 Optimized Hash Computation

**Problem**: Computing hash before AND after each file doubles I/O.

**Solution**: Use mtime as initial filter, only hash when needed:

```go
func (d *XXHashDetector) HasChangedOptimized(path string, lastState *FileState) (bool, error) {
    stat, err := os.Stat(path)
    if err != nil {
        if os.IsNotExist(err) {
            return false, nil // File deleted, skip
        }
        return true, err
    }

    // Fast path: mtime unchanged
    if lastState != nil && stat.ModTime().Unix() == lastState.Mtime {
        return false, nil
    }

    // Slow path: compute hash
    hash, err := d.ComputeHash(path)
    if err != nil {
        return true, err
    }

    if lastState == nil {
        return true, nil
    }

    return hash != lastState.ContentHash, nil
}
```

### 9.3 Benchmark Targets

| Operation | Target | Notes |
|-----------|--------|-------|
| Checkpoint create | < 50ms | Serialize + write to SQLite |
| Checkpoint restore | < 100ms | Read + deserialize |
| File state lookup | < 1ms | Indexed query |
| Batch flush (100 files) | < 50ms | Single transaction |
| Hash computation | < 5ms/file | xxHash + mtime filter |
| Progress update | < 1ms | In-memory, async |

---

## 10. Crash Recovery and Safety

### 10.1 Orphaned Run Detection

**Problem**: If process crashes hard (SIGKILL), runs remain in "running" status forever.

**Solution**: Detect and remediate on startup:

```go
func (m *sqliteManager) ValidateOnStartup() error {
    // Find runs stuck in "running" status for > 1 hour
    result, err := m.db.Exec(`
        UPDATE runs
        SET status = 'interrupted',
            error_message = 'Process terminated unexpectedly'
        WHERE status = 'running'
        AND started_at < datetime('now', '-1 hour')
    `)
    if err != nil {
        return err
    }

    affected, _ := result.RowsAffected()
    if affected > 0 {
        log.Warnf("Marked %d orphaned runs as interrupted", affected)
    }

    return nil
}
```

### 10.2 Graceful Shutdown

```go
func (e *Executor) Execute(recipe *Recipe, opts Options) (*Result, error) {
    // Register signal handler
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigChan
        // Graceful shutdown
        log.Info("Interrupt received, creating checkpoint...")

        // Flush any pending writes
        e.state.Flush()

        // Create checkpoint
        e.createCheckpoint(ctx, CheckpointOptions{
            Type: CheckpointTypeInterrupt,
            Description: "Interrupted by user",
        })

        e.state.UpdateRunStatus(ctx.RunID, StatusInterrupted)
        os.Exit(130)
    }()

    // ... execution ...
}
```

### 10.3 Cross-Platform File Locking

**Problem**: `flock()` doesn't work on Windows or NFS (common in CI).

**Solution**: Use cross-platform locking library:

```go
import "github.com/gofrs/flock"

type StateLock struct {
    flock *flock.Flock
}

func NewStateLock(path string) *StateLock {
    return &StateLock{
        flock: flock.New(path),
    }
}

func (l *StateLock) Acquire() error {
    locked, err := l.flock.TryLock()
    if err != nil {
        return fmt.Errorf("failed to acquire lock: %w", err)
    }
    if !locked {
        return ErrLockHeld
    }
    return nil
}

func (l *StateLock) Release() error {
    return l.flock.Unlock()
}
```

### 10.4 Atomic File Updates

**Problem**: If process crashes mid-write, files can be corrupted with no recovery path.

**Solution**: Two-phase commit pattern for file modifications:

```go
func (e *Executor) modifyFileAtomically(ctx *ExecutionContext, path string, transform func([]byte) ([]byte, error)) error {
    // 1. Read original content
    original, err := os.ReadFile(path)
    if err != nil {
        return err
    }

    // 2. Apply transformation
    modified, err := transform(original)
    if err != nil {
        return err
    }

    // 3. Write to temp file
    tempPath := path + ".bazelbump-tmp"
    if err := os.WriteFile(tempPath, modified, 0644); err != nil {
        return err
    }

    // 4. Mark file as "staged"
    if err := ctx.StateManager.SetFileState(ctx.RunID, path, FileStatusStaged, ""); err != nil {
        os.Remove(tempPath)
        return err
    }

    // 5. Atomic rename (on POSIX, this is atomic within same filesystem)
    if err := os.Rename(tempPath, path); err != nil {
        os.Remove(tempPath)
        return err
    }

    // 6. Mark as completed
    afterHash, _ := computeHash(path)
    return ctx.StateManager.SetFileState(ctx.RunID, path, FileStatusCompleted, afterHash)
}

// Recovery: clean up any staged files on resume
func (e *Executor) cleanupStagedFiles(ctx *ExecutionContext) error {
    staged, err := ctx.StateManager.GetFilesByStatus(ctx.RunID, FileStatusStaged)
    if err != nil {
        return err
    }

    for _, path := range staged {
        tempPath := path + ".bazelbump-tmp"
        if _, err := os.Stat(tempPath); err == nil {
            // Temp file exists, complete the rename
            os.Rename(tempPath, path)
        }
        ctx.StateManager.SetFileState(ctx.RunID, path, FileStatusCompleted, "")
    }

    return nil
}
```

---

## 11. Concurrency and Locking

### 11.1 SQLite Concurrency

**Important**: SQLite already handles concurrency well. We rely on SQLite's locking, not Go mutexes:

```go
// Configure SQLite for safe concurrent access
db.SetMaxOpenConns(1)                    // Single writer (SQLite limitation)
db.Exec("PRAGMA journal_mode=WAL")       // Allow concurrent reads
db.Exec("PRAGMA busy_timeout=5000")      // Wait up to 5s for locks
```

### 11.2 Process-Level Locking

```go
// Prevent multiple bazelbump instances on same project
func (m *sqliteManager) AcquireLock() error {
    lockPath := filepath.Join(m.path, "..", "locks", "state.lock")
    m.lock = NewStateLock(lockPath)

    if err := m.lock.Acquire(); err != nil {
        if errors.Is(err, ErrLockHeld) {
            return fmt.Errorf("another bazelbump instance is running on this project")
        }
        return err
    }

    return nil
}
```

---

## 12. Examples and Workflows

### 12.1 Basic Migration Workflow

```bash
# 1. Start migration (auto-checkpoints enabled)
$ bazelbump recipe run autoload-migration --dry-run
==> Scanning 1,696 files...
==> Would modify 893 files
==> Run without --dry-run to apply

# 2. Apply migration
$ bazelbump recipe run autoload-migration
==> Running recipe: autoload-migration
    Step 1/3: Detect native rules [████████████████] 893/893
    Step 2/3: Add module dependencies [████████████████] 3/3
    Step 3/3: Add load statements [████████░░░░░░░░] 423/893
^C
==> Checkpoint saved. Resume with: bazelbump recipe run autoload-migration --resume

# 3. Check status
$ bazelbump status
Last Run: autoload-migration
  Status: interrupted
  Progress: 423/893 files (47%)

# 4. Resume
$ bazelbump recipe run autoload-migration --resume
==> Resuming from checkpoint (423/893)
    Step 3/3: Add load statements [████████████████] 893/893
==> Completed! 893 files modified.
```

### 12.2 Debugging with Export

```bash
# Export checkpoint state for debugging
$ bazelbump checkpoint export cp-abc123 --format=json > checkpoint.json

# View checkpoint contents
$ jq '.execution_state.scan_results | length' checkpoint.json
1823

# Export run state
$ bazelbump status --export run-state.json
```

---

## 13. Implementation Checklist

### Phase 1: Core State Management
- [x] SQLite schema with durability settings
- [x] State manager interface
- [x] SQLite implementation with `synchronous=FULL`
- [x] No-op implementation for `nostate` build
- [ ] Cross-platform file locking (gofrs/flock)
- [ ] Orphaned run detection on startup
- [ ] Batched writes implementation

### Phase 2: Checkpointing
- [ ] Checkpoint create/restore
- [ ] Checkpoint size limits and cleanup
- [ ] Recipe hash for cache invalidation
- [ ] Atomic file updates

### Phase 3: Progress and CLI
- [ ] Progress reporter implementations
- [ ] Enhanced CLI commands
- [ ] JSON export for debugging

### Phase 4: Integration
- [ ] Executor integration
- [ ] Resume semantics
- [ ] Incremental execution

### Phase 5: Testing
- [ ] Unit tests for all components
- [ ] Integration tests on Envoy-scale project
- [ ] Performance benchmarks

---

## Appendix A: Design Rationale and Benchmarks

### A.1 Industry Solutions Comparison

| Feature | bazelbump | Temporal | DBOS | Prefect | Airflow | Bazel |
|---------|-----------|----------|------|---------|---------|-------|
| Automatic Checkpointing | Interval-based | Every activity | Every step | Result caching | Task-level | Action cache |
| Exactly-Once | No (at-least-once) | Yes (via replay) | Yes (via txn) | Yes (via cache) | No | Yes (cache key) |
| Distributed State | No | Yes | Yes | Yes (optional) | Yes | Yes (remote cache) |
| Event History | Event log table | Full | Full | Partial | Full | No |
| Resume Granularity | File-level | Activity-level | Step-level | Task-level | Task-level | Action-level |
| Storage Backend | SQLite | Cassandra/SQL | Postgres | Postgres/S3 | SQL | Custom binary |
| Complexity | Low | High | Medium | Medium | High | High |

### A.2 Storage Approach Comparison

| Approach | Atomicity | Query Flexibility | Debugging | Portability |
|----------|-----------|-------------------|-----------|-------------|
| SQLite-only | ACID transactions | Full SQL | Requires tools | Better |
| JSON files | File-level only | Load + filter | Human-readable | FS edge cases |
| Hybrid | Mixed | Mixed | Best of both | Complex |

**Decision**: SQLite-only with JSON export for debugging.

### A.3 Temporal.io Insights

Temporal's "durable execution" model captures every state transition automatically, eliminating manual checkpoint placement decisions. Key insight: add an event log for debugging and audit:

```sql
CREATE TABLE event_log (
    id INTEGER PRIMARY KEY,
    run_id TEXT NOT NULL,
    timestamp TIMESTAMP,
    event_type TEXT NOT NULL,
    event_data JSON
);
```

**Reference**: [Temporal Workflow Documentation](https://docs.temporal.io/workflows)

### A.4 DBOS Insights

DBOS achieves exactly-once semantics by executing the step AND its checkpoint in a single transaction. For file operations (which have side effects), we use upsert patterns for idempotent state updates:

```sql
INSERT INTO file_states (run_id, file_path, status, ...)
VALUES (?, ?, ?, ...)
ON CONFLICT(run_id, file_path) DO UPDATE SET
    status = excluded.status,
    ...
WHERE status != 'completed'
```

**Reference**: [DBOS Architecture](https://docs.dbos.dev/architecture)

### A.5 Prefect Insights

Prefect distinguishes between "Running" and "Retrying" states, helping users understand recovery. We add `retry_count` and the "retrying" state:

```sql
ALTER TABLE runs ADD COLUMN retry_count INTEGER DEFAULT 0;
```

**Reference**: [Prefect States Documentation](https://docs.prefect.io/v3/concepts/states)

### A.6 SQLite WAL Mode Analysis

**Critical**: With `synchronous=NORMAL` (the default), transactions can be lost on power failure. WAL mode is not crash-safe without `synchronous=FULL`:

| Setting | Durability | Performance |
|---------|------------|-------------|
| `synchronous=OFF` | May lose data | Fastest |
| `synchronous=NORMAL` | May lose on power fail | Fast |
| `synchronous=FULL` | Durable | ~2x slower |

For a checkpointing system, durability is critical. We use `synchronous=FULL`.

**Reference**: [SQLite WAL Mode](https://sqlite.org/wal.html)

### A.7 Bazel Cache Key Insights

Bazel's action cache includes both input hashes AND action definition. This means changing the "recipe" invalidates the cache even if files haven't changed.

We add `recipe_hash` to runs and check it during incremental execution.

**Reference**: [Bazel Persistent Action Cache](https://sluongng.hashnode.dev/bazel-caching-explained-pt-4-persistent-action-cache)

### A.8 Git Content-Addressed Storage (Future)

For non-git repos, content-addressed storage enables:
- Deduplication across runs
- Efficient undo without git
- Proof of what was changed

This is a v2 feature.

**Reference**: [Git Objects](https://git-scm.com/book/en/v2/Git-Internals-Git-Objects)

---

## References

- [SQLite WAL Mode](https://www.sqlite.org/wal.html)
- [Temporal Workflow Documentation](https://docs.temporal.io/workflows)
- [DBOS Architecture](https://docs.dbos.dev/architecture)
- [Prefect States Documentation](https://docs.prefect.io/v3/concepts/states)
- [Bazel Persistent Action Cache](https://sluongng.hashnode.dev/bazel-caching-explained-pt-4-persistent-action-cache)
- [gofrs/flock - Cross-platform file locking](https://github.com/gofrs/flock)
- [xxHash](https://cyan4973.github.io/xxHash/)
