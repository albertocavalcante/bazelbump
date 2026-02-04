//go:build nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"time"
)

// noopManager is a no-op implementation of Manager for builds without SQLite.
// All operations succeed silently but don't persist any state.
type noopManager struct{}

// New returns a no-op Manager for builds without SQLite support.
// All operations succeed silently but don't persist any state.
func New(dbPath string) (Manager, error) {
	return &noopManager{}, nil
}

// Available returns false because this build doesn't include SQLite support.
func Available() bool { return false }

// Close is a no-op.
func (m *noopManager) Close() error { return nil }

// =============================================================================
// Maintenance (no-op)
// =============================================================================

// ValidateOnStartup is a no-op.
func (m *noopManager) ValidateOnStartup() error { return nil }

// MarkOrphanedRunsInterrupted is a no-op, returns 0 affected rows.
func (m *noopManager) MarkOrphanedRunsInterrupted(timeout time.Duration) (int64, error) {
	return 0, nil
}

// =============================================================================
// Run Management (no-op)
// =============================================================================

// CreateRun returns a minimal Run struct without persisting.
func (m *noopManager) CreateRun(recipe string, opts RunOptions) (*Run, error) {
	return &Run{
		ID:            "noop-run",
		RecipeName:    recipe,
		RecipeVersion: opts.RecipeVersion,
		RecipeHash:    opts.RecipeHash,
		ProjectPath:   opts.ProjectPath,
		StartedAt:     time.Now(),
		Status:        RunStatusRunning,
		DryRun:        opts.DryRun,
	}, nil
}

// GetRun returns ErrRunNotFound (no state available).
func (m *noopManager) GetRun(id string) (*Run, error) {
	return nil, ErrRunNotFound
}

// GetLatestRun returns ErrRunNotFound (no state available).
func (m *noopManager) GetLatestRun(recipe string) (*Run, error) {
	return nil, ErrRunNotFound
}

// UpdateRunStatus is a no-op.
func (m *noopManager) UpdateRunStatus(id string, status RunStatus) error {
	return nil
}

// UpdateRunProgress is a no-op.
func (m *noopManager) UpdateRunProgress(id string, processed, total int, currentFile string) error {
	return nil
}

// ListRuns returns an empty list.
func (m *noopManager) ListRuns(opts ListRunsOptions) ([]*Run, error) {
	return nil, nil
}

// =============================================================================
// Checkpoint Management (no-op)
// =============================================================================

// CreateCheckpoint returns a minimal Checkpoint struct without persisting.
func (m *noopManager) CreateCheckpoint(runID string, opts CheckpointOptions) (*Checkpoint, error) {
	return &Checkpoint{
		ID:        "noop-checkpoint",
		RunID:     runID,
		StepID:    opts.StepID,
		StepIndex: opts.StepIndex,
		FileIndex: opts.FileIndex,
		CreatedAt: time.Now(),
		Type:      opts.Type,
	}, nil
}

// GetCheckpoint returns ErrCheckpointNotFound (no state available).
func (m *noopManager) GetCheckpoint(id string) (*Checkpoint, error) {
	return nil, ErrCheckpointNotFound
}

// GetLatestCheckpoint returns ErrCheckpointNotFound (no state available).
func (m *noopManager) GetLatestCheckpoint(runID string) (*Checkpoint, error) {
	return nil, ErrCheckpointNotFound
}

// ListCheckpoints returns an empty list.
func (m *noopManager) ListCheckpoints(runID string) ([]*Checkpoint, error) {
	return nil, nil
}

// =============================================================================
// File State Management (no-op)
// =============================================================================

// SetFileState is a no-op.
func (m *noopManager) SetFileState(runID, path string, state FileState) error {
	return nil
}

// GetFileState returns nil (no state available).
func (m *noopManager) GetFileState(runID, path string) (*FileState, error) {
	return nil, nil
}

// GetPendingFiles returns an empty list.
func (m *noopManager) GetPendingFiles(runID string) ([]string, error) {
	return nil, nil
}

// GetFilesByStatus returns an empty list.
func (m *noopManager) GetFilesByStatus(runID string, status FileStatus) ([]string, error) {
	return nil, nil
}

// =============================================================================
// Audit Log (no-op)
// =============================================================================

// LogAction is a no-op.
func (m *noopManager) LogAction(runID string, action AuditAction, opts AuditLogOptions) error {
	return nil
}
