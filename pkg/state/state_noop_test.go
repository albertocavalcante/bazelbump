//go:build nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"errors"
	"testing"
	"time"
)

func TestNoopManagerAvailable(t *testing.T) {
	if Available() {
		t.Error("Available() should return false in nostate build")
	}
}

func TestNoopManagerNew(t *testing.T) {
	mgr, err := New("/nonexistent/path/state.db")
	if err != nil {
		t.Fatalf("New() should not fail in nostate build: %v", err)
	}
	defer mgr.Close()

	// Verify all operations succeed silently
	run, err := mgr.CreateRun("test-recipe", RunOptions{
		ProjectPath: "/test",
		RecipeHash:  "test-hash",
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}
	if run == nil {
		t.Fatal("CreateRun should return a minimal run struct")
	}
	if run.ID == "" {
		t.Error("Run ID should be set to 'noop-run'")
	}
	if run.RecipeHash != "test-hash" {
		t.Errorf("RecipeHash = %q, want %q", run.RecipeHash, "test-hash")
	}

	// GetRun should return ErrRunNotFound
	_, err = mgr.GetRun("any-id")
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("GetRun should return ErrRunNotFound, got %v", err)
	}

	// GetLatestRun should return ErrRunNotFound
	_, err = mgr.GetLatestRun("any-recipe")
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("GetLatestRun should return ErrRunNotFound, got %v", err)
	}

	// ListRuns should return empty
	runs, err := mgr.ListRuns(ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if runs != nil && len(runs) > 0 {
		t.Error("ListRuns should return empty in nostate build")
	}

	// Checkpoint operations
	cp, err := mgr.CreateCheckpoint(run.ID, CheckpointOptions{
		StepID:    "test-step",
		FileIndex: 100,
	})
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}
	if cp == nil {
		t.Fatal("CreateCheckpoint should return a minimal checkpoint")
	}

	// GetCheckpoint should return ErrCheckpointNotFound
	_, err = mgr.GetCheckpoint("any-id")
	if !errors.Is(err, ErrCheckpointNotFound) {
		t.Errorf("GetCheckpoint should return ErrCheckpointNotFound, got %v", err)
	}

	// GetLatestCheckpoint should return ErrCheckpointNotFound
	_, err = mgr.GetLatestCheckpoint(run.ID)
	if !errors.Is(err, ErrCheckpointNotFound) {
		t.Errorf("GetLatestCheckpoint should return ErrCheckpointNotFound, got %v", err)
	}

	// File state operations
	if err := mgr.SetFileState(run.ID, "test/BUILD", FileState{Status: FileStatusPending}); err != nil {
		t.Fatalf("SetFileState failed: %v", err)
	}

	fs, err := mgr.GetFileState(run.ID, "test/BUILD")
	if err != nil {
		t.Fatalf("GetFileState failed: %v", err)
	}
	if fs != nil {
		t.Error("GetFileState should return nil in nostate build")
	}

	// GetFilesByStatus should return empty
	files, err := mgr.GetFilesByStatus(run.ID, FileStatusPending)
	if err != nil {
		t.Fatalf("GetFilesByStatus failed: %v", err)
	}
	if files != nil && len(files) > 0 {
		t.Error("GetFilesByStatus should return empty in nostate build")
	}

	// Log action
	if err := mgr.LogAction(run.ID, AuditFileModified, AuditLogOptions{}); err != nil {
		t.Fatalf("LogAction failed: %v", err)
	}

	// Maintenance operations
	if err := mgr.ValidateOnStartup(); err != nil {
		t.Fatalf("ValidateOnStartup failed: %v", err)
	}

	affected, err := mgr.MarkOrphanedRunsInterrupted(time.Hour)
	if err != nil {
		t.Fatalf("MarkOrphanedRunsInterrupted failed: %v", err)
	}
	if affected != 0 {
		t.Errorf("MarkOrphanedRunsInterrupted should return 0 in nostate build, got %d", affected)
	}
}

func TestNoopProjectLock(t *testing.T) {
	lock, err := NewProjectLock("/any/path")
	if err != nil {
		t.Fatalf("NewProjectLock failed: %v", err)
	}

	// All lock operations should succeed silently
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if lock.Locked() {
		t.Error("Locked() should return false in nostate build")
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
}

func TestNoopBatchedManager(t *testing.T) {
	mgr, _ := New("/any/path/state.db")
	batched := NewBatchedManager(mgr, WithFlushThreshold(10))

	run, _ := mgr.CreateRun("test", RunOptions{ProjectPath: "/test"})

	// All operations should succeed silently
	if err := batched.SetFileState(run.ID, "test/BUILD", FileState{Status: FileStatusPending}); err != nil {
		t.Fatalf("SetFileState failed: %v", err)
	}

	if batched.PendingCount() != 0 {
		t.Errorf("PendingCount should be 0 in nostate build, got %d", batched.PendingCount())
	}

	if err := batched.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if err := batched.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
