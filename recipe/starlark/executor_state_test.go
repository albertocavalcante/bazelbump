// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/albertocavalcante/bazelbump/pkg/state"
)

// TestExecutorWithoutStateManager tests that execution works without a state manager.
func TestExecutorWithoutStateManager(t *testing.T) {
	// Create a temp directory with a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a simple recipe with a scan step (minimum viable recipe)
	scanStep := &ScanStep{
		FilePatterns: []string{"*.txt"},
		Patterns:     []string{"hello"},
	}
	scanStep.ID = "find-files"
	scanStep.Name = "Find files"

	recipe := &Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Steps:       []Step{scanStep},
	}

	// Create executor without state options
	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true})

	// Execute - should work without state manager
	result, err := executor.ExecuteWithState(recipe, Options{DryRun: true})
	if err != nil {
		t.Fatalf("ExecuteWithState failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}
}

// TestExecutorWithStateManager tests that execution works with a state manager.
func TestExecutorWithStateManager(t *testing.T) {
	// Skip if state management is not available (nostate build)
	if !state.Available() {
		t.Skip("State management not available in this build")
	}

	// Create temp directory for state DB and test file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".bazelbump", "state.db")

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create state manager
	mgr, err := state.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}
	defer mgr.Close()

	// Create a simple recipe with a scan step
	scanStep := &ScanStep{
		FilePatterns: []string{"*.txt"},
		Patterns:     []string{"hello"},
	}
	scanStep.ID = "find-files"
	scanStep.Name = "Find files"

	recipe := &Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe with state",
		Steps:       []Step{scanStep},
	}

	// Create executor with state manager
	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true},
		WithStateManager(mgr),
		WithCheckpointConfig(DefaultCheckpointConfig()),
	)

	// Execute
	result, err := executor.ExecuteWithState(recipe, Options{DryRun: true})
	if err != nil {
		t.Fatalf("ExecuteWithState failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}

	// Verify run was created
	runID := executor.GetRunID()
	if runID == "" {
		t.Error("Expected run ID to be set")
	}

	// Verify run exists in state
	run, err := mgr.GetRun(runID)
	if err != nil {
		t.Errorf("Failed to get run: %v", err)
	}
	if run == nil {
		t.Error("Expected run to exist in state")
	}
}

// TestCheckpointConfig tests checkpoint configuration.
func TestCheckpointConfig(t *testing.T) {
	cfg := DefaultCheckpointConfig()

	if !cfg.Enabled {
		t.Error("Expected default config to be enabled")
	}

	if cfg.AutoInterval != 100 {
		t.Errorf("Expected AutoInterval=100, got %d", cfg.AutoInterval)
	}

	if !cfg.StepBoundary {
		t.Error("Expected StepBoundary to be true")
	}

	if cfg.MinTimeBetween != 30*time.Second {
		t.Errorf("Expected MinTimeBetween=30s, got %v", cfg.MinTimeBetween)
	}

	if cfg.MaxCheckpoints != 10 {
		t.Errorf("Expected MaxCheckpoints=10, got %d", cfg.MaxCheckpoints)
	}
}

// TestComputeRecipeHash tests recipe hash computation.
func TestComputeRecipeHash(t *testing.T) {
	recipe1 := &Recipe{
		Name:    "test-recipe",
		Version: "1.0.0",
		Steps:   []Step{},
	}

	recipe2 := &Recipe{
		Name:    "test-recipe",
		Version: "1.0.0",
		Steps:   []Step{},
	}

	recipe3 := &Recipe{
		Name:    "test-recipe",
		Version: "1.0.1", // Different version
		Steps:   []Step{},
	}

	hash1 := computeRecipeHash(recipe1)
	hash2 := computeRecipeHash(recipe2)
	hash3 := computeRecipeHash(recipe3)

	// Same recipes should have same hash
	if hash1 != hash2 {
		t.Errorf("Expected same hash for identical recipes, got %s vs %s", hash1, hash2)
	}

	// Different recipes should have different hash
	if hash1 == hash3 {
		t.Error("Expected different hash for different recipe version")
	}

	// Hash should be non-empty
	if hash1 == "" {
		t.Error("Expected non-empty hash")
	}

	// Hash should be reasonable length (16 chars for our truncation)
	if len(hash1) != 16 {
		t.Errorf("Expected hash length 16, got %d", len(hash1))
	}
}

// TestNoopProgressReporter tests that NoopProgressReporter implements ProgressReporter.
func TestNoopProgressReporter(t *testing.T) {
	var reporter ProgressReporter = &NoopProgressReporter{}

	// These should all be no-ops that don't panic
	reporter.RunStart("run-123", "test-recipe")
	reporter.RunComplete("run-123", true, 1000)
	reporter.RunError("run-123", nil)
	reporter.StepStart("step-1", 0, 3)
	reporter.StepComplete("step-1", true, "done")
	reporter.FileStart("/path/to/file")
	reporter.FileChanged("/path/to/file")
	reporter.FileUnchanged("/path/to/file")
	reporter.FileSkipped("/path/to/file", "reason")
	reporter.FileError("/path/to/file", nil)
	reporter.CheckpointCreated("cp-123", 100)
	reporter.CheckpointRestored("cp-123")
	reporter.Info("test %s", "message")
	reporter.Warning("test %s", "warning")
}

// TestExecutorOptions tests executor option application.
func TestExecutorOptions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create executor with various options
	cfg := DefaultCheckpointConfig()
	cfg.AutoInterval = 50

	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true},
		WithCheckpointConfig(cfg),
		WithRunID("test-run-123"),
		WithProgressReporter(&NoopProgressReporter{}),
	)

	// Verify options were applied
	if executor.stateOpts.checkpointConfig == nil {
		t.Error("Expected checkpoint config to be set")
	}

	if executor.stateOpts.checkpointConfig.AutoInterval != 50 {
		t.Errorf("Expected AutoInterval=50, got %d", executor.stateOpts.checkpointConfig.AutoInterval)
	}

	if executor.stateOpts.runID != "test-run-123" {
		t.Errorf("Expected runID=test-run-123, got %s", executor.stateOpts.runID)
	}

	if executor.stateOpts.progressReporter == nil {
		t.Error("Expected progress reporter to be set")
	}
}

// TestWithResume tests resume option configuration.
func TestWithResume(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true},
		WithResume("run-123", "checkpoint-456"),
	)

	if executor.stateOpts.resumeFromRun != "run-123" {
		t.Errorf("Expected resumeFromRun=run-123, got %s", executor.stateOpts.resumeFromRun)
	}

	if executor.stateOpts.resumeCheckpoint != "checkpoint-456" {
		t.Errorf("Expected resumeCheckpoint=checkpoint-456, got %s", executor.stateOpts.resumeCheckpoint)
	}

	if !executor.stateOpts.skipCompleted {
		t.Error("Expected skipCompleted=true")
	}
}

// TestAvailableWithoutStateManager tests Available() without state manager.
func TestAvailableWithoutStateManager(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true})

	// Without state manager, should return false
	if executor.Available() {
		t.Error("Expected Available()=false without state manager")
	}
}

// TestAvailableWithStateManager tests Available() with state manager.
func TestAvailableWithStateManager(t *testing.T) {
	// Skip if state management is not available
	if !state.Available() {
		t.Skip("State management not available in this build")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".bazelbump", "state.db")

	mgr, err := state.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}
	defer mgr.Close()

	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true},
		WithStateManager(mgr),
	)

	// With state manager in full build, should return true
	if !executor.Available() {
		t.Error("Expected Available()=true with state manager")
	}
}

// TestShouldAutoCheckpoint tests auto-checkpoint logic.
func TestShouldAutoCheckpoint(t *testing.T) {
	// Skip if state management is not available
	if !state.Available() {
		t.Skip("State management not available in this build")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".bazelbump", "state.db")

	mgr, err := state.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}
	defer mgr.Close()

	cfg := DefaultCheckpointConfig()
	cfg.AutoInterval = 10
	cfg.MinTimeBetween = 0 // Disable time check for testing

	executor := NewExecutorWithOptions(tmpDir, Options{DryRun: true},
		WithStateManager(mgr),
		WithCheckpointConfig(cfg),
	)

	// At 0 files, should not checkpoint
	executor.stateOpts.filesProcessed = 0
	if executor.shouldAutoCheckpoint() {
		t.Error("Should not checkpoint at 0 files")
	}

	// At 10 files (interval), should checkpoint
	executor.stateOpts.filesProcessed = 10
	if !executor.shouldAutoCheckpoint() {
		t.Error("Should checkpoint at 10 files")
	}

	// At 15 files (not interval), should not checkpoint
	executor.stateOpts.filesProcessed = 15
	if executor.shouldAutoCheckpoint() {
		t.Error("Should not checkpoint at 15 files")
	}

	// At 20 files (interval), should checkpoint
	executor.stateOpts.filesProcessed = 20
	if !executor.shouldAutoCheckpoint() {
		t.Error("Should checkpoint at 20 files")
	}
}

// TestGetGitInfo tests git info extraction.
func TestGetGitInfo(t *testing.T) {
	// Create a fake git repo
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git dir: %v", err)
	}

	// Write HEAD file pointing to a branch
	headContent := "ref: refs/heads/main\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatalf("Failed to write HEAD: %v", err)
	}

	// Write the ref file
	refsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatalf("Failed to create refs dir: %v", err)
	}

	commitHash := "abc123def456789012345678901234567890abcd\n"
	if err := os.WriteFile(filepath.Join(refsDir, "main"), []byte(commitHash), 0644); err != nil {
		t.Fatalf("Failed to write ref: %v", err)
	}

	// Get git info
	commit, branch := getGitInfo(tmpDir)

	if branch != "main" {
		t.Errorf("Expected branch=main, got %s", branch)
	}

	if commit != "abc123def456789012345678901234567890abcd" {
		t.Errorf("Unexpected commit hash: %s", commit)
	}
}

// TestCountLineChanges tests line change counting.
func TestCountLineChanges(t *testing.T) {
	change := &FileChange{
		Path: "/test/file.txt",
		Hunks: []Hunk{
			{
				Lines: []LineDiff{
					{Type: "add", Content: "new line 1"},
					{Type: "add", Content: "new line 2"},
					{Type: "remove", Content: "old line 1"},
					{Type: "context", Content: "unchanged"},
				},
			},
			{
				Lines: []LineDiff{
					{Type: "add", Content: "new line 3"},
					{Type: "remove", Content: "old line 2"},
					{Type: "remove", Content: "old line 3"},
				},
			},
		},
	}

	added, removed := countLineChanges(change)

	if added != 3 {
		t.Errorf("Expected 3 lines added, got %d", added)
	}

	if removed != 3 {
		t.Errorf("Expected 3 lines removed, got %d", removed)
	}
}
