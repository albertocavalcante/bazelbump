// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/albertocavalcante/bazelbump/pkg/state"
	"github.com/google/uuid"
)

// stateAvailable checks if state management is available.
// This checks both that we have a manager AND that the package-level
// Available() returns true (indicating non-nostate build).
func stateAvailable(sm state.Manager) bool {
	return sm != nil && state.Available()
}

// ExecutorWithState extends Executor with state management capabilities.
// It provides checkpointing, progress tracking, and resume functionality.
type ExecutorWithState struct {
	*Executor
	stateOpts executorOptions
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewExecutorWithOptions creates a new executor with state management options.
func NewExecutorWithOptions(projectPath string, opts Options, execOpts ...ExecutorOption) *ExecutorWithState {
	baseExecutor := NewExecutor(projectPath, opts)

	e := &ExecutorWithState{
		Executor:  baseExecutor,
		stateOpts: executorOptions{},
	}

	// Apply functional options
	for _, opt := range execOpts {
		opt(&e.stateOpts)
	}

	// Set up context with cancellation for signal handling
	e.ctx, e.cancel = context.WithCancel(context.Background())

	return e
}

// ExecuteWithState runs a recipe with full state management.
// This method:
// - Creates or resumes a run
// - Tracks progress
// - Creates checkpoints
// - Handles interrupts gracefully
func (e *ExecutorWithState) ExecuteWithState(recipe *Recipe, opts Options) (*Result, error) {
	// Set up signal handling
	e.setupSignalHandler(recipe)
	defer e.cancel()

	// Create execution context
	ctx := &Context{
		ProjectPath: e.projectPath,
		DryRun:      e.dryRun || opts.DryRun,
		Verbose:     e.verbose || opts.Verbose,
		Outputs:     make(map[string]any),
		Changes:     make([]*FileChange, 0),
		Tracker:     e.tracker,
	}

	execCtx := newExecutionContext(ctx, recipe)

	// Start or resume run
	if err := e.startRun(recipe, execCtx); err != nil {
		return nil, fmt.Errorf("failed to start run: %w", err)
	}

	// Handle resume if requested
	checkpoint, err := e.restoreFromCheckpoint(execCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to restore from checkpoint: %w", err)
	}

	startStepIndex := 0
	if checkpoint != nil {
		startStepIndex = checkpoint.StepIndex
	}

	// Report run start
	if e.stateOpts.progressReporter != nil {
		e.stateOpts.progressReporter.RunStart(e.stateOpts.runID, recipe.Name)
	}

	startTime := time.Now()

	// Phase 2: Validation (Phase 1 is done by Interpret)
	validationErrors := e.validate(recipe)
	if len(validationErrors) > 0 {
		e.endRun(false, fmt.Errorf("validation failed: %v", validationErrors[0]))
		return &Result{
			Recipe:  recipe.Name,
			Success: false,
			Error:   NewValidationError(validationErrors...),
		}, nil
	}

	// Phase 3: Execution with state tracking
	result, err := e.executeWithState(recipe, opts, execCtx, startStepIndex)

	// End run
	durationMs := time.Since(startTime).Milliseconds()
	if result != nil {
		e.endRun(result.Success, result.Error)
	} else {
		e.endRun(false, err)
	}

	// Report run completion
	if e.stateOpts.progressReporter != nil {
		success := result != nil && result.Success
		e.stateOpts.progressReporter.RunComplete(e.stateOpts.runID, success, durationMs)
	}

	return result, err
}

// setupSignalHandler registers handlers for SIGINT and SIGTERM.
func (e *ExecutorWithState) setupSignalHandler(recipe *Recipe) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigChan:
			if e.verbose {
				fmt.Printf("\nReceived signal %v, creating interrupt checkpoint...\n", sig)
			}

			// Create interrupt checkpoint
			if e.stateOpts.stateManager != nil && stateAvailable(e.stateOpts.stateManager) && e.stateOpts.runID != "" {
				// Create a minimal execution context for the checkpoint
				execCtx := &executionContext{
					recipeHash: computeRecipeHash(recipe),
				}
				e.createCheckpoint(execCtx, CheckpointReasonInterrupt)

				// Mark run as interrupted
				e.stateOpts.stateManager.UpdateRunStatus(e.stateOpts.runID, state.RunStatusInterrupted)
				e.stateOpts.stateManager.LogAction(e.stateOpts.runID, state.AuditRunCompleted, state.AuditLogOptions{
					Description: fmt.Sprintf("Run interrupted by signal %v", sig),
				})
			}

			// Cancel context to stop execution
			e.cancel()

			// Re-raise signal for default handling
			signal.Reset(sig.(syscall.Signal))
			syscall.Kill(syscall.Getpid(), sig.(syscall.Signal))

		case <-e.ctx.Done():
			// Context cancelled, clean up
			signal.Stop(sigChan)
		}
	}()
}

// startRun creates a new run record or prepares for resume.
func (e *ExecutorWithState) startRun(recipe *Recipe, execCtx *executionContext) error {
	if e.stateOpts.stateManager == nil || !stateAvailable(e.stateOpts.stateManager) {
		// State management not available, use a random ID for logging
		if e.stateOpts.runID == "" {
			e.stateOpts.runID = uuid.New().String()
		}
		return nil
	}

	// If resuming, don't create a new run
	if e.stateOpts.resumeFromRun != "" {
		return nil
	}

	// Generate run ID if not provided
	if e.stateOpts.runID == "" {
		e.stateOpts.runID = uuid.New().String()
	}

	// Create run options
	runOpts := state.RunOptions{
		RecipeVersion: recipe.Version,
		RecipeHash:    execCtx.recipeHash,
		ProjectPath:   e.projectPath,
		DryRun:        e.dryRun,
	}

	// Try to get git info
	if gitCommit, gitBranch := getGitInfo(e.projectPath); gitCommit != "" {
		runOpts.GitCommit = gitCommit
		runOpts.GitBranch = gitBranch
	}

	// Create run record
	run, err := e.stateOpts.stateManager.CreateRun(recipe.Name, runOpts)
	if err != nil {
		// Log warning but don't fail - state management is optional
		if e.verbose {
			fmt.Printf("Warning: failed to create run record: %v\n", err)
		}
		return nil
	}

	e.stateOpts.runID = run.ID

	// Log run start
	e.stateOpts.stateManager.LogAction(run.ID, state.AuditRunStarted, state.AuditLogOptions{
		Description: fmt.Sprintf("Started recipe %s v%s", recipe.Name, recipe.Version),
	})

	return nil
}

// endRun finalizes the run status.
func (e *ExecutorWithState) endRun(success bool, runErr error) {
	if e.stateOpts.stateManager == nil || !stateAvailable(e.stateOpts.stateManager) {
		return
	}

	if e.stateOpts.runID == "" {
		return
	}

	var status state.RunStatus
	if success {
		status = state.RunStatusCompleted
	} else {
		status = state.RunStatusFailed
	}

	if err := e.stateOpts.stateManager.UpdateRunStatus(e.stateOpts.runID, status); err != nil {
		if e.verbose {
			fmt.Printf("Warning: failed to update run status: %v\n", err)
		}
	}

	// Log completion
	var action state.AuditAction
	var description string
	if success {
		action = state.AuditRunCompleted
		description = "Run completed successfully"
	} else {
		action = state.AuditRunFailed
		description = "Run failed"
		if runErr != nil {
			description = fmt.Sprintf("Run failed: %v", runErr)
		}
	}

	e.stateOpts.stateManager.LogAction(e.stateOpts.runID, action, state.AuditLogOptions{
		Description: description,
	})
}

// executeWithState runs the recipe steps with state tracking.
func (e *ExecutorWithState) executeWithState(recipe *Recipe, opts Options, execCtx *executionContext, startStepIndex int) (*Result, error) {
	result := &Result{
		Recipe:      recipe.Name,
		StepResults: make([]*StepResult, 0),
		Changes:     make([]*FileChange, 0),
	}

	// Get topologically sorted steps
	sortedSteps, err := e.topologicalSort(recipe.Steps)
	if err != nil {
		return nil, fmt.Errorf("failed to sort steps: %w", err)
	}

	// Execute steps in order
	for i, step := range sortedSteps {
		// Check for cancellation
		select {
		case <-e.ctx.Done():
			result.Success = false
			result.Error = fmt.Errorf("execution interrupted")
			return result, nil
		default:
		}

		// Skip steps before the resume point
		if i < startStepIndex {
			continue
		}

		// Skip if running specific step
		if opts.StepID != "" && step.GetID() != opts.StepID {
			continue
		}

		// Skip verify steps if requested
		if opts.NoVerify {
			if _, ok := step.(*VerifyStep); ok {
				continue
			}
		}

		// Update current step tracking
		execCtx.stepIndex = i
		execCtx.currentStepID = step.GetID()

		// Report step start
		if e.stateOpts.progressReporter != nil {
			e.stateOpts.progressReporter.StepStart(step.GetName(), i, len(sortedSteps))
		}

		// Log step start
		if e.stateOpts.stateManager != nil && stateAvailable(e.stateOpts.stateManager) {
			e.stateOpts.stateManager.LogAction(e.stateOpts.runID, state.AuditStepStarted, state.AuditLogOptions{
				StepID:      step.GetID(),
				Description: fmt.Sprintf("Starting step: %s", step.GetName()),
			})
		}

		// Execute step
		ctx := execCtx.toBaseContext()
		stepResult := e.executeStepWithState(ctx, step, execCtx)
		result.StepResults = append(result.StepResults, stepResult)
		result.StepsRun++

		// Store output for dependent steps
		if stepResult.Output != nil {
			execCtx.outputs[step.GetID()] = stepResult.Output
			ctx.Outputs[step.GetID()] = stepResult.Output
		}

		// Collect changes
		result.Changes = append(result.Changes, stepResult.Changes...)
		execCtx.changes = append(execCtx.changes, stepResult.Changes...)

		// Report step completion
		if e.stateOpts.progressReporter != nil {
			e.stateOpts.progressReporter.StepComplete(step.GetName(), stepResult.Success, stepResult.Description)
		}

		// Log step completion
		if e.stateOpts.stateManager != nil && stateAvailable(e.stateOpts.stateManager) {
			e.stateOpts.stateManager.LogAction(e.stateOpts.runID, state.AuditStepCompleted, state.AuditLogOptions{
				StepID:      step.GetID(),
				Description: fmt.Sprintf("Step completed: %s - %s", step.GetName(), stepResult.Description),
			})
		}

		// Create step boundary checkpoint if configured
		if e.stateOpts.checkpointConfig != nil && e.stateOpts.checkpointConfig.StepBoundary {
			e.createCheckpoint(execCtx, CheckpointReasonStepBoundary)
		}

		// Handle failure
		if !stepResult.Success {
			result.Error = stepResult.Error
			result.Success = false

			// Report error
			if e.stateOpts.progressReporter != nil {
				e.stateOpts.progressReporter.RunError(e.stateOpts.runID, stepResult.Error)
			}

			// Rollback if configured
			if recipe.OnFailure == "rollback" && !execCtx.dryRun {
				if rollbackErr := e.rollback(ctx); rollbackErr != nil {
					result.Error = fmt.Errorf("%w (rollback also failed: %v)", result.Error, rollbackErr)
				}
			}

			// Stop unless configured to continue
			if recipe.OnFailure != "continue" {
				return result, nil
			}
		}
	}

	result.Success = result.Error == nil
	return result, nil
}

// executeStepWithState runs a single step with file state tracking.
func (e *ExecutorWithState) executeStepWithState(ctx *Context, step Step, execCtx *executionContext) *StepResult {
	// Execute the step using the base executor
	result := e.executeStep(ctx, step)

	// Track file state for changes
	if e.stateOpts.stateManager != nil && stateAvailable(e.stateOpts.stateManager) {
		for _, change := range result.Changes {
			e.trackFileChange(change, execCtx)
		}
	}

	return result
}

// trackFileChange updates file state and checks for auto-checkpoint.
func (e *ExecutorWithState) trackFileChange(change *FileChange, execCtx *executionContext) {
	if e.stateOpts.stateManager == nil || !stateAvailable(e.stateOpts.stateManager) {
		return
	}

	e.stateOpts.filesProcessed++

	// Determine file status
	status := state.FileStatusCompleted
	changed := change.NewContent != nil && len(change.NewContent) > 0

	// Update file state
	fileState := state.FileState{
		RunID:   e.stateOpts.runID,
		Path:    change.Path,
		Status:  status,
		Changed: changed,
	}

	// Calculate lines changed
	if changed {
		fileState.LinesAdded, fileState.LinesRemoved = countLineChanges(change)
	}

	if err := e.stateOpts.stateManager.SetFileState(e.stateOpts.runID, change.Path, fileState); err != nil {
		if e.verbose {
			fmt.Printf("Warning: failed to set file state for %s: %v\n", change.Path, err)
		}
	}

	// Log file change
	if changed {
		e.stateOpts.stateManager.LogAction(e.stateOpts.runID, state.AuditFileModified, state.AuditLogOptions{
			FilePath:    change.Path,
			StepID:      execCtx.currentStepID,
			Description: change.Description,
			DiffSummary: fmt.Sprintf("+%d -%d", fileState.LinesAdded, fileState.LinesRemoved),
		})

		// Report file change
		if e.stateOpts.progressReporter != nil {
			e.stateOpts.progressReporter.FileChanged(change.Path)
		}
	}

	// Update progress
	if e.stateOpts.stateManager != nil && stateAvailable(e.stateOpts.stateManager) {
		e.stateOpts.stateManager.UpdateRunProgress(e.stateOpts.runID, e.stateOpts.filesProcessed, 0, change.Path)
	}

	// Check for auto-checkpoint
	if e.shouldAutoCheckpoint() {
		e.createCheckpoint(execCtx, CheckpointReasonAuto)
	}
}

// countLineChanges counts the lines added and removed in a change.
func countLineChanges(change *FileChange) (added, removed int) {
	for _, hunk := range change.Hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case "add":
				added++
			case "remove":
				removed++
			}
		}
	}
	return
}

// getGitInfo retrieves the current git commit and branch.
func getGitInfo(projectPath string) (commit, branch string) {
	// Try to read HEAD
	headPath := projectPath + "/.git/HEAD"
	headContent, err := os.ReadFile(headPath)
	if err != nil {
		return "", ""
	}

	// Parse HEAD content
	headStr := string(headContent)
	if len(headStr) > 5 && headStr[:5] == "ref: " {
		// Symbolic reference
		ref := headStr[5:]
		ref = ref[:len(ref)-1] // Remove newline

		// Extract branch name
		if len(ref) > 11 && ref[:11] == "refs/heads/" {
			branch = ref[11:]
		}

		// Read the commit hash
		refPath := projectPath + "/.git/" + ref
		commitContent, err := os.ReadFile(refPath)
		if err == nil {
			commit = string(commitContent)
			if len(commit) > 0 {
				commit = commit[:len(commit)-1] // Remove newline
			}
		}
	} else {
		// Detached HEAD
		commit = headStr[:len(headStr)-1]
	}

	return
}

// GetRunID returns the current run ID.
func (e *ExecutorWithState) GetRunID() string {
	return e.stateOpts.runID
}

// Available returns whether state management is available.
func (e *ExecutorWithState) Available() bool {
	return e.stateOpts.stateManager != nil && stateAvailable(e.stateOpts.stateManager)
}
