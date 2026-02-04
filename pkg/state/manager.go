// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package state provides workflow and checkpoint management for bazelbump.
//
// This package implements a state persistence system that enables:
//   - Progress tracking with real-time feedback
//   - Checkpointing for resumable execution
//   - Incremental execution to skip already-processed files
//   - Audit trail for change history
//   - State persistence across runs
//
// The state manager uses SQLite for persistence (via modernc.org/sqlite,
// a pure-Go implementation requiring no CGO). The system can be disabled
// at compile time using the "nostate" build tag for minimal builds.
//
// Usage:
//
//	// Create a state manager
//	mgr, err := state.New(".bazelbump/state.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mgr.Close()
//
//	// Create a new run
//	run, err := mgr.CreateRun("autoload-migration", state.RunOptions{
//	    ProjectPath: "/path/to/project",
//	    DryRun:      false,
//	})
//
//	// Track progress
//	mgr.UpdateRunProgress(run.ID, 100, 893, "source/server/BUILD")
//
//	// Create checkpoint
//	cp, err := mgr.CreateCheckpoint(run.ID, state.CheckpointOptions{
//	    Type:      state.CheckpointAuto,
//	    StepID:    "detect-native-rules",
//	    FileIndex: 100,
//	})
package state

import (
	"encoding/json"
	"errors"
	"time"
)

// Sentinel errors for consistent error handling.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrRunNotFound is returned when a run with the specified ID does not exist.
	ErrRunNotFound = errors.New("run not found")

	// ErrCheckpointNotFound is returned when a checkpoint with the specified ID does not exist.
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	// ErrAlreadyCompleted is returned when attempting to modify a completed run.
	ErrAlreadyCompleted = errors.New("run already completed")

	// ErrLockHeld is returned when another instance holds the state lock.
	ErrLockHeld = errors.New("another instance is running")

	// ErrStateCorrupted is returned when the state database is corrupted.
	ErrStateCorrupted = errors.New("state database corrupted")
)

// Manager provides workflow state management and checkpointing.
// It handles run tracking, checkpoints, file state, and audit logs.
//
// The Manager interface is implemented by sqliteManager (default build)
// and noopManager (nostate build tag).
type Manager interface {
	// Run management
	CreateRun(recipe string, opts RunOptions) (*Run, error)
	GetRun(id string) (*Run, error)
	GetLatestRun(recipe string) (*Run, error)
	UpdateRunStatus(id string, status RunStatus) error
	UpdateRunProgress(id string, processed, total int, currentFile string) error

	// Checkpoint management
	CreateCheckpoint(runID string, opts CheckpointOptions) (*Checkpoint, error)
	GetCheckpoint(id string) (*Checkpoint, error)
	GetLatestCheckpoint(runID string) (*Checkpoint, error)
	ListCheckpoints(runID string) ([]*Checkpoint, error)

	// File state tracking
	SetFileState(runID, path string, state FileState) error
	GetFileState(runID, path string) (*FileState, error)
	GetPendingFiles(runID string) ([]string, error)
	GetFilesByStatus(runID string, status FileStatus) ([]string, error)

	// History
	ListRuns(opts ListRunsOptions) ([]*Run, error)

	// Audit log
	LogAction(runID string, action AuditAction, opts AuditLogOptions) error

	// Maintenance
	ValidateOnStartup() error
	MarkOrphanedRunsInterrupted(timeout time.Duration) (int64, error)

	// Cleanup
	Close() error
}

// RunStatus represents the state of a recipe run.
type RunStatus string

const (
	// RunStatusPending indicates the run has been created but not started.
	RunStatusPending RunStatus = "pending"

	// RunStatusRunning indicates the run is currently executing.
	RunStatusRunning RunStatus = "running"

	// RunStatusCompleted indicates the run finished successfully.
	RunStatusCompleted RunStatus = "completed"

	// RunStatusFailed indicates the run failed with an error.
	RunStatusFailed RunStatus = "failed"

	// RunStatusInterrupted indicates the run was interrupted (e.g., SIGINT).
	RunStatusInterrupted RunStatus = "interrupted"

	// RunStatusCancelled indicates the run was cancelled by the user.
	RunStatusCancelled RunStatus = "cancelled"
)

// CheckpointType indicates how a checkpoint was created.
type CheckpointType string

const (
	// CheckpointAuto is created automatically at regular intervals.
	CheckpointAuto CheckpointType = "auto"

	// CheckpointManual is created by explicit user command.
	CheckpointManual CheckpointType = "manual"

	// CheckpointStepBoundary is created between recipe steps.
	CheckpointStepBoundary CheckpointType = "step_boundary"
)

// FileStatus represents the processing state of a file.
type FileStatus string

const (
	// FileStatusPending indicates the file has not been processed yet.
	FileStatusPending FileStatus = "pending"

	// FileStatusSkipped indicates the file was skipped (unchanged or excluded).
	FileStatusSkipped FileStatus = "skipped"

	// FileStatusProcessing indicates the file is currently being processed.
	FileStatusProcessing FileStatus = "processing"

	// FileStatusStaged indicates the file has been written to temp file but not yet renamed.
	// This enables atomic file updates and crash recovery.
	FileStatusStaged FileStatus = "staged"

	// FileStatusCompleted indicates the file was processed successfully.
	FileStatusCompleted FileStatus = "completed"

	// FileStatusFailed indicates the file processing failed.
	FileStatusFailed FileStatus = "failed"
)

// AuditAction represents an action recorded in the audit log.
type AuditAction string

const (
	AuditFileModified       AuditAction = "file_modified"
	AuditFileCreated        AuditAction = "file_created"
	AuditFileDeleted        AuditAction = "file_deleted"
	AuditCheckpointCreated  AuditAction = "checkpoint_created"
	AuditCheckpointRestored AuditAction = "checkpoint_restored"
	AuditRunStarted         AuditAction = "run_started"
	AuditRunCompleted       AuditAction = "run_completed"
	AuditRunFailed          AuditAction = "run_failed"
	AuditStepStarted        AuditAction = "step_started"
	AuditStepCompleted      AuditAction = "step_completed"
)

// Run represents a recipe execution.
type Run struct {
	// ID is the unique identifier (UUID).
	ID string `json:"id"`

	// RecipeName is the name of the recipe being run.
	RecipeName string `json:"recipe_name"`

	// RecipeVersion is the version of the recipe.
	RecipeVersion string `json:"recipe_version,omitempty"`

	// RecipeHash is the hash of the recipe content for cache invalidation.
	RecipeHash string `json:"recipe_hash,omitempty"`

	// ProjectPath is the root directory of the project.
	ProjectPath string `json:"project_path"`

	// StartedAt is when the run began.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the run finished (if completed).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Status is the current run status.
	Status RunStatus `json:"status"`

	// TotalFiles is the total number of files to process.
	TotalFiles int `json:"total_files"`

	// ProcessedFiles is the number of files processed so far.
	ProcessedFiles int `json:"processed_files"`

	// CurrentStep is the ID of the currently executing step.
	CurrentStep string `json:"current_step,omitempty"`

	// CurrentFile is the path of the file currently being processed.
	CurrentFile string `json:"current_file,omitempty"`

	// RetryCount is the number of times this run has been retried.
	RetryCount int `json:"retry_count"`

	// GitCommit is the HEAD commit at the start of the run.
	GitCommit string `json:"git_commit,omitempty"`

	// GitBranch is the branch name at the start of the run.
	GitBranch string `json:"git_branch,omitempty"`

	// BazelVersion is the detected Bazel version.
	BazelVersion string `json:"bazel_version,omitempty"`

	// DryRun indicates if this is a dry-run execution.
	DryRun bool `json:"dry_run"`

	// Config stores recipe-specific configuration as JSON.
	Config json.RawMessage `json:"config,omitempty"`

	// ContextOutputs stores outputs preserved across retries as JSON.
	ContextOutputs json.RawMessage `json:"context_outputs,omitempty"`

	// ErrorMessage contains the error if the run failed.
	ErrorMessage string `json:"error_message,omitempty"`

	// ChangesCount is the number of files changed.
	ChangesCount int `json:"changes_count"`
}

// RunOptions configures a new run.
type RunOptions struct {
	// RecipeVersion is the version of the recipe.
	RecipeVersion string

	// RecipeHash is the hash of the recipe content for cache invalidation.
	RecipeHash string

	// ProjectPath is the root directory of the project.
	ProjectPath string

	// DryRun indicates if this is a dry-run execution.
	DryRun bool

	// GitCommit is the HEAD commit at the start.
	GitCommit string

	// GitBranch is the branch name at the start.
	GitBranch string

	// BazelVersion is the detected Bazel version.
	BazelVersion string

	// Config stores recipe-specific configuration.
	Config map[string]any
}

// ListRunsOptions configures the ListRuns query.
type ListRunsOptions struct {
	// Recipe filters by recipe name.
	Recipe string

	// Status filters by run status.
	Status RunStatus

	// Since filters to runs started after this time.
	Since *time.Time

	// Limit is the maximum number of runs to return.
	Limit int
}

// Checkpoint represents a savepoint within a run.
type Checkpoint struct {
	// ID is the unique identifier (UUID).
	ID string `json:"id"`

	// RunID is the ID of the parent run.
	RunID string `json:"run_id"`

	// StepID is the ID of the step being executed.
	StepID string `json:"step_id"`

	// StepIndex is the index of the step (0-based).
	StepIndex int `json:"step_index"`

	// FileIndex is the index of the current file (0-based).
	FileIndex int `json:"file_index"`

	// CreatedAt is when the checkpoint was created.
	CreatedAt time.Time `json:"created_at"`

	// Type indicates how the checkpoint was created.
	Type CheckpointType `json:"checkpoint_type"`

	// State contains serialized execution state.
	State json.RawMessage `json:"state,omitempty"`

	// StateSize is the size of the serialized state in bytes.
	StateSize int `json:"state_size,omitempty"`

	// Description is an optional description.
	Description string `json:"description,omitempty"`
}

// CheckpointOptions configures a new checkpoint.
type CheckpointOptions struct {
	// Type indicates how the checkpoint is being created.
	Type CheckpointType

	// StepID is the ID of the current step.
	StepID string

	// StepIndex is the index of the current step.
	StepIndex int

	// FileIndex is the index of the current file.
	FileIndex int

	// Description is an optional description.
	Description string

	// State is the execution state to serialize.
	State *ExecutionState
}

// ExecutionState holds serializable execution state for checkpoints.
type ExecutionState struct {
	// ScanResults contains partial scan results.
	ScanResults map[string]any `json:"scan_results,omitempty"`

	// PendingOperations contains remaining operations.
	PendingOperations []any `json:"pending_operations,omitempty"`

	// ContextOutputs contains step outputs so far.
	ContextOutputs map[string]any `json:"context_outputs,omitempty"`

	// Statistics contains progress statistics.
	Statistics *ExecutionStats `json:"statistics,omitempty"`
}

// ExecutionStats holds execution statistics.
type ExecutionStats struct {
	FilesProcessed int   `json:"files_processed"`
	FilesModified  int   `json:"files_modified"`
	LinesAdded     int   `json:"lines_added"`
	LinesRemoved   int   `json:"lines_removed"`
	ElapsedMs      int64 `json:"elapsed_ms"`
}

// FileState represents the processing state of a single file.
type FileState struct {
	// RunID is the ID of the run this state belongs to.
	RunID string `json:"run_id"`

	// Path is the file path relative to project root.
	Path string `json:"path"`

	// Status is the processing status.
	Status FileStatus `json:"status"`

	// ContentHash is the xxHash of the file content before modification.
	ContentHash string `json:"content_hash,omitempty"`

	// AfterHash is the xxHash of the file content after modification.
	AfterHash string `json:"after_hash,omitempty"`

	// Mtime is the file modification time.
	Mtime int64 `json:"mtime,omitempty"`

	// FileExists tracks if the file still exists on disk.
	FileExists bool `json:"file_exists"`

	// StartedAt is when processing started.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when processing completed.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Changed indicates if the file was modified.
	Changed bool `json:"changed"`

	// LinesAdded is the number of lines added.
	LinesAdded int `json:"lines_added"`

	// LinesRemoved is the number of lines removed.
	LinesRemoved int `json:"lines_removed"`

	// ErrorMessage contains the error if processing failed.
	ErrorMessage string `json:"error_message,omitempty"`
}

// AuditLogOptions configures an audit log entry.
type AuditLogOptions struct {
	// FilePath is the file involved in the action.
	FilePath string

	// StepID is the step involved in the action.
	StepID string

	// Description describes what happened.
	Description string

	// DiffSummary is a "+N -M" style summary.
	DiffSummary string

	// OldValue is the value before the change.
	OldValue string

	// NewValue is the value after the change.
	NewValue string
}

// AuditLogEntry represents an entry in the audit log.
type AuditLogEntry struct {
	// ID is the auto-increment ID.
	ID int64 `json:"id"`

	// RunID is the ID of the run.
	RunID string `json:"run_id"`

	// Timestamp is when the action occurred.
	Timestamp time.Time `json:"timestamp"`

	// Action is the type of action.
	Action AuditAction `json:"action"`

	// FilePath is the file involved (if any).
	FilePath string `json:"file_path,omitempty"`

	// StepID is the step involved (if any).
	StepID string `json:"step_id,omitempty"`

	// Description describes what happened.
	Description string `json:"description,omitempty"`

	// DiffSummary is a "+N -M" style summary.
	DiffSummary string `json:"diff_summary,omitempty"`

	// OldValue is the value before the change.
	OldValue string `json:"old_value,omitempty"`

	// NewValue is the value after the change.
	NewValue string `json:"new_value,omitempty"`
}

// StateDBPath returns the default path for the state database.
func StateDBPath(projectPath string) string {
	return projectPath + "/.bazelbump/state.db"
}

// StateLockPath returns the default path for the state lock file.
func StateLockPath(projectPath string) string {
	return projectPath + "/.bazelbump/state.lock"
}
