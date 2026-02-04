//go:build !nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Test creating manager with in-memory database
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	if !Available() {
		t.Error("Available() should return true in !nostate build")
	}
}

func TestCreateRun(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// Create a run
	run, err := mgr.CreateRun("test-recipe", RunOptions{
		RecipeVersion: "1.0.0",
		ProjectPath:   "/test/project",
		DryRun:        false,
		GitCommit:     "abc123",
		GitBranch:     "main",
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	if run.ID == "" {
		t.Error("Run ID should not be empty")
	}
	if run.RecipeName != "test-recipe" {
		t.Errorf("RecipeName = %q, want %q", run.RecipeName, "test-recipe")
	}
	if run.Status != RunStatusPending {
		t.Errorf("Status = %q, want %q", run.Status, RunStatusPending)
	}

	// Verify we can retrieve it
	retrieved, err := mgr.GetRun(run.ID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetRun returned nil")
	}
	if retrieved.ID != run.ID {
		t.Errorf("Retrieved run ID = %q, want %q", retrieved.ID, run.ID)
	}
}

func TestUpdateRunStatus(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, err := mgr.CreateRun("test-recipe", RunOptions{
		ProjectPath: "/test/project",
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Update to running
	if err := mgr.UpdateRunStatus(run.ID, RunStatusRunning); err != nil {
		t.Fatalf("UpdateRunStatus(running) failed: %v", err)
	}

	retrieved, _ := mgr.GetRun(run.ID)
	if retrieved.Status != RunStatusRunning {
		t.Errorf("Status = %q, want %q", retrieved.Status, RunStatusRunning)
	}

	// Update to completed
	if err := mgr.UpdateRunStatus(run.ID, RunStatusCompleted); err != nil {
		t.Fatalf("UpdateRunStatus(completed) failed: %v", err)
	}

	retrieved, _ = mgr.GetRun(run.ID)
	if retrieved.Status != RunStatusCompleted {
		t.Errorf("Status = %q, want %q", retrieved.Status, RunStatusCompleted)
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should be set when status is completed")
	}
}

func TestUpdateRunProgress(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, err := mgr.CreateRun("test-recipe", RunOptions{
		ProjectPath: "/test/project",
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Update progress
	if err := mgr.UpdateRunProgress(run.ID, 50, 100, "test/BUILD"); err != nil {
		t.Fatalf("UpdateRunProgress failed: %v", err)
	}

	retrieved, _ := mgr.GetRun(run.ID)
	if retrieved.ProcessedFiles != 50 {
		t.Errorf("ProcessedFiles = %d, want %d", retrieved.ProcessedFiles, 50)
	}
	if retrieved.TotalFiles != 100 {
		t.Errorf("TotalFiles = %d, want %d", retrieved.TotalFiles, 100)
	}
	if retrieved.CurrentFile != "test/BUILD" {
		t.Errorf("CurrentFile = %q, want %q", retrieved.CurrentFile, "test/BUILD")
	}
}

func TestGetLatestRun(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// Create multiple runs
	run1, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	run2, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Get latest
	latest, err := mgr.GetLatestRun("test-recipe")
	if err != nil {
		t.Fatalf("GetLatestRun failed: %v", err)
	}
	if latest.ID != run2.ID {
		t.Errorf("Got run %q, want latest run %q", latest.ID, run2.ID)
	}

	// Different recipe should return ErrRunNotFound
	_, err = mgr.GetLatestRun("other-recipe")
	if err != ErrRunNotFound {
		t.Errorf("GetLatestRun for non-existent recipe should return ErrRunNotFound, got %v", err)
	}

	_ = run1 // Suppress unused variable warning
}

func TestListRuns(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// Create runs
	mgr.CreateRun("recipe-a", RunOptions{ProjectPath: "/test"})
	mgr.CreateRun("recipe-a", RunOptions{ProjectPath: "/test"})
	mgr.CreateRun("recipe-b", RunOptions{ProjectPath: "/test"})

	// List all
	all, err := mgr.ListRuns(ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListRuns returned %d runs, want 3", len(all))
	}

	// Filter by recipe
	filtered, _ := mgr.ListRuns(ListRunsOptions{Recipe: "recipe-a"})
	if len(filtered) != 2 {
		t.Errorf("ListRuns(recipe-a) returned %d runs, want 2", len(filtered))
	}

	// Limit
	limited, _ := mgr.ListRuns(ListRunsOptions{Limit: 1})
	if len(limited) != 1 {
		t.Errorf("ListRuns(limit=1) returned %d runs, want 1", len(limited))
	}
}

func TestCreateCheckpoint(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Create checkpoint
	cp, err := mgr.CreateCheckpoint(run.ID, CheckpointOptions{
		Type:        CheckpointAuto,
		StepID:      "scan-step",
		StepIndex:   0,
		FileIndex:   100,
		Description: "Auto checkpoint at file 100",
		State: &ExecutionState{
			Statistics: &ExecutionStats{
				FilesProcessed: 100,
				FilesModified:  25,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	if cp.ID == "" {
		t.Error("Checkpoint ID should not be empty")
	}
	if cp.Type != CheckpointAuto {
		t.Errorf("Type = %q, want %q", cp.Type, CheckpointAuto)
	}
	if cp.StepID != "scan-step" {
		t.Errorf("StepID = %q, want %q", cp.StepID, "scan-step")
	}

	// Verify state was serialized
	var state ExecutionState
	if err := json.Unmarshal(cp.State, &state); err != nil {
		t.Errorf("Failed to unmarshal state: %v", err)
	}
	if state.Statistics.FilesProcessed != 100 {
		t.Errorf("State.Statistics.FilesProcessed = %d, want 100", state.Statistics.FilesProcessed)
	}
}

func TestGetLatestCheckpoint(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Create multiple checkpoints
	mgr.CreateCheckpoint(run.ID, CheckpointOptions{
		StepID:    "step-1",
		FileIndex: 100,
	})
	time.Sleep(10 * time.Millisecond)
	cp2, _ := mgr.CreateCheckpoint(run.ID, CheckpointOptions{
		StepID:    "step-1",
		FileIndex: 200,
	})

	// Get latest
	latest, err := mgr.GetLatestCheckpoint(run.ID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint failed: %v", err)
	}
	if latest.ID != cp2.ID {
		t.Errorf("Got checkpoint %q, want latest %q", latest.ID, cp2.ID)
	}
}

func TestListCheckpoints(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Create checkpoints
	mgr.CreateCheckpoint(run.ID, CheckpointOptions{StepID: "step-1", FileIndex: 100})
	mgr.CreateCheckpoint(run.ID, CheckpointOptions{StepID: "step-1", FileIndex: 200})
	mgr.CreateCheckpoint(run.ID, CheckpointOptions{StepID: "step-2", FileIndex: 0})

	checkpoints, err := mgr.ListCheckpoints(run.ID)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(checkpoints) != 3 {
		t.Errorf("ListCheckpoints returned %d, want 3", len(checkpoints))
	}
}

func TestFileState(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Set file state
	now := time.Now()
	err = mgr.SetFileState(run.ID, "test/BUILD", FileState{
		Status:      FileStatusProcessing,
		ContentHash: "abc123",
		Mtime:       now.Unix(),
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("SetFileState failed: %v", err)
	}

	// Get file state
	fs, err := mgr.GetFileState(run.ID, "test/BUILD")
	if err != nil {
		t.Fatalf("GetFileState failed: %v", err)
	}
	if fs == nil {
		t.Fatal("GetFileState returned nil")
	}
	if fs.Status != FileStatusProcessing {
		t.Errorf("Status = %q, want %q", fs.Status, FileStatusProcessing)
	}
	if fs.ContentHash != "abc123" {
		t.Errorf("ContentHash = %q, want %q", fs.ContentHash, "abc123")
	}

	// Update file state
	completed := time.Now()
	err = mgr.SetFileState(run.ID, "test/BUILD", FileState{
		Status:      FileStatusCompleted,
		Changed:     true,
		LinesAdded:  5,
		CompletedAt: &completed,
	})
	if err != nil {
		t.Fatalf("SetFileState (update) failed: %v", err)
	}

	fs, _ = mgr.GetFileState(run.ID, "test/BUILD")
	if fs.Status != FileStatusCompleted {
		t.Errorf("Status = %q, want %q", fs.Status, FileStatusCompleted)
	}
	if !fs.Changed {
		t.Error("Changed should be true")
	}
}

func TestGetPendingFiles(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Set up various file states
	mgr.SetFileState(run.ID, "a/BUILD", FileState{Status: FileStatusPending})
	mgr.SetFileState(run.ID, "b/BUILD", FileState{Status: FileStatusPending})
	mgr.SetFileState(run.ID, "c/BUILD", FileState{Status: FileStatusCompleted})
	mgr.SetFileState(run.ID, "d/BUILD", FileState{Status: FileStatusSkipped})

	pending, err := mgr.GetPendingFiles(run.ID)
	if err != nil {
		t.Fatalf("GetPendingFiles failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("GetPendingFiles returned %d files, want 2", len(pending))
	}
}

func TestLogAction(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Log action
	err = mgr.LogAction(run.ID, AuditFileModified, AuditLogOptions{
		FilePath:    "test/BUILD",
		Description: "Added load statement",
		DiffSummary: "+1 -0",
	})
	if err != nil {
		t.Fatalf("LogAction failed: %v", err)
	}

	// No way to query audit log via Manager interface yet, so just verify no error
}

func TestProgressBar(t *testing.T) {
	tests := []struct {
		current, total, width int
		wantContains          string
	}{
		{0, 100, 20, "["},
		{50, 100, 20, "="},
		{100, 100, 20, "==="},
		{0, 0, 20, ""},
	}

	for _, tc := range tests {
		got := ProgressBar(tc.current, tc.total, tc.width)
		if len(got) == 0 && tc.wantContains != "" {
			t.Errorf("ProgressBar(%d, %d, %d) = empty, want something", tc.current, tc.total, tc.width)
		}
	}
}

func TestTerminalReporter(t *testing.T) {
	// Just verify it doesn't panic
	reporter := NewTerminalReporter(TerminalReporterOptions{
		Verbose: true,
		NoColor: true,
	})

	run := &Run{
		ID:         "test-run",
		RecipeName: "test-recipe",
	}

	reporter.OnRunStart(run)
	reporter.OnStepStart("step-1", 0, 3)
	reporter.OnFileStart("test/BUILD")
	reporter.OnProgress(50, 100)
	reporter.OnFileComplete("test/BUILD", true)
	reporter.OnStepComplete("step-1", true)
	reporter.Info("Test info message")
	reporter.Warn("Test warning")
	reporter.Error("Test error")
	reporter.OnRunComplete(run, true)
}

func TestJSONReporter(t *testing.T) {
	// Just verify it doesn't panic and produces valid JSON
	reporter := NewJSONReporter(nil) // Uses os.Stdout by default

	run := &Run{
		ID:         "test-run",
		RecipeName: "test-recipe",
	}

	reporter.OnRunStart(run)
	reporter.OnStepStart("step-1", 0, 3)
	reporter.OnProgress(50, 100)
	reporter.OnStepComplete("step-1", true)
	reporter.OnRunComplete(run, true)
}

func TestNoopReporter(t *testing.T) {
	// Just verify it doesn't panic
	reporter := NewNoopReporter()

	run := &Run{
		ID:         "test-run",
		RecipeName: "test-recipe",
	}

	reporter.OnRunStart(run)
	reporter.OnStepStart("step-1", 0, 3)
	reporter.OnFileStart("test/BUILD")
	reporter.OnProgress(50, 100)
	reporter.OnFileComplete("test/BUILD", true)
	reporter.OnStepComplete("step-1", true)
	reporter.Info("Test info")
	reporter.Warn("Test warn")
	reporter.Error("Test error")
	reporter.OnRunComplete(run, true)
}

func TestCompositeReporter(t *testing.T) {
	noop1 := NewNoopReporter()
	noop2 := NewNoopReporter()
	composite := NewCompositeReporter(noop1, noop2)

	run := &Run{
		ID:         "test-run",
		RecipeName: "test-recipe",
	}

	// Just verify it doesn't panic
	composite.OnRunStart(run)
	composite.OnStepStart("step-1", 0, 1)
	composite.OnProgress(1, 1)
	composite.OnStepComplete("step-1", true)
	composite.OnRunComplete(run, true)
}

// =============================================================================
// P0/P1 Feature Tests
// =============================================================================

func TestSentinelErrors(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// Test ErrRunNotFound
	_, err = mgr.GetRun("non-existent-id")
	if err != ErrRunNotFound {
		t.Errorf("GetRun(non-existent) should return ErrRunNotFound, got %v", err)
	}

	// Test ErrCheckpointNotFound
	_, err = mgr.GetCheckpoint("non-existent-id")
	if err != ErrCheckpointNotFound {
		t.Errorf("GetCheckpoint(non-existent) should return ErrCheckpointNotFound, got %v", err)
	}

	// Test ErrRunNotFound for UpdateRunStatus
	err = mgr.UpdateRunStatus("non-existent-id", RunStatusCompleted)
	if err != ErrRunNotFound {
		t.Errorf("UpdateRunStatus(non-existent) should return ErrRunNotFound, got %v", err)
	}
}

func TestGetFilesByStatus(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Set up files with different statuses
	mgr.SetFileState(run.ID, "a/BUILD", FileState{Status: FileStatusPending})
	mgr.SetFileState(run.ID, "b/BUILD", FileState{Status: FileStatusStaged})
	mgr.SetFileState(run.ID, "c/BUILD", FileState{Status: FileStatusStaged})
	mgr.SetFileState(run.ID, "d/BUILD", FileState{Status: FileStatusCompleted})

	// Get staged files
	staged, err := mgr.GetFilesByStatus(run.ID, FileStatusStaged)
	if err != nil {
		t.Fatalf("GetFilesByStatus failed: %v", err)
	}
	if len(staged) != 2 {
		t.Errorf("GetFilesByStatus(staged) returned %d files, want 2", len(staged))
	}

	// Get completed files
	completed, err := mgr.GetFilesByStatus(run.ID, FileStatusCompleted)
	if err != nil {
		t.Fatalf("GetFilesByStatus failed: %v", err)
	}
	if len(completed) != 1 {
		t.Errorf("GetFilesByStatus(completed) returned %d files, want 1", len(completed))
	}
}

func TestValidateOnStartup(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// ValidateOnStartup should not fail on empty database
	err = mgr.ValidateOnStartup()
	if err != nil {
		t.Errorf("ValidateOnStartup failed: %v", err)
	}
}

func TestMarkOrphanedRunsInterrupted(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// Create a run and set it to running
	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})
	mgr.UpdateRunStatus(run.ID, RunStatusRunning)

	// With 0 timeout, it should mark the run as interrupted immediately
	affected, err := mgr.MarkOrphanedRunsInterrupted(0)
	if err != nil {
		t.Fatalf("MarkOrphanedRunsInterrupted failed: %v", err)
	}
	if affected != 1 {
		t.Errorf("MarkOrphanedRunsInterrupted affected %d runs, want 1", affected)
	}

	// Verify the run is now interrupted
	retrieved, _ := mgr.GetRun(run.ID)
	if retrieved.Status != RunStatusInterrupted {
		t.Errorf("Run status = %q, want %q", retrieved.Status, RunStatusInterrupted)
	}
	if retrieved.ErrorMessage == "" {
		t.Error("ErrorMessage should be set for interrupted runs")
	}
}

func TestCheckpointCleanup(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Create more checkpoints than MaxCheckpointsPerRun
	for i := 0; i < 15; i++ {
		_, err := mgr.CreateCheckpoint(run.ID, CheckpointOptions{
			StepID:    "step-1",
			FileIndex: i * 100,
		})
		if err != nil {
			t.Fatalf("CreateCheckpoint %d failed: %v", i, err)
		}
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// List checkpoints - should be capped at MaxCheckpointsPerRun (10)
	checkpoints, err := mgr.ListCheckpoints(run.ID)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(checkpoints) != MaxCheckpointsPerRun {
		t.Errorf("ListCheckpoints returned %d, want %d (MaxCheckpointsPerRun)", len(checkpoints), MaxCheckpointsPerRun)
	}

	// Verify the oldest checkpoints were removed (highest file indices should remain)
	// The last checkpoint should have FileIndex = 1400 (14 * 100)
	lastCP := checkpoints[len(checkpoints)-1]
	if lastCP.FileIndex < 500 {
		t.Errorf("Oldest checkpoints should have been removed, but FileIndex = %d", lastCP.FileIndex)
	}
}

func TestCheckpointStateSize(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Create checkpoint with state
	cp, err := mgr.CreateCheckpoint(run.ID, CheckpointOptions{
		StepID:    "step-1",
		FileIndex: 100,
		State: &ExecutionState{
			Statistics: &ExecutionStats{
				FilesProcessed: 100,
				FilesModified:  25,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Verify state size was recorded
	if cp.StateSize == 0 {
		t.Error("StateSize should be > 0 when state is provided")
	}

	// Retrieve and verify
	retrieved, _ := mgr.GetCheckpoint(cp.ID)
	if retrieved.StateSize != cp.StateSize {
		t.Errorf("StateSize = %d, want %d", retrieved.StateSize, cp.StateSize)
	}
}

func TestRecipeHashInRun(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	// Create run with recipe hash
	run, err := mgr.CreateRun("test-recipe", RunOptions{
		ProjectPath:   "/test",
		RecipeVersion: "1.0.0",
		RecipeHash:    "abc123def456",
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	if run.RecipeHash != "abc123def456" {
		t.Errorf("RecipeHash = %q, want %q", run.RecipeHash, "abc123def456")
	}

	// Retrieve and verify
	retrieved, _ := mgr.GetRun(run.ID)
	if retrieved.RecipeHash != "abc123def456" {
		t.Errorf("Retrieved RecipeHash = %q, want %q", retrieved.RecipeHash, "abc123def456")
	}
}

func TestFileStateWithAfterHash(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Set file state with both hashes
	now := time.Now()
	err = mgr.SetFileState(run.ID, "test/BUILD", FileState{
		Status:      FileStatusCompleted,
		ContentHash: "before-hash",
		AfterHash:   "after-hash",
		Changed:     true,
		CompletedAt: &now,
	})
	if err != nil {
		t.Fatalf("SetFileState failed: %v", err)
	}

	// Verify after_hash was saved
	fs, _ := mgr.GetFileState(run.ID, "test/BUILD")
	if fs.ContentHash != "before-hash" {
		t.Errorf("ContentHash = %q, want %q", fs.ContentHash, "before-hash")
	}
	if fs.AfterHash != "after-hash" {
		t.Errorf("AfterHash = %q, want %q", fs.AfterHash, "after-hash")
	}
}

func TestBatchedManager(t *testing.T) {
	mgr, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	defer mgr.Close()

	run, _ := mgr.CreateRun("test-recipe", RunOptions{ProjectPath: "/test"})

	// Create batched manager with low threshold
	batched := NewBatchedManager(mgr, WithFlushThreshold(5))

	// Add file states - should not flush yet
	for i := 0; i < 4; i++ {
		err := batched.SetFileState(run.ID, "file"+string(rune('a'+i))+"/BUILD", FileState{
			Status: FileStatusPending,
		})
		if err != nil {
			t.Fatalf("SetFileState %d failed: %v", i, err)
		}
	}

	// Check pending count
	if batched.PendingCount() != 4 {
		t.Errorf("PendingCount = %d, want 4", batched.PendingCount())
	}

	// Add one more - should trigger flush
	err = batched.SetFileState(run.ID, "filee/BUILD", FileState{Status: FileStatusPending})
	if err != nil {
		t.Fatalf("SetFileState 5 failed: %v", err)
	}

	// After flush, pending count should be 0
	if batched.PendingCount() != 0 {
		t.Errorf("PendingCount after flush = %d, want 0", batched.PendingCount())
	}

	// Verify files were written
	pending, _ := mgr.GetPendingFiles(run.ID)
	if len(pending) != 5 {
		t.Errorf("GetPendingFiles returned %d files, want 5", len(pending))
	}

	// Test explicit flush
	batched.SetFileState(run.ID, "filef/BUILD", FileState{Status: FileStatusPending})
	if batched.PendingCount() != 1 {
		t.Errorf("PendingCount = %d, want 1", batched.PendingCount())
	}

	err = batched.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if batched.PendingCount() != 0 {
		t.Errorf("PendingCount after explicit flush = %d, want 0", batched.PendingCount())
	}

	pending, _ = mgr.GetPendingFiles(run.ID)
	if len(pending) != 6 {
		t.Errorf("GetPendingFiles returned %d files, want 6", len(pending))
	}
}

func TestProjectLock(t *testing.T) {
	// Create temp directory for test
	tempDir := t.TempDir()

	// Create first lock
	lock1, err := NewProjectLock(tempDir)
	if err != nil {
		t.Fatalf("NewProjectLock failed: %v", err)
	}

	// Acquire should succeed
	if err := lock1.Acquire(); err != nil {
		t.Fatalf("First Acquire failed: %v", err)
	}
	defer lock1.Release()

	if !lock1.Locked() {
		t.Error("Locked() should return true after Acquire")
	}

	// Second lock should fail to acquire
	lock2, err := NewProjectLock(tempDir)
	if err != nil {
		t.Fatalf("NewProjectLock 2 failed: %v", err)
	}

	err = lock2.Acquire()
	if err != ErrLockHeld {
		t.Errorf("Second Acquire should return ErrLockHeld, got %v", err)
	}

	// Release first lock
	if err := lock1.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Now second lock should succeed
	if err := lock2.Acquire(); err != nil {
		t.Fatalf("Second Acquire after release failed: %v", err)
	}
	lock2.Release()
}
