# Executor-State Integration Specification

**Status:** Draft v2 (Updated with review findings)
**Updated:** 2026-02-03
**Related:** [Workflow Checkpoint Spec](workflow-checkpoint-spec.md) | [Checkpoint Spec Review](checkpoint-spec-review.md)

## Overview

This document specifies how the recipe executor integrates with the state management system to provide:
- Run tracking and persistence
- Automatic checkpointing during execution
- Resume capability for interrupted runs
- Progress reporting
- Audit logging
- **Atomic file updates** (crash-safe)
- **Batched database writes** (performance)
- **Cross-platform locking** (concurrent access prevention)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Recipe Executor                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │   Starlark   │  │    Step      │  │      File            │  │
│  │   Engine     │──│   Executor   │──│    Processor         │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
│         │                 │                    │                │
│         ▼                 ▼                    ▼                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                  Execution Context                       │   │
│  │  - State Manager reference                               │   │
│  │  - Current run ID                                        │   │
│  │  - Progress reporter                                     │   │
│  │  - Checkpoint config                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      State Manager                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │     Run      │  │  Checkpoint  │  │     File State       │  │
│  │   Tracking   │  │   Manager    │  │      Tracker         │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
│                              │                                  │
│                              ▼                                  │
│                    ┌──────────────┐                            │
│                    │   SQLite DB  │                            │
│                    │  state.db    │                            │
│                    └──────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
```

## Execution Context Extensions

### Current Context Structure

```go
// recipe/executor.go - Current
type ExecutionContext struct {
    Path      string
    DryRun    bool
    Verbose   bool
    Outputs   map[string]interface{}
    StepIndex int
}
```

### Extended Context Structure

```go
// recipe/executor.go - Extended
type ExecutionContext struct {
    // Existing fields
    Path      string
    DryRun    bool
    Verbose   bool
    Outputs   map[string]interface{}
    StepIndex int

    // State management fields
    StateManager     state.Manager
    RunID            string
    ProgressReporter state.ProgressReporter
    CheckpointConfig CheckpointConfig

    // Resume support
    ResumeFromRun    string           // Run ID to resume from
    ResumeCheckpoint string           // Specific checkpoint ID (optional)
    SkipCompleted    bool             // Skip files marked completed

    // Internal tracking
    filesProcessed   int
    lastCheckpoint   time.Time
}

type CheckpointConfig struct {
    Enabled           bool
    AutoInterval      int           // Files between auto checkpoints (default: 100)
    StepBoundary      bool          // Checkpoint at step boundaries (default: true)
    MinTimeBetween    time.Duration // Minimum time between checkpoints (default: 30s)
    MaxCheckpoints    int           // Max checkpoints to keep per run (default: 10)
}

// RecipeInfo for cache invalidation (inspired by Bazel)
type RecipeInfo struct {
    Name    string
    Version string
    Hash    string // Hash of recipe content - invalidates cache when recipe changes
}
```

## Execution Lifecycle

### 1. Run Initialization

```go
func (e *Executor) initializeRun(ctx *ExecutionContext, recipe *Recipe) error {
    // Skip if state management unavailable or disabled
    if ctx.StateManager == nil || !ctx.StateManager.Available() {
        return nil
    }

    // Check for resume
    if ctx.ResumeFromRun != "" {
        return e.resumeRun(ctx)
    }

    // Create new run
    run, err := ctx.StateManager.CreateRun(
        recipe.Name,
        recipe.Version,
        ctx.Path,
        serializeConfig(ctx),
    )
    if err != nil {
        return fmt.Errorf("failed to create run: %w", err)
    }

    ctx.RunID = run.ID

    // Log run start
    ctx.StateManager.LogAction(run.ID, "", "run_start", map[string]interface{}{
        "recipe":  recipe.Name,
        "version": recipe.Version,
        "path":    ctx.Path,
        "dry_run": ctx.DryRun,
    })

    // Report run start
    if ctx.ProgressReporter != nil {
        ctx.ProgressReporter.RunStart(run.ID, recipe.Name)
    }

    return nil
}
```

### 2. Resume Logic

```go
func (e *Executor) resumeRun(ctx *ExecutionContext) error {
    run, err := ctx.StateManager.GetRun(ctx.ResumeFromRun)
    if err != nil {
        return fmt.Errorf("failed to get run: %w", err)
    }

    if run.Status == state.RunStatusCompleted {
        return fmt.Errorf("run %s already completed", run.ID)
    }

    // Get checkpoint to resume from
    var checkpoint *state.Checkpoint
    if ctx.ResumeCheckpoint != "" {
        checkpoint, err = ctx.StateManager.GetCheckpoint(ctx.ResumeCheckpoint)
    } else {
        checkpoint, err = ctx.StateManager.GetLatestCheckpoint(run.ID)
    }
    if err != nil {
        return fmt.Errorf("failed to get checkpoint: %w", err)
    }

    // Restore execution state
    ctx.RunID = run.ID
    ctx.StepIndex = checkpoint.StepIndex
    ctx.Outputs = deserializeOutputs(checkpoint.Metadata)
    ctx.SkipCompleted = true

    // Update run status
    ctx.StateManager.UpdateRunStatus(run.ID, state.RunStatusRunning)

    // Log resume
    ctx.StateManager.LogAction(run.ID, "", "run_resume", map[string]interface{}{
        "checkpoint_id":   checkpoint.ID,
        "step_index":      checkpoint.StepIndex,
        "files_processed": checkpoint.FilesProcessed,
    })

    return nil
}
```

### 3. Step Execution Hooks

```go
func (e *Executor) executeStep(ctx *ExecutionContext, step Step, index int) error {
    // Report step start
    if ctx.ProgressReporter != nil {
        ctx.ProgressReporter.StepStart(step.Name, index+1, len(e.recipe.Steps))
    }

    // Log step start
    if ctx.StateManager != nil && ctx.StateManager.Available() {
        ctx.StateManager.LogAction(ctx.RunID, "", "step_start", map[string]interface{}{
            "step_name":  step.Name,
            "step_index": index,
            "step_type":  step.Type,
        })
    }

    // Execute step
    result, err := e.runStep(ctx, step)

    // Handle result
    if err != nil {
        e.handleStepError(ctx, step, index, err)
        return err
    }

    // Report step completion
    if ctx.ProgressReporter != nil {
        ctx.ProgressReporter.StepComplete(step.Name, true, "")
    }

    // Create step boundary checkpoint
    if ctx.CheckpointConfig.StepBoundary {
        e.createCheckpoint(ctx, state.CheckpointTypeStepBoundary)
    }

    return nil
}
```

### 4. File Processing Hooks

```go
func (e *Executor) processFile(ctx *ExecutionContext, file string, step Step) error {
    // Check if file should be skipped (resume mode)
    if ctx.SkipCompleted && ctx.StateManager != nil {
        fileState, _ := ctx.StateManager.GetFileState(ctx.RunID, file)
        if fileState != nil && fileState.Status == state.FileStatusCompleted {
            if ctx.ProgressReporter != nil {
                ctx.ProgressReporter.FileSkipped(file, "already processed")
            }
            return nil
        }
    }

    // Compute file hash before processing
    beforeHash := computeFileHash(file)

    // Record file as in-progress
    if ctx.StateManager != nil && ctx.StateManager.Available() {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusInProgress, beforeHash)
    }

    // Report file start
    if ctx.ProgressReporter != nil {
        ctx.ProgressReporter.FileStart(file)
    }

    // Process the file
    changed, err := e.applyTransforms(ctx, file, step)

    if err != nil {
        // Record failure
        if ctx.StateManager != nil {
            ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusFailed, beforeHash)
            ctx.StateManager.LogAction(ctx.RunID, file, "file_error", map[string]interface{}{
                "error": err.Error(),
            })
        }
        return err
    }

    // Compute after hash and record completion
    afterHash := ""
    if changed {
        afterHash = computeFileHash(file)
    }

    if ctx.StateManager != nil && ctx.StateManager.Available() {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusCompleted, afterHash)

        if changed {
            ctx.StateManager.LogAction(ctx.RunID, file, "file_modified", map[string]interface{}{
                "before_hash": beforeHash,
                "after_hash":  afterHash,
            })
        }
    }

    // Report file completion
    if ctx.ProgressReporter != nil {
        if changed {
            ctx.ProgressReporter.FileChanged(file)
        } else {
            ctx.ProgressReporter.FileUnchanged(file)
        }
    }

    // Update progress and check for auto checkpoint
    ctx.filesProcessed++
    e.maybeAutoCheckpoint(ctx)

    return nil
}
```

### 5. Auto Checkpoint Logic

```go
func (e *Executor) maybeAutoCheckpoint(ctx *ExecutionContext) {
    if !ctx.CheckpointConfig.Enabled {
        return
    }

    if ctx.StateManager == nil || !ctx.StateManager.Available() {
        return
    }

    // Check interval
    if ctx.filesProcessed%ctx.CheckpointConfig.AutoInterval != 0 {
        return
    }

    // Check minimum time between checkpoints
    if time.Since(ctx.lastCheckpoint) < ctx.CheckpointConfig.MinTimeBetween {
        return
    }

    e.createCheckpoint(ctx, state.CheckpointTypeAuto)
}

func (e *Executor) createCheckpoint(ctx *ExecutionContext, checkpointType state.CheckpointType) error {
    if ctx.StateManager == nil || !ctx.StateManager.Available() {
        return nil
    }

    checkpoint, err := ctx.StateManager.CreateCheckpoint(
        ctx.RunID,
        checkpointType,
        ctx.StepIndex,
        ctx.filesProcessed,
        serializeOutputs(ctx.Outputs),
    )
    if err != nil {
        // Log but don't fail execution
        if ctx.Verbose {
            fmt.Fprintf(os.Stderr, "Warning: failed to create checkpoint: %v\n", err)
        }
        return err
    }

    ctx.lastCheckpoint = time.Now()

    // Log checkpoint creation
    ctx.StateManager.LogAction(ctx.RunID, "", "checkpoint_created", map[string]interface{}{
        "checkpoint_id":   checkpoint.ID,
        "checkpoint_type": string(checkpointType),
        "step_index":      ctx.StepIndex,
        "files_processed": ctx.filesProcessed,
    })

    return nil
}
```

### 6. Run Completion

```go
func (e *Executor) finalizeRun(ctx *ExecutionContext, err error) {
    if ctx.StateManager == nil || !ctx.StateManager.Available() {
        return
    }

    var finalStatus state.RunStatus
    var errorMsg string

    if err != nil {
        if errors.Is(err, context.Canceled) || errors.Is(err, ErrInterrupted) {
            finalStatus = state.RunStatusInterrupted
        } else {
            finalStatus = state.RunStatusFailed
            errorMsg = err.Error()
        }
    } else {
        finalStatus = state.RunStatusCompleted
    }

    // Update run status
    ctx.StateManager.UpdateRunStatus(ctx.RunID, finalStatus)

    // Update final progress
    summary := e.computeSummary(ctx)
    ctx.StateManager.UpdateRunProgress(
        ctx.RunID,
        summary.TotalFiles,
        summary.ProcessedFiles,
        summary.ChangedFiles,
    )

    // Log completion
    ctx.StateManager.LogAction(ctx.RunID, "", "run_complete", map[string]interface{}{
        "status":          string(finalStatus),
        "error":           errorMsg,
        "total_files":     summary.TotalFiles,
        "processed_files": summary.ProcessedFiles,
        "changed_files":   summary.ChangedFiles,
        "duration_ms":     summary.DurationMs,
    })

    // Report completion
    if ctx.ProgressReporter != nil {
        ctx.ProgressReporter.RunComplete(ctx.RunID, err == nil, summary.DurationMs)
    }

    // Create final checkpoint for completed runs
    if finalStatus == state.RunStatusCompleted {
        e.createCheckpoint(ctx, state.CheckpointTypeStepBoundary)
    }
}
```

## Atomic File Updates (Critical for Crash Safety)

**Problem**: If process crashes mid-write, files can be corrupted with no recovery path.

**Solution**: Two-phase commit pattern using temp file + atomic rename:

```go
func (e *Executor) processFileAtomically(ctx *ExecutionContext, file string, step Step) error {
    // 1. Read original content
    original, err := os.ReadFile(file)
    if err != nil {
        return err
    }
    beforeHash := computeHash(original)

    // 2. Set state to in_progress
    if ctx.StateManager != nil {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusInProgress, beforeHash)
    }

    // 3. Apply transformations (in memory)
    modified, changed, err := e.applyTransforms(original, step)
    if err != nil {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusFailed, beforeHash)
        return err
    }

    if !changed {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusCompleted, beforeHash)
        return nil
    }

    // 4. Write to temp file (same directory for atomic rename)
    tempPath := file + ".bazelbump-tmp"
    if err := os.WriteFile(tempPath, modified, 0644); err != nil {
        os.Remove(tempPath)
        return err
    }

    // 5. Mark as "staged" (temp file exists)
    if ctx.StateManager != nil {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusStaged, "")
    }

    // 6. Atomic rename (POSIX guarantees atomicity on same filesystem)
    if err := os.Rename(tempPath, file); err != nil {
        os.Remove(tempPath)
        return err
    }

    // 7. Mark as completed with new hash
    afterHash := computeHash(modified)
    if ctx.StateManager != nil {
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusCompleted, afterHash)
    }

    return nil
}

// Recovery on resume: complete any staged files
func (e *Executor) recoverStagedFiles(ctx *ExecutionContext) error {
    if ctx.StateManager == nil {
        return nil
    }

    staged, err := ctx.StateManager.GetFilesByStatus(ctx.RunID, state.FileStatusStaged)
    if err != nil {
        return err
    }

    for _, file := range staged {
        tempPath := file + ".bazelbump-tmp"
        if _, err := os.Stat(tempPath); err == nil {
            // Temp file exists - complete the rename
            if err := os.Rename(tempPath, file); err != nil {
                log.Warnf("Failed to recover staged file %s: %v", file, err)
                continue
            }
        }
        // Mark as completed
        ctx.StateManager.SetFileState(ctx.RunID, file, state.FileStatusCompleted, "")
    }

    return nil
}
```

## Batched Database Writes (Performance)

**Problem**: Individual writes per file cause 2,679+ writes for Envoy (893 files × 3 writes).

**Solution**: Buffer writes and flush periodically:

```go
// BatchedStateWriter wraps state manager with write batching
type BatchedStateWriter struct {
    manager        state.Manager
    pendingStates  []pendingFileState
    pendingEvents  []pendingEvent

    flushThreshold int           // Flush every N operations (default: 50)
    flushInterval  time.Duration // Flush every N seconds (default: 5s)
    lastFlush      time.Time

    mu sync.Mutex
}

type pendingFileState struct {
    runID       string
    filePath    string
    status      state.FileStatus
    contentHash string
}

func NewBatchedStateWriter(manager state.Manager) *BatchedStateWriter {
    return &BatchedStateWriter{
        manager:        manager,
        flushThreshold: 50,
        flushInterval:  5 * time.Second,
        lastFlush:      time.Now(),
    }
}

func (b *BatchedStateWriter) SetFileState(runID, path string, status state.FileStatus, hash string) error {
    b.mu.Lock()
    defer b.mu.Unlock()

    b.pendingStates = append(b.pendingStates, pendingFileState{
        runID:       runID,
        filePath:    path,
        status:      status,
        contentHash: hash,
    })

    // Check if we should flush
    if len(b.pendingStates) >= b.flushThreshold ||
       time.Since(b.lastFlush) >= b.flushInterval {
        return b.flushLocked()
    }

    return nil
}

func (b *BatchedStateWriter) Flush() error {
    b.mu.Lock()
    defer b.mu.Unlock()
    return b.flushLocked()
}

func (b *BatchedStateWriter) flushLocked() error {
    if len(b.pendingStates) == 0 {
        return nil
    }

    // Batch all writes in single transaction
    err := b.manager.BatchSetFileStates(b.pendingStates)
    if err != nil {
        return err
    }

    b.pendingStates = b.pendingStates[:0]
    b.lastFlush = time.Now()
    return nil
}
```

## Signal Handling for Graceful Shutdown

```go
func (e *Executor) Execute(ctx *ExecutionContext, recipe *Recipe) error {
    // Set up signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // Create cancellable context
    execCtx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle signals
    go func() {
        <-sigChan
        if ctx.ProgressReporter != nil {
            ctx.ProgressReporter.Warning("Interrupt received, saving state...")
        }

        // CRITICAL: Flush any pending batched writes first
        if ctx.BatchedWriter != nil {
            ctx.BatchedWriter.Flush()
        }

        // Create interrupt checkpoint
        e.createCheckpoint(ctx, state.CheckpointTypeAuto)

        // Mark run as interrupted
        if ctx.StateManager != nil {
            ctx.StateManager.UpdateRunStatus(ctx.RunID, state.RunStatusInterrupted)
        }

        cancel()
    }()

    // Execute with context
    return e.executeWithContext(execCtx, ctx, recipe)
}
```

## CLI Integration

### Executor Options

```go
// recipe/executor.go

type ExecutorOption func(*Executor)

func WithStateManager(sm state.Manager) ExecutorOption {
    return func(e *Executor) {
        e.stateManager = sm
    }
}

func WithProgressReporter(pr state.ProgressReporter) ExecutorOption {
    return func(e *Executor) {
        e.progressReporter = pr
    }
}

func WithCheckpointConfig(cfg CheckpointConfig) ExecutorOption {
    return func(e *Executor) {
        e.checkpointConfig = cfg
    }
}

func WithResume(runID string, checkpointID string) ExecutorOption {
    return func(e *Executor) {
        e.resumeRunID = runID
        e.resumeCheckpointID = checkpointID
    }
}

func NewExecutor(opts ...ExecutorOption) *Executor {
    e := &Executor{
        checkpointConfig: DefaultCheckpointConfig(),
    }
    for _, opt := range opts {
        opt(e)
    }
    return e
}

func DefaultCheckpointConfig() CheckpointConfig {
    return CheckpointConfig{
        Enabled:        true,
        AutoInterval:   100,
        StepBoundary:   true,
        MinTimeBetween: 30 * time.Second,
    }
}
```

### CLI Flags

```go
// cmd/bazelbump/recipe.go

func recipeRunCommand() {
    fs := flag.NewFlagSet("recipe run", flag.ExitOnError)

    // Existing flags
    path := fs.String("path", ".", "Path to project directory")
    dryRun := fs.Bool("dry-run", false, "Show changes without applying")
    verbose := fs.Bool("verbose", false, "Verbose output")

    // State management flags
    resume := fs.String("resume", "", "Resume from run ID")
    checkpoint := fs.String("checkpoint", "", "Resume from specific checkpoint")
    restart := fs.Bool("restart", false, "Restart failed/interrupted run")
    noCheckpoint := fs.Bool("no-checkpoint", false, "Disable checkpointing")
    checkpointInterval := fs.Int("checkpoint-interval", 100, "Files between auto checkpoints")

    // Progress output flags
    progress := fs.String("progress", "auto", "Progress output: auto, terminal, json, none")

    // Parse and execute...
}
```

### Command Examples

```bash
# Normal execution with checkpointing
bazelbump recipe run --path /project autoload-migration.star

# Resume interrupted run
bazelbump recipe run --resume run-abc123 autoload-migration.star

# Resume from specific checkpoint
bazelbump recipe run --resume run-abc123 --checkpoint chk-xyz789 autoload-migration.star

# Restart failed run from beginning
bazelbump recipe run --restart autoload-migration.star

# Disable checkpointing for small projects
bazelbump recipe run --no-checkpoint autoload-migration.star

# Increase checkpoint frequency for critical runs
bazelbump recipe run --checkpoint-interval 50 autoload-migration.star

# JSON progress output for CI
bazelbump recipe run --progress json autoload-migration.star
```

## State Manager Interface Usage

### Required Interface Methods

The executor uses these state.Manager methods:

```go
type Manager interface {
    // Run management
    CreateRun(recipe, version, projectPath, config string) (*Run, error)
    GetRun(id string) (*Run, error)
    GetLatestRun(projectPath string) (*Run, error)
    UpdateRunStatus(id string, status RunStatus) error
    UpdateRunProgress(id string, total, processed, changed int) error

    // Checkpoint management
    CreateCheckpoint(runID string, checkpointType CheckpointType, stepIndex, filesProcessed int, metadata string) (*Checkpoint, error)
    GetCheckpoint(id string) (*Checkpoint, error)
    GetLatestCheckpoint(runID string) (*Checkpoint, error)

    // File state tracking
    SetFileState(runID, filePath string, status FileStatus, contentHash string) error
    GetFileState(runID, filePath string) (*FileState, error)
    GetPendingFiles(runID string) ([]string, error)

    // Audit logging
    LogAction(runID, filePath, action string, details map[string]interface{}) error

    // Availability check
    Available() bool
}
```

## Orphaned Run Detection (Startup Validation)

**Problem**: If process crashes hard (SIGKILL), runs remain in "running" status forever.

**Solution**: Detect and remediate orphaned runs on startup:

```go
func (e *Executor) validateStateOnStartup(ctx *ExecutionContext) error {
    if ctx.StateManager == nil || !ctx.StateManager.Available() {
        return nil
    }

    // Mark runs stuck in "running" for > 1 hour as interrupted
    affected, err := ctx.StateManager.MarkOrphanedRunsInterrupted(1 * time.Hour)
    if err != nil {
        return fmt.Errorf("failed to validate state: %w", err)
    }

    if affected > 0 {
        log.Warnf("Marked %d orphaned runs as interrupted", affected)
    }

    return nil
}
```

## Cross-Platform File Locking

**Problem**: `flock()` doesn't work on Windows or NFS (common in CI).

**Solution**: Use cross-platform locking library:

```go
import "github.com/gofrs/flock"

// ProjectLock prevents concurrent bazelbump instances on same project
type ProjectLock struct {
    flock *flock.Flock
    path  string
}

func NewProjectLock(projectPath string) *ProjectLock {
    lockPath := filepath.Join(projectPath, ".bazelbump", "locks", "state.lock")
    os.MkdirAll(filepath.Dir(lockPath), 0755)

    return &ProjectLock{
        flock: flock.New(lockPath),
        path:  lockPath,
    }
}

func (l *ProjectLock) Acquire() error {
    locked, err := l.flock.TryLock()
    if err != nil {
        return fmt.Errorf("failed to acquire lock: %w", err)
    }
    if !locked {
        return state.ErrLockHeld
    }
    return nil
}

func (l *ProjectLock) Release() error {
    return l.flock.Unlock()
}

// Usage in executor
func (e *Executor) Execute(ctx *ExecutionContext, recipe *Recipe) error {
    // Acquire project lock
    lock := NewProjectLock(ctx.Path)
    if err := lock.Acquire(); err != nil {
        if errors.Is(err, state.ErrLockHeld) {
            return fmt.Errorf("another bazelbump instance is running on this project")
        }
        return err
    }
    defer lock.Release()

    // ... continue with execution
}
```

## Recipe Hash for Cache Invalidation

**Key Insight from Bazel**: Cache keys must include action definition, not just input content. If the recipe changes, cached results should be invalidated.

```go
func (e *Executor) initializeRun(ctx *ExecutionContext, recipe *Recipe) error {
    // Compute recipe hash
    recipeHash := computeRecipeHash(recipe)

    // Check if we should do incremental run
    if ctx.IncrementalFrom != "" {
        lastRun, err := ctx.StateManager.GetRun(ctx.IncrementalFrom)
        if err == nil && lastRun.RecipeHash != recipeHash {
            log.Warnf("Recipe changed since last run, forcing full re-run")
            ctx.IncrementalFrom = "" // Invalidate incremental
        }
    }

    // Create run with recipe hash
    run, err := ctx.StateManager.CreateRun(state.RunOptions{
        RecipeName:    recipe.Name,
        RecipeVersion: recipe.Version,
        RecipeHash:    recipeHash,  // NEW: Include recipe hash
        ProjectPath:   ctx.Path,
        Config:        serializeConfig(ctx),
    })
    // ...
}

func computeRecipeHash(recipe *Recipe) string {
    h := xxhash.New()

    // Hash recipe name and version
    h.Write([]byte(recipe.Name))
    h.Write([]byte(recipe.Version))

    // Hash step definitions (order matters)
    for _, step := range recipe.Steps {
        h.Write([]byte(step.Type))
        h.Write([]byte(step.Name))
        // Hash step parameters
        params, _ := json.Marshal(step.Params)
        h.Write(params)
    }

    return fmt.Sprintf("%016x", h.Sum64())
}
```

## File Hash Computation

```go
// pkg/state/hash.go

import (
    "io"
    "os"

    "github.com/cespare/xxhash/v2"
)

// ComputeFileHash computes xxHash of file contents
func ComputeFileHash(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()

    h := xxhash.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", err
    }

    return fmt.Sprintf("%016x", h.Sum64()), nil
}
```

## Error Handling

### Checkpoint Failures

Checkpoint failures should not fail the execution:

```go
func (e *Executor) createCheckpoint(ctx *ExecutionContext, t state.CheckpointType) {
    if err := e.doCreateCheckpoint(ctx, t); err != nil {
        // Log warning but continue
        ctx.StateManager.LogAction(ctx.RunID, "", "checkpoint_failed", map[string]interface{}{
            "error": err.Error(),
            "type":  string(t),
        })

        if ctx.Verbose {
            fmt.Fprintf(os.Stderr, "Warning: checkpoint failed: %v\n", err)
        }
    }
}
```

### State Manager Unavailable

When state manager is unavailable (nostate build or initialization failure), execution continues without state tracking:

```go
func (e *Executor) Execute(ctx *ExecutionContext) error {
    // Initialize state if available
    if ctx.StateManager != nil && ctx.StateManager.Available() {
        if err := e.initializeRun(ctx); err != nil {
            // Log but continue without state tracking
            fmt.Fprintf(os.Stderr, "Warning: state tracking disabled: %v\n", err)
            ctx.StateManager = nil
        }
    }

    // Execute recipe (works with or without state)
    return e.doExecute(ctx)
}
```

## Progress Reporter Selection

```go
func selectProgressReporter(mode string, stateManager state.Manager) state.ProgressReporter {
    switch mode {
    case "terminal":
        return state.NewTerminalReporter(os.Stdout, os.Stderr, true)
    case "json":
        return state.NewJSONReporter(os.Stdout)
    case "none":
        return state.NewNoopReporter()
    case "auto":
        fallthrough
    default:
        if isTerminal(os.Stdout) {
            return state.NewTerminalReporter(os.Stdout, os.Stderr, true)
        }
        return state.NewJSONReporter(os.Stdout)
    }
}

func isTerminal(f *os.File) bool {
    stat, _ := f.Stat()
    return (stat.Mode() & os.ModeCharDevice) != 0
}
```

## Testing

### Mock State Manager

```go
// pkg/state/mock.go

type MockManager struct {
    Runs        map[string]*Run
    Checkpoints map[string]*Checkpoint
    FileStates  map[string]map[string]*FileState
    Actions     []AuditLogEntry

    // Control behavior
    FailCreateRun       bool
    FailCreateCheckpoint bool
    available           bool
}

func NewMockManager() *MockManager {
    return &MockManager{
        Runs:        make(map[string]*Run),
        Checkpoints: make(map[string]*Checkpoint),
        FileStates:  make(map[string]map[string]*FileState),
        available:   true,
    }
}

func (m *MockManager) Available() bool {
    return m.available
}

// ... implement other methods
```

### Integration Tests

```go
func TestExecutorWithCheckpointing(t *testing.T) {
    // Create mock state manager
    sm := state.NewMockManager()

    // Create executor with state
    exec := recipe.NewExecutor(
        recipe.WithStateManager(sm),
        recipe.WithCheckpointConfig(recipe.CheckpointConfig{
            Enabled:      true,
            AutoInterval: 10,
            StepBoundary: true,
        }),
    )

    // Execute recipe
    err := exec.Execute(ctx, testRecipe)
    require.NoError(t, err)

    // Verify run was created
    assert.Len(t, sm.Runs, 1)
    run := sm.Runs[sm.RunIDs()[0]]
    assert.Equal(t, state.RunStatusCompleted, run.Status)

    // Verify checkpoints were created
    assert.NotEmpty(t, sm.Checkpoints)

    // Verify file states
    for file := range expectedFiles {
        fs := sm.FileStates[run.ID][file]
        assert.Equal(t, state.FileStatusCompleted, fs.Status)
    }
}

func TestExecutorResume(t *testing.T) {
    sm := state.NewMockManager()

    // Create a partially completed run
    run := &state.Run{
        ID:     "run-123",
        Status: state.RunStatusInterrupted,
    }
    sm.Runs[run.ID] = run

    // Mark some files as completed
    sm.FileStates[run.ID] = map[string]*state.FileState{
        "file1.go": {Status: state.FileStatusCompleted},
        "file2.go": {Status: state.FileStatusCompleted},
        "file3.go": {Status: state.FileStatusPending},
    }

    // Create checkpoint
    checkpoint := &state.Checkpoint{
        ID:             "chk-456",
        RunID:          run.ID,
        StepIndex:      1,
        FilesProcessed: 2,
    }
    sm.Checkpoints[checkpoint.ID] = checkpoint

    // Resume execution
    exec := recipe.NewExecutor(
        recipe.WithStateManager(sm),
        recipe.WithResume(run.ID, ""),
    )

    err := exec.Execute(ctx, testRecipe)
    require.NoError(t, err)

    // Verify only file3 was processed
    assert.Equal(t, state.FileStatusCompleted, sm.FileStates[run.ID]["file3.go"].Status)
}
```

## Migration Notes

### Backward Compatibility

- Existing recipe files work unchanged
- State tracking is opt-in (disabled with `--no-checkpoint`)
- `nostate` build tag removes all SQLite dependencies

### Deprecation Path

None required - this is a new feature.

## Implementation Checklist

### P0 - Critical (Before Production)

- [ ] **Atomic file updates** - Write to temp file, then rename
- [ ] **Cross-platform file locking** - Use gofrs/flock
- [ ] **Orphaned run detection** - Mark stuck "running" runs on startup
- [ ] **Graceful shutdown with flush** - Flush batched writes before exit

### P1 - Important (During Implementation)

- [ ] **Batched database writes** - Buffer file states, flush every N
- [ ] **Recipe hash for cache invalidation** - Invalidate when recipe changes
- [ ] **Staged file recovery** - Complete temp files on resume
- [ ] Extend ExecutionContext with state fields
- [ ] Add executor options for state configuration
- [ ] Implement run initialization in executor
- [ ] Add file processing hooks
- [ ] Implement auto checkpointing logic
- [ ] Add step boundary checkpointing
- [ ] Implement resume logic
- [ ] Add signal handling for graceful shutdown
- [ ] Update CLI with new flags
- [ ] Add file hash computation utility

### P2 - Post-Implementation

- [ ] Create mock state manager for testing
- [ ] Write integration tests
- [ ] Performance benchmarks on Envoy-scale project
- [ ] Update documentation

## Appendix: Full Execution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     bazelbump recipe run                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. Initialize State Manager                                     │
│    - Open/create .bazelbump/state.db                           │
│    - Check for existing interrupted run                         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. Create/Resume Run                                            │
│    - If --resume: load checkpoint, restore state                │
│    - Else: create new run record                                │
│    - Log run_start action                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. Parse Recipe                                                 │
│    - Load Starlark file                                         │
│    - Validate steps                                             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. For Each Step (starting from checkpoint.StepIndex)           │
│    ├─ Log step_start                                            │
│    ├─ Report step start to progress reporter                    │
│    │                                                            │
│    │  ┌──────────────────────────────────────────────────────┐ │
│    │  │ 4a. For Each File                                    │ │
│    │  │     ├─ Skip if completed (resume mode)               │ │
│    │  │     ├─ Compute before hash                           │ │
│    │  │     ├─ Set file state: in_progress                   │ │
│    │  │     ├─ Apply transforms                              │ │
│    │  │     ├─ Compute after hash                            │ │
│    │  │     ├─ Set file state: completed/failed              │ │
│    │  │     ├─ Log file action                               │ │
│    │  │     ├─ Report file progress                          │ │
│    │  │     └─ Maybe create auto checkpoint                  │ │
│    │  └──────────────────────────────────────────────────────┘ │
│    │                                                            │
│    ├─ Log step_complete                                         │
│    ├─ Report step complete to progress reporter                 │
│    └─ Create step_boundary checkpoint                           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. Finalize Run                                                 │
│    - Update run status: completed/failed/interrupted            │
│    - Update run progress counts                                 │
│    - Log run_complete action                                    │
│    - Report run complete to progress reporter                   │
│    - Create final checkpoint (if completed)                     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 6. Output Summary                                               │
│    - Print files changed                                        │
│    - Print run ID for future reference                          │
│    - Suggest resume command if interrupted                      │
└─────────────────────────────────────────────────────────────────┘
```

## References

- [Workflow Checkpoint Spec](workflow-checkpoint-spec.md) - Full specification with rationale
- [Checkpoint Spec Review](checkpoint-spec-review.md) - Comparison with Temporal, DBOS, Prefect, Airflow, Bazel
- [SQLite WAL Mode](https://sqlite.org/wal.html) - Why `synchronous=FULL` is critical
- [gofrs/flock](https://github.com/gofrs/flock) - Cross-platform file locking
