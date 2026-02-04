// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/albertocavalcante/bazelbump/pkg/state"
)

// NOTE: stateAvailable() is defined in executor_state.go

// checkpointState holds the serializable state for a checkpoint.
type checkpointState struct {
	// StepIndex is the current step index (0-based).
	StepIndex int `json:"step_index"`

	// FilesProcessed is the number of files processed so far.
	FilesProcessed int `json:"files_processed"`

	// Outputs contains step outputs that need to be preserved.
	Outputs map[string]any `json:"outputs,omitempty"`

	// RecipeHash is the hash of the recipe for cache invalidation.
	RecipeHash string `json:"recipe_hash"`
}

// createCheckpoint creates a checkpoint at the current execution state.
// This method is nil-safe and will log warnings but not fail on errors.
func (e *ExecutorWithState) createCheckpoint(ctx *executionContext, reason CheckpointReason) {
	if e.stateOpts.stateManager == nil || !stateAvailable(e.stateOpts.stateManager) {
		return
	}

	if e.stateOpts.runID == "" {
		return // No active run
	}

	// Serialize the execution state
	stateData := &checkpointState{
		StepIndex:      ctx.stepIndex,
		FilesProcessed: e.stateOpts.filesProcessed,
		Outputs:        ctx.outputs,
		RecipeHash:     ctx.recipeHash,
	}

	execState := &state.ExecutionState{
		ContextOutputs: ctx.outputs,
		Statistics: &state.ExecutionStats{
			FilesProcessed: e.stateOpts.filesProcessed,
		},
	}

	// Determine checkpoint type
	checkpointType := state.CheckpointAuto
	switch reason {
	case CheckpointReasonStepBoundary:
		checkpointType = state.CheckpointStepBoundary
	case CheckpointReasonManual:
		checkpointType = state.CheckpointManual
	}

	// Create the checkpoint
	cp, err := e.stateOpts.stateManager.CreateCheckpoint(e.stateOpts.runID, state.CheckpointOptions{
		Type:        checkpointType,
		StepID:      ctx.currentStepID,
		StepIndex:   ctx.stepIndex,
		FileIndex:   e.stateOpts.filesProcessed,
		Description: fmt.Sprintf("%s checkpoint at step %d, file %d", reason, ctx.stepIndex, e.stateOpts.filesProcessed),
		State:       execState,
	})

	if err != nil {
		// Log warning but don't fail execution
		if e.verbose {
			fmt.Printf("Warning: failed to create checkpoint: %v\n", err)
		}
		// Also log to audit
		if e.stateOpts.stateManager != nil {
			e.stateOpts.stateManager.LogAction(e.stateOpts.runID, state.AuditCheckpointCreated, state.AuditLogOptions{
				Description: fmt.Sprintf("Checkpoint creation failed: %v", err),
			})
		}
		return
	}

	e.stateOpts.lastCheckpoint = time.Now()

	// Report checkpoint creation
	if e.stateOpts.progressReporter != nil {
		e.stateOpts.progressReporter.CheckpointCreated(cp.ID, e.stateOpts.filesProcessed)
	}

	// Log to audit
	e.stateOpts.stateManager.LogAction(e.stateOpts.runID, state.AuditCheckpointCreated, state.AuditLogOptions{
		Description: fmt.Sprintf("Created %s checkpoint: %s", reason, cp.ID),
	})

	// Keep reference to serialized state for debugging
	_ = stateData
}

// shouldAutoCheckpoint determines if an auto checkpoint should be created.
func (e *ExecutorWithState) shouldAutoCheckpoint() bool {
	if e.stateOpts.checkpointConfig == nil || !e.stateOpts.checkpointConfig.Enabled {
		return false
	}

	if e.stateOpts.stateManager == nil || !stateAvailable(e.stateOpts.stateManager) {
		return false
	}

	// Check file interval
	if e.stateOpts.checkpointConfig.AutoInterval <= 0 {
		return false
	}

	// Don't checkpoint at 0 files
	if e.stateOpts.filesProcessed == 0 {
		return false
	}

	if e.stateOpts.filesProcessed%e.stateOpts.checkpointConfig.AutoInterval != 0 {
		return false
	}

	// Check minimum time between checkpoints
	if time.Since(e.stateOpts.lastCheckpoint) < e.stateOpts.checkpointConfig.MinTimeBetween {
		return false
	}

	return true
}

// restoreFromCheckpoint restores execution state from a checkpoint.
// Returns the checkpoint that was restored from, or nil if not resuming.
func (e *ExecutorWithState) restoreFromCheckpoint(ctx *executionContext) (*state.Checkpoint, error) {
	if e.stateOpts.stateManager == nil || !stateAvailable(e.stateOpts.stateManager) {
		return nil, nil
	}

	if e.stateOpts.resumeFromRun == "" {
		return nil, nil
	}

	// Get the run to resume from
	run, err := e.stateOpts.stateManager.GetRun(e.stateOpts.resumeFromRun)
	if err != nil {
		return nil, fmt.Errorf("failed to get run %s: %w", e.stateOpts.resumeFromRun, err)
	}

	if run.Status == state.RunStatusCompleted {
		return nil, fmt.Errorf("run %s already completed", run.ID)
	}

	// Get the checkpoint to restore from
	var checkpoint *state.Checkpoint
	if e.stateOpts.resumeCheckpoint != "" {
		checkpoint, err = e.stateOpts.stateManager.GetCheckpoint(e.stateOpts.resumeCheckpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to get checkpoint %s: %w", e.stateOpts.resumeCheckpoint, err)
		}
	} else {
		checkpoint, err = e.stateOpts.stateManager.GetLatestCheckpoint(run.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest checkpoint for run %s: %w", run.ID, err)
		}
	}

	if checkpoint == nil {
		return nil, fmt.Errorf("no checkpoint found for run %s", run.ID)
	}

	// Restore the execution state
	e.stateOpts.runID = run.ID
	ctx.stepIndex = checkpoint.StepIndex
	e.stateOpts.filesProcessed = checkpoint.FileIndex
	e.stateOpts.skipCompleted = true

	// Deserialize state if available
	if len(checkpoint.State) > 0 {
		var execState state.ExecutionState
		if err := json.Unmarshal(checkpoint.State, &execState); err == nil {
			// Restore outputs
			if execState.ContextOutputs != nil {
				ctx.outputs = execState.ContextOutputs
			}
		}
	}

	// Update run status back to running
	if err := e.stateOpts.stateManager.UpdateRunStatus(run.ID, state.RunStatusRunning); err != nil {
		// Log warning but continue
		if e.verbose {
			fmt.Printf("Warning: failed to update run status: %v\n", err)
		}
	}

	// Log resume
	e.stateOpts.stateManager.LogAction(run.ID, state.AuditRunStarted, state.AuditLogOptions{
		Description: fmt.Sprintf("Resumed from checkpoint %s at step %d, file %d",
			checkpoint.ID, checkpoint.StepIndex, checkpoint.FileIndex),
	})

	// Report checkpoint restoration
	if e.stateOpts.progressReporter != nil {
		e.stateOpts.progressReporter.CheckpointRestored(checkpoint.ID)
	}

	return checkpoint, nil
}

// computeRecipeHash computes a hash of the recipe definition.
// This is used for cache invalidation - if the recipe changes,
// cached results should be invalidated.
func computeRecipeHash(recipe *Recipe) string {
	h := sha256.New()

	// Hash recipe name and version
	h.Write([]byte(recipe.Name))
	h.Write([]byte{0}) // separator
	h.Write([]byte(recipe.Version))
	h.Write([]byte{0})

	// Hash step definitions (order matters)
	for _, step := range recipe.Steps {
		h.Write([]byte(step.GetID()))
		h.Write([]byte{0})
		h.Write([]byte(step.GetName()))
		h.Write([]byte{0})

		// Hash step-specific content
		switch s := step.(type) {
		case *ScanStep:
			h.Write([]byte("scan"))
			for _, p := range s.FilePatterns {
				h.Write([]byte(p))
			}
			for _, p := range s.Patterns {
				h.Write([]byte(p))
			}
		case *EditStep:
			h.Write([]byte("edit"))
			// Hash operations
			for _, op := range s.Operations {
				h.Write([]byte(op.GetType()))
			}
		case *VerifyStep:
			h.Write([]byte("verify"))
			h.Write([]byte(s.Command))
		}
		h.Write([]byte{0})
	}

	// Hash failure mode
	h.Write([]byte(recipe.OnFailure))

	return hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars for brevity
}

// executionContext holds runtime state during recipe execution with state management.
type executionContext struct {
	// Inherited from base Context
	projectPath string
	dryRun      bool
	verbose     bool
	outputs     map[string]any
	changes     []*FileChange
	tracker     FileTracker

	// State management additions
	stepIndex     int
	currentStepID string
	recipeHash    string
	startTime     time.Time
}

// newExecutionContext creates a new execution context from base Context.
func newExecutionContext(ctx *Context, recipe *Recipe) *executionContext {
	return &executionContext{
		projectPath: ctx.ProjectPath,
		dryRun:      ctx.DryRun,
		verbose:     ctx.Verbose,
		outputs:     ctx.Outputs,
		changes:     ctx.Changes,
		tracker:     ctx.Tracker,
		recipeHash:  computeRecipeHash(recipe),
		startTime:   time.Now(),
	}
}

// toBaseContext converts back to a base Context for operations that need it.
func (ec *executionContext) toBaseContext() *Context {
	return &Context{
		ProjectPath: ec.projectPath,
		DryRun:      ec.dryRun,
		Verbose:     ec.verbose,
		Outputs:     ec.outputs,
		Changes:     ec.changes,
		Tracker:     ec.tracker,
	}
}
