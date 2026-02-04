// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"time"

	"github.com/albertocavalcante/bazelbump/pkg/state"
)

// ExecutorOption is a functional option for configuring the Executor.
type ExecutorOption func(*executorOptions)

// executorOptions holds all configurable options for the Executor.
// These are applied via functional options in NewExecutorWithOptions.
type executorOptions struct {
	// State management (nil-safe, check Available())
	stateManager     state.Manager
	progressReporter ProgressReporter

	// Run tracking
	runID string

	// Checkpoint config
	checkpointConfig *CheckpointConfig

	// Resume support
	resumeFromRun    string
	resumeCheckpoint string
	skipCompleted    bool

	// Internal tracking (initialized at runtime)
	filesProcessed int
	lastCheckpoint time.Time
}

// CheckpointConfig configures automatic checkpointing during execution.
type CheckpointConfig struct {
	// Enabled determines if checkpointing is active.
	Enabled bool

	// AutoInterval is the number of files between auto checkpoints.
	// Default: 100. Set to 0 to disable file-based checkpointing.
	AutoInterval int

	// StepBoundary creates a checkpoint at step boundaries.
	// Default: true.
	StepBoundary bool

	// MinTimeBetween is the minimum time between checkpoints.
	// This prevents excessive checkpointing for small intervals.
	// Default: 30 seconds.
	MinTimeBetween time.Duration

	// MaxCheckpoints is the maximum number of checkpoints to keep per run.
	// Older checkpoints are cleaned up automatically.
	// Default: 10.
	MaxCheckpoints int
}

// DefaultCheckpointConfig returns the default checkpoint configuration.
func DefaultCheckpointConfig() *CheckpointConfig {
	return &CheckpointConfig{
		Enabled:        true,
		AutoInterval:   100,
		StepBoundary:   true,
		MinTimeBetween: 30 * time.Second,
		MaxCheckpoints: 10,
	}
}

// WithStateManager sets the state manager for run tracking and checkpointing.
// If nil, state management is disabled.
func WithStateManager(sm state.Manager) ExecutorOption {
	return func(o *executorOptions) {
		o.stateManager = sm
	}
}

// WithProgressReporter sets the progress reporter for execution feedback.
// If nil, no progress reporting is performed.
func WithProgressReporter(pr ProgressReporter) ExecutorOption {
	return func(o *executorOptions) {
		o.progressReporter = pr
	}
}

// WithCheckpointConfig sets the checkpoint configuration.
// If nil, checkpointing is disabled.
func WithCheckpointConfig(cfg *CheckpointConfig) ExecutorOption {
	return func(o *executorOptions) {
		o.checkpointConfig = cfg
	}
}

// WithResume configures the executor to resume from an interrupted run.
// runID is the ID of the run to resume.
// checkpointID is optional - if empty, resumes from the latest checkpoint.
func WithResume(runID, checkpointID string) ExecutorOption {
	return func(o *executorOptions) {
		o.resumeFromRun = runID
		o.resumeCheckpoint = checkpointID
		o.skipCompleted = true
	}
}

// WithRunID sets a specific run ID to use instead of generating one.
// This is primarily useful for testing.
func WithRunID(runID string) ExecutorOption {
	return func(o *executorOptions) {
		o.runID = runID
	}
}

// ProgressReporter receives execution progress events.
// Implementations can display progress to terminals, log to files,
// or emit JSON for CI/tooling integration.
type ProgressReporter interface {
	// Run lifecycle
	RunStart(runID, recipeName string)
	RunComplete(runID string, success bool, durationMs int64)
	RunError(runID string, err error)

	// Step lifecycle
	StepStart(stepName string, stepIndex, totalSteps int)
	StepComplete(stepName string, success bool, description string)

	// File lifecycle
	FileStart(path string)
	FileChanged(path string)
	FileUnchanged(path string)
	FileSkipped(path, reason string)
	FileError(path string, err error)

	// Checkpoints
	CheckpointCreated(checkpointID string, filesProcessed int)
	CheckpointRestored(checkpointID string)

	// Messages
	Info(format string, args ...any)
	Warning(format string, args ...any)
}

// NoopProgressReporter is a ProgressReporter that does nothing.
// Used when progress reporting is disabled.
type NoopProgressReporter struct{}

func (n *NoopProgressReporter) RunStart(runID, recipeName string)                    {}
func (n *NoopProgressReporter) RunComplete(runID string, success bool, durationMs int64) {}
func (n *NoopProgressReporter) RunError(runID string, err error)                     {}
func (n *NoopProgressReporter) StepStart(stepName string, stepIndex, totalSteps int) {}
func (n *NoopProgressReporter) StepComplete(stepName string, success bool, description string) {}
func (n *NoopProgressReporter) FileStart(path string)                                {}
func (n *NoopProgressReporter) FileChanged(path string)                              {}
func (n *NoopProgressReporter) FileUnchanged(path string)                            {}
func (n *NoopProgressReporter) FileSkipped(path, reason string)                      {}
func (n *NoopProgressReporter) FileError(path string, err error)                     {}
func (n *NoopProgressReporter) CheckpointCreated(checkpointID string, filesProcessed int) {}
func (n *NoopProgressReporter) CheckpointRestored(checkpointID string)               {}
func (n *NoopProgressReporter) Info(format string, args ...any)                      {}
func (n *NoopProgressReporter) Warning(format string, args ...any)                   {}

// CheckpointReason indicates why a checkpoint was created.
type CheckpointReason string

const (
	// CheckpointReasonAuto is an automatic checkpoint based on file interval.
	CheckpointReasonAuto CheckpointReason = "auto"

	// CheckpointReasonStepBoundary is a checkpoint between recipe steps.
	CheckpointReasonStepBoundary CheckpointReason = "step_boundary"

	// CheckpointReasonInterrupt is a checkpoint created due to SIGINT/SIGTERM.
	CheckpointReasonInterrupt CheckpointReason = "interrupt"

	// CheckpointReasonManual is a checkpoint created by explicit user request.
	CheckpointReasonManual CheckpointReason = "manual"
)
