# Workflow and Checkpointing System Specification

**Status:** Draft v1  
**Author:** bazelbump contributors  
**Created:** 2026-02-03  
**Updated:** 2026-02-03

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

### 2.3 Optional Feature (Build Tags)

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

**Justfile Integration:**

```makefile
# justfile
build:
    go build ./cmd/bazelbump

build-minimal:
    go build -tags nostate ./cmd/bazelbump

# Show binary sizes
size:
    @echo "Full build:"
    @go build -o /tmp/bazelbump-full ./cmd/bazelbump && ls -lh /tmp/bazelbump-full
    @echo "Minimal build:"
    @go build -tags nostate -o /tmp/bazelbump-minimal ./cmd/bazelbump && ls -lh /tmp/bazelbump-minimal
```

### 2.3 Database Schema

```sql
-- ============================================================
-- RUNS: Top-level recipe executions
-- ============================================================
CREATE TABLE runs (
    id              TEXT PRIMARY KEY,           -- UUID
    recipe_name     TEXT NOT NULL,
    recipe_version  TEXT,
    project_path    TEXT NOT NULL,
    
    -- Timing
    started_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at    TIMESTAMP,
    
    -- Status
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK(status IN ('pending', 'running', 'completed', 
                                     'failed', 'interrupted', 'cancelled')),
    
    -- Progress
    total_files     INTEGER DEFAULT 0,
    processed_files INTEGER DEFAULT 0,
    current_step    TEXT,
    current_file    TEXT,
    
    -- Context
    git_commit      TEXT,                       -- HEAD at start
    git_branch      TEXT,
    bazel_version   TEXT,
    dry_run         BOOLEAN DEFAULT FALSE,
    
    -- Configuration
    config_json     TEXT,                       -- Recipe options
    
    -- Result
    error_message   TEXT,
    changes_count   INTEGER DEFAULT 0
);

CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_started ON runs(started_at DESC);

-- ============================================================
-- CHECKPOINTS: Savepoints within runs
-- ============================================================
CREATE TABLE checkpoints (
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
    
    -- State snapshot
    state_json      TEXT,                       -- Serialized execution state
    
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
                    CHECK(status IN ('pending', 'skipped', 'processing', 
                                     'completed', 'failed')),
    
    -- Change detection
    content_hash    TEXT,                       -- xxHash of file content
    mtime           INTEGER,                    -- File modification time
    
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
-- AUDIT_LOG: Detailed change history
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
                                     'step_started', 'step_completed')),
    
    -- Details
    file_path       TEXT,
    step_id         TEXT,
    description     TEXT,
    diff_summary    TEXT,                       -- "+3 -1" style summary
    
    -- Before/after for debugging
    old_value       TEXT,
    new_value       TEXT
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

INSERT INTO schema_version (version) VALUES (1);
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
        ┌──────────┐   ┌─────────────┐   ┌────────┐
        │  failed  │   │ interrupted │   │ paused │
        └──────────┘   └─────────────┘   └────────┘
                              │               │
                              │    User: --resume
                              │               │
                              ▼               ▼
                         ┌─────────────────────┐
                         │    resumed/running  │
                         └─────────────────────┘
```

### 3.2 File States

```
┌─────────┐   scan    ┌────────────┐   process   ┌───────────┐
│ pending │─────────▶│ processing │────────────▶│ completed │
└─────────┘          └────────────┘             └───────────┘
     │                     │
     │ unchanged           │ error
     ▼                     ▼
┌─────────┐          ┌────────┐
│ skipped │          │ failed │
└─────────┘          └────────┘
```

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
    "file_index": 423,
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

### 4.3 Checkpoint Operations

```go
// CheckpointManager handles checkpoint lifecycle
type CheckpointManager interface {
    // Create a new checkpoint
    Create(ctx context.Context, opts CheckpointOptions) (*Checkpoint, error)
    
    // Restore execution state from checkpoint
    Restore(ctx context.Context, checkpointID string) (*ExecutionState, error)
    
    // List checkpoints for a run
    List(ctx context.Context, runID string) ([]*Checkpoint, error)
    
    // Get latest checkpoint
    Latest(ctx context.Context, runID string) (*Checkpoint, error)
    
    // Delete old checkpoints
    Cleanup(ctx context.Context, keepLast int) error
}

type CheckpointOptions struct {
    Type        CheckpointType  // auto, manual, step_boundary
    Description string          // Optional user description
    StepID      string          // Current step
    FileIndex   int             // Current file position
    State       *ExecutionState // Serializable state
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

### 5.4 JSON Progress Stream

```jsonl
{"event":"run_start","run_id":"run-123","recipe":"autoload-migration","timestamp":"..."}
{"event":"step_start","step_id":"detect-native-rules","step_index":0}
{"event":"file_start","path":"source/common/BUILD"}
{"event":"file_complete","path":"source/common/BUILD","changed":true,"lines_added":1}
{"event":"progress","step_id":"detect-native-rules","processed":100,"total":893}
{"event":"checkpoint","checkpoint_id":"cp-456","type":"auto","file_index":100}
{"event":"step_complete","step_id":"detect-native-rules","duration_ms":4200}
{"event":"run_complete","run_id":"run-123","success":true,"files_modified":893}
```

---

## 6. Incremental Execution

### 6.1 Change Detection

```go
// ChangeDetector determines if a file needs reprocessing
type ChangeDetector interface {
    // HasChanged returns true if file needs reprocessing
    HasChanged(path string, lastRun *Run) (bool, error)
    
    // ComputeHash computes content hash for a file
    ComputeHash(path string) (string, error)
    
    // RecordState stores file state after processing
    RecordState(path string, hash string) error
}

// Implementation using xxHash (fast, non-cryptographic)
type XXHashDetector struct {
    stateManager StateManager
}

func (d *XXHashDetector) HasChanged(path string, lastRun *Run) (bool, error) {
    // Get stored hash from last run
    lastState, err := d.stateManager.GetFileState(lastRun.ID, path)
    if err != nil || lastState == nil {
        return true, nil // Never processed, needs processing
    }
    
    // Compute current hash
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

### 6.3 Skip Reasons

| Reason | Description |
|--------|-------------|
| `unchanged` | File content hash matches previous run |
| `already_processed` | File completed in current run (resume) |
| `excluded` | File matches exclude pattern |
| `no_match` | File doesn't match scan patterns |

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

Examples:
  bazelbump status                    # Current/last run summary
  bazelbump status --progress         # Live progress display
  bazelbump status --run run-abc123   # Specific run details
```

**Output:**
```
Last Run: autoload-migration
  Status:    interrupted
  Started:   2026-02-03 14:25:00 (38 minutes ago)
  Progress:  423/893 files (47%)
  Step:      detect-native-rules (1/3)
  
  Checkpoint: step-1-file-400 (2 minutes ago)
  
  To resume: bazelbump recipe run autoload-migration --resume
  To restart: bazelbump recipe run autoload-migration --restart
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

Examples:
  bazelbump history                           # Recent runs
  bazelbump history --since 7d                # Last 7 days
  bazelbump history --recipe autoload-migration
  bazelbump history --run run-abc123          # Full details
```

**Output:**
```
Recent Runs:

ID              Recipe              Status       Files    Duration   When
─────────────────────────────────────────────────────────────────────────────
run-abc123      autoload-migration  interrupted  423/893  4m 12s     38m ago
run-xyz789      flag-cleanup        completed    5/5      1.2s       2h ago
run-def456      autoload-migration  completed    893/893  8m 34s     1d ago

Use 'bazelbump history --run <id>' for details.
```

### 7.4 Checkpoint Commands

```bash
bazelbump checkpoint list [flags]
bazelbump checkpoint create [flags]
bazelbump checkpoint restore <id> [flags]
bazelbump checkpoint delete <id> [flags]

Flags:
  --run <id>      Target run (default: current/last)
  --json          JSON output
  --force         Skip confirmation for destructive ops

Examples:
  bazelbump checkpoint list
  bazelbump checkpoint create --description "Before risky step"
  bazelbump checkpoint restore cp-abc123
```

**Output:**
```
Checkpoints for run-abc123:

ID              Type    Step                   File      Created
────────────────────────────────────────────────────────────────────
cp-001          auto    detect-native-rules    100/893   14:27:00
cp-002          auto    detect-native-rules    200/893   14:28:30
cp-003          auto    detect-native-rules    300/893   14:30:00
cp-004          auto    detect-native-rules    400/893   14:31:30

Use 'bazelbump checkpoint restore <id>' to restore.
```

---

## 8. Integration with Existing Systems

### 8.1 Integration with pkg/track

The checkpoint system builds on the existing `pkg/track/` system:

```go
// Extended Tracker interface
type Tracker interface {
    // Existing methods (file tracking)
    TrackFile(path string) error
    GetLastRun() (*TrackedRun, error)
    Undo() error
    
    // New methods (state management)
    StateManager() StateManager
    CheckpointManager() CheckpointManager
    ProgressReporter() ProgressReporter
}

// TrackerWithState wraps existing trackers with state management
type TrackerWithState struct {
    Tracker              // Embedded existing tracker (git or file)
    state   StateManager
    cp      CheckpointManager
}
```

### 8.2 Integration with Recipe Executor

```go
// Enhanced executor context
type Context struct {
    // Existing fields
    ProjectPath string
    DryRun      bool
    Verbose     bool
    Outputs     map[string]any
    Changes     []*FileChange
    Tracker     FileTracker
    
    // New fields for checkpointing
    RunID       string
    State       StateManager
    Checkpoints CheckpointManager
    Progress    ProgressReporter
    
    // Resume state (populated when resuming)
    ResumeFrom  *Checkpoint
    SkipFiles   map[string]bool  // Already processed files
}

// Enhanced executor
func (e *Executor) Execute(recipe *Recipe, opts Options) (*Result, error) {
    // Initialize or resume run
    var run *Run
    if opts.Resume {
        run, checkpoint = e.resumeRun(recipe)
    } else {
        run = e.startNewRun(recipe, opts)
    }
    
    // Set up progress reporting
    ctx.Progress.OnRunStart(run)
    defer ctx.Progress.OnRunComplete(run, result)
    
    // Execute with checkpointing
    for i, step := range sortedSteps {
        if checkpoint != nil && i < checkpoint.StepIndex {
            continue // Skip completed steps
        }
        
        ctx.Progress.OnStepStart(step)
        result := e.executeStepWithCheckpoints(ctx, step, opts)
        ctx.Progress.OnStepComplete(step, result)
    }
    
    return result, nil
}
```

---

## 9. Performance Considerations

### 9.1 Benchmarks

Target performance for Envoy-scale projects (1,696 files, 893 to modify):

| Operation | Target | Notes |
|-----------|--------|-------|
| Checkpoint create | < 50ms | Serialize + write to SQLite |
| Checkpoint restore | < 100ms | Read + deserialize |
| File state lookup | < 1ms | Indexed query |
| Hash computation | < 10ms/file | xxHash is fast |
| Progress update | < 1ms | In-memory, batch DB writes |

### 9.2 Optimization Strategies

| Strategy | Implementation |
|----------|---------------|
| Batch DB writes | Buffer file states, flush every 100 files |
| Hash caching | LRU cache for recently computed hashes |
| Lazy serialization | Only serialize state at checkpoint time |
| Async progress | Progress updates on separate goroutine |

### 9.3 Memory Management

```go
// For very large projects, stream file processing
type StreamingExecutor struct {
    batchSize int // Process files in batches
    
    // Don't hold all file states in memory
    // Query from DB as needed
}
```

---

## 10. Error Handling and Recovery

### 10.1 Error Scenarios

| Scenario | Behavior |
|----------|----------|
| Process killed (SIGKILL) | Resume from last checkpoint |
| Graceful interrupt (SIGINT) | Create checkpoint, then exit |
| Disk full | Fail with clear error, no corruption |
| Corrupt state.db | Backup + recreate, warn user |
| File disappeared mid-run | Log warning, continue with remaining |

### 10.2 Crash Recovery

```go
func (e *Executor) Execute(recipe *Recipe, opts Options) (*Result, error) {
    // Register signal handler
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        // Graceful shutdown
        log.Info("Interrupt received, creating checkpoint...")
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

### 10.3 State Validation

```go
// Validate state on startup
func (sm *StateManager) Validate() error {
    // Check schema version
    version, err := sm.getSchemaVersion()
    if err != nil || version != CurrentSchemaVersion {
        return sm.migrate(version, CurrentSchemaVersion)
    }
    
    // Check for incomplete runs
    incompleteRuns, err := sm.findIncompleteRuns()
    if err != nil {
        return err
    }
    
    for _, run := range incompleteRuns {
        log.Warnf("Found incomplete run: %s (status: %s)", run.ID, run.Status)
    }
    
    return nil
}
```

---

## 11. Concurrency and Locking

### 11.1 File Locking

```go
// Prevent multiple bazelbump instances
type StateLock struct {
    lockFile string
    file     *os.File
}

func (l *StateLock) Acquire() error {
    l.file, err := os.OpenFile(l.lockFile, os.O_CREATE|os.O_RDWR, 0600)
    if err != nil {
        return err
    }
    
    err = syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
    if err != nil {
        return fmt.Errorf("another bazelbump instance is running")
    }
    
    return nil
}

func (l *StateLock) Release() error {
    syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
    return l.file.Close()
}
```

### 11.2 SQLite Configuration

```go
// Configure SQLite for concurrent access (read-only queries)
db.SetMaxOpenConns(1)        // Single writer
db.Exec("PRAGMA journal_mode=WAL")  // Allow concurrent reads
db.Exec("PRAGMA busy_timeout=5000") // Wait up to 5s for locks
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

### 12.2 Incremental Re-run

```bash
# First run
$ bazelbump recipe run autoload-migration
==> Completed! 893 files modified.

# ... developer modifies some files ...

# Re-run (only processes changed files)
$ bazelbump recipe run autoload-migration
==> Incremental run (baseline: run-abc123)
==> Scanning for changes...
==> 12 files changed since last run
==> Processing changed files...
==> Completed! 8 files modified.
```

### 12.3 Viewing History

```bash
$ bazelbump history --since 7d

Recent Runs:
ID              Recipe              Status       Files    Duration   When
─────────────────────────────────────────────────────────────────────────────
run-abc123      autoload-migration  completed    893/893  8m 34s     2h ago
run-xyz789      flag-cleanup        completed    5/5      1.2s       1d ago
run-def456      autoload-migration  failed       234/893  2m 12s     3d ago

$ bazelbump history --run run-def456

Run: run-def456
  Recipe:    autoload-migration v2.0.0
  Status:    failed
  Started:   2026-01-31 10:15:00
  Duration:  2m 12s
  Progress:  234/893 files
  Error:     "source/BUILD:15: syntax error"
  
  Checkpoints:
    - cp-001: auto at file 100 (10:16:00)
    - cp-002: auto at file 200 (10:17:00)
  
  Files modified: 187
  Files failed: 1
```

---

## 13. Migration Plan

| Phase | Scope | Timeline |
|-------|-------|----------|
| 1 | Core state management (SQLite, schema) | Week 1-2 |
| 2 | Checkpoint create/restore | Week 3-4 |
| 3 | Progress reporting | Week 5 |
| 4 | CLI commands (status, history, checkpoint) | Week 6 |
| 5 | Incremental execution | Week 7-8 |
| 6 | Testing on Envoy-scale projects | Week 9-10 |

---

## References

- [SQLite WAL Mode](https://www.sqlite.org/wal.html)
- [Terraform State Management](https://developer.hashicorp.com/terraform/language/state)
- [rsync Algorithm](https://rsync.samba.org/tech_report/)
- [xxHash](https://cyan4973.github.io/xxHash/)
