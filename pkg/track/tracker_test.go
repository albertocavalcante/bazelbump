// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package track

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewTracker_GitRepo tests that NewTracker selects git strategy for git repos.
func TestNewTracker_GitRepo(t *testing.T) {
	tmpDir := createGitRepo(t)

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	if tracker.Strategy() != "git" {
		t.Errorf("expected git strategy, got %s", tracker.Strategy())
	}
}

// TestNewTracker_NonGitDir tests that NewTracker selects file strategy for non-git dirs.
func TestNewTracker_NonGitDir(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	if tracker.Strategy() != "file" {
		t.Errorf("expected file strategy, got %s", tracker.Strategy())
	}
}

// TestNewTracker_ForceFileStrategy tests that ForceFileStrategy overrides git detection.
func TestNewTracker_ForceFileStrategy(t *testing.T) {
	tmpDir := createGitRepo(t)

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath:       tmpDir,
		ForceFileStrategy: true,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	if tracker.Strategy() != "file" {
		t.Errorf("expected file strategy when forced, got %s", tracker.Strategy())
	}
}

// TestGitTracker_BeforeRun tests git strategy BeforeRun behavior.
func TestGitTracker_BeforeRun(t *testing.T) {
	tmpDir := createGitRepo(t)

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}
}

// TestGitTracker_DirtyWarning tests that dirty working tree is detected.
func TestGitTracker_DirtyWarning(t *testing.T) {
	tmpDir := createGitRepo(t)

	// Make the tree dirty
	if err := os.WriteFile(filepath.Join(tmpDir, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	var warnings []string
	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
		Logger: func(format string, args ...any) {
			warnings = append(warnings, format)
		},
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	// Should have warned about dirty tree
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "uncommitted changes") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning about uncommitted changes")
	}
}

// TestGitTracker_TrackAndUndo tests tracking files and undoing changes.
func TestGitTracker_TrackAndUndo(t *testing.T) {
	tmpDir := createGitRepo(t)

	// Create and commit a file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "test.txt")
	gitCommit(t, tmpDir, "initial")

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	// Track and modify the file
	if err := tracker.TrackFile(testFile); err != nil {
		t.Fatalf("TrackFile failed: %v", err)
	}

	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := tracker.AfterRun(ctx, true); err != nil {
		t.Fatalf("AfterRun failed: %v", err)
	}

	// Verify file was modified
	content, _ := os.ReadFile(testFile)
	if string(content) != "modified content" {
		t.Errorf("expected modified content, got %q", string(content))
	}

	// Undo the change
	if err := tracker.Undo(""); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify file was restored
	content, _ = os.ReadFile(testFile)
	if string(content) != "original content" {
		t.Errorf("expected original content after undo, got %q", string(content))
	}
}

// TestFileTracker_BeforeRun tests file strategy BeforeRun behavior.
func TestFileTracker_BeforeRun(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	// Should have created snapshot directory
	snapshotsDir := SnapshotsDir(tmpDir)
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		t.Fatalf("failed to read snapshots dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 snapshot directory, got %d", len(entries))
	}
}

// TestFileTracker_CopyOnWrite tests that files are backed up on first access.
func TestFileTracker_CopyOnWrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file to track
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	// Track the file
	if err := tracker.TrackFile(testFile); err != nil {
		t.Fatalf("TrackFile failed: %v", err)
	}

	// Complete the run to write metadata
	if err := tracker.AfterRun(ctx, true); err != nil {
		t.Fatalf("AfterRun failed: %v", err)
	}

	// Find the snapshot directory
	snapshots, err := tracker.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	// Verify backup exists
	backupPath := filepath.Join(snapshots[0].Path, "test.txt")
	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}
	if string(content) != "original content" {
		t.Errorf("backup content mismatch: got %q", string(content))
	}
}

// TestFileTracker_TrackAndUndo tests tracking files and undoing changes.
func TestFileTracker_TrackAndUndo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file to track
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	// Track and modify the file
	if err := tracker.TrackFile(testFile); err != nil {
		t.Fatalf("TrackFile failed: %v", err)
	}

	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := tracker.AfterRun(ctx, true); err != nil {
		t.Fatalf("AfterRun failed: %v", err)
	}

	// Verify file was modified
	content, _ := os.ReadFile(testFile)
	if string(content) != "modified content" {
		t.Errorf("expected modified content, got %q", string(content))
	}

	// Undo the change
	if err := tracker.Undo(""); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify file was restored
	content, _ = os.ReadFile(testFile)
	if string(content) != "original content" {
		t.Errorf("expected original content after undo, got %q", string(content))
	}
}

// TestFileTracker_UndoNewFile tests undoing creation of a new file.
func TestFileTracker_UndoNewFile(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	// Track a file that doesn't exist yet
	newFile := filepath.Join(tmpDir, "new.txt")
	if err := tracker.TrackFile(newFile); err != nil {
		t.Fatalf("TrackFile failed: %v", err)
	}

	// Create the new file
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := tracker.AfterRun(ctx, true); err != nil {
		t.Fatalf("AfterRun failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("new file should exist")
	}

	// Undo should delete the new file
	if err := tracker.Undo(""); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Error("new file should have been deleted by undo")
	}
}

// TestFileTracker_SnapshotCleanup tests that old snapshots are cleaned up.
func TestFileTracker_SnapshotCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 4 runs to exceed max of 2 snapshots (faster test)
	for i := 0; i < 4; i++ {
		tracker, err := NewTracker(TrackerOptions{
			ProjectPath:  tmpDir,
			MaxSnapshots: 2, // Use small max for faster test
		})
		if err != nil {
			t.Fatalf("NewTracker failed: %v", err)
		}

		ctx := context.Background()
		if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
			t.Fatalf("BeforeRun failed: %v", err)
		}

		// Track a file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := tracker.TrackFile(testFile); err != nil {
			t.Fatalf("TrackFile failed: %v", err)
		}

		if err := tracker.AfterRun(ctx, true); err != nil {
			t.Fatalf("AfterRun failed: %v", err)
		}

		// Sleep 1 second to ensure unique snapshot IDs (timestamp-based)
		time.Sleep(time.Second)
	}

	// Check that only 2 snapshots remain
	tracker, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir})
	snapshots, err := tracker.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots after cleanup, got %d", len(snapshots))
	}
}

// TestFileTracker_ListSnapshots tests listing available snapshots.
func TestFileTracker_ListSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 2 runs
	for i := 0; i < 2; i++ {
		tracker, err := NewTracker(TrackerOptions{
			ProjectPath:  tmpDir,
			MaxSnapshots: 10, // High max to prevent cleanup
		})
		if err != nil {
			t.Fatalf("NewTracker failed: %v", err)
		}

		ctx := context.Background()
		if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
			t.Fatalf("BeforeRun failed: %v", err)
		}

		// Track a file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := tracker.TrackFile(testFile); err != nil {
			t.Fatalf("TrackFile failed: %v", err)
		}

		if err := tracker.AfterRun(ctx, true); err != nil {
			t.Fatalf("AfterRun failed: %v", err)
		}

		// Sleep to ensure unique snapshot IDs
		time.Sleep(time.Second)
	}

	// List snapshots
	tracker, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir})
	snapshots, err := tracker.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Verify they're sorted newest first
	if len(snapshots) >= 2 {
		if snapshots[0].Timestamp.Before(snapshots[1].Timestamp) {
			t.Error("snapshots should be sorted newest first")
		}
	}
}

// TestFileTracker_UndoSpecificSnapshot tests undoing a specific snapshot.
func TestFileTracker_UndoSpecificSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")

	// First run: create with "v1", then modify to "v2"
	if err := os.WriteFile(testFile, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	tracker1, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir, MaxSnapshots: 10})
	ctx := context.Background()
	tracker1.BeforeRun(ctx, "run1")
	tracker1.TrackFile(testFile)
	if err := os.WriteFile(testFile, []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	tracker1.AfterRun(ctx, true)

	// Get first snapshot ID
	snapshots, _ := tracker1.ListSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	firstSnapshotID := snapshots[0].ID

	// Wait for different timestamp
	time.Sleep(time.Second)

	// Second run: modify from "v2" to "v3"
	tracker2, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir, MaxSnapshots: 10})
	tracker2.BeforeRun(ctx, "run2")
	tracker2.TrackFile(testFile)
	if err := os.WriteFile(testFile, []byte("v3"), 0644); err != nil {
		t.Fatal(err)
	}
	tracker2.AfterRun(ctx, true)

	// Undo specifically the first snapshot
	tracker3, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir})
	if err := tracker3.Undo(firstSnapshotID); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Should restore to "v1" (the first snapshot's backup)
	content, _ := os.ReadFile(testFile)
	if string(content) != "v1" {
		t.Errorf("expected v1 after undoing first snapshot, got %q", string(content))
	}
}

// TestGetLastRun tests reading last run info.
func TestGetLastRun(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := tracker.TrackFile(testFile); err != nil {
		t.Fatalf("TrackFile failed: %v", err)
	}

	if err := tracker.AfterRun(ctx, true); err != nil {
		t.Fatalf("AfterRun failed: %v", err)
	}

	// Get last run
	info, err := tracker.GetLastRun()
	if err != nil {
		t.Fatalf("GetLastRun failed: %v", err)
	}

	if info.Recipe != "test-recipe" {
		t.Errorf("expected recipe 'test-recipe', got %q", info.Recipe)
	}
	if info.Strategy != "file" {
		t.Errorf("expected strategy 'file', got %q", info.Strategy)
	}
	if !info.Success {
		t.Error("expected success to be true")
	}
	if len(info.ModifiedFiles) != 1 {
		t.Errorf("expected 1 modified file, got %d", len(info.ModifiedFiles))
	}
}

// TestDryRun tests that dry run doesn't create files.
func TestDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(TrackerOptions{
		ProjectPath: tmpDir,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	ctx := context.Background()
	if err := tracker.BeforeRun(ctx, "test-recipe"); err != nil {
		t.Fatalf("BeforeRun failed: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := tracker.TrackFile(testFile); err != nil {
		t.Fatalf("TrackFile failed: %v", err)
	}

	if err := tracker.AfterRun(ctx, true); err != nil {
		t.Fatalf("AfterRun failed: %v", err)
	}

	// No last-run.json should exist
	if _, err := os.Stat(LastRunPath(tmpDir)); !os.IsNotExist(err) {
		t.Error("last-run.json should not exist in dry run mode")
	}
}

// TestGitTracker_HeadMovedError tests that undo fails if HEAD moved.
func TestGitTracker_HeadMovedError(t *testing.T) {
	tmpDir := createGitRepo(t)

	// Create and commit a file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "test.txt")
	gitCommit(t, tmpDir, "initial")

	tracker, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir})

	ctx := context.Background()
	tracker.BeforeRun(ctx, "test-recipe")
	tracker.TrackFile(testFile)
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	tracker.AfterRun(ctx, true)

	// Now make a new commit (HEAD moves)
	if err := os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("other"), 0644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "other.txt")
	gitCommit(t, tmpDir, "another commit")

	// Undo should fail
	err := tracker.Undo("")
	if err == nil {
		t.Error("expected error when HEAD has moved")
	}
	if !strings.Contains(err.Error(), "HEAD has moved") {
		t.Errorf("expected 'HEAD has moved' error, got: %v", err)
	}
}

// TestDeleteSnapshot tests deleting a specific snapshot.
func TestDeleteSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a snapshot
	tracker, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir})
	ctx := context.Background()
	tracker.BeforeRun(ctx, "test-recipe")

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	tracker.TrackFile(testFile)
	tracker.AfterRun(ctx, true)

	// Get snapshot ID
	snapshots, _ := tracker.ListSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	snapshotID := snapshots[0].ID

	// Delete it
	if err := DeleteSnapshot(tmpDir, snapshotID); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Should be gone
	snapshots, _ = tracker.ListSnapshots()
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots after delete, got %d", len(snapshots))
	}
}

// TestDeleteSnapshot_InvalidID tests that invalid snapshot IDs are rejected.
func TestDeleteSnapshot_InvalidID(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to delete with path traversal
	err := DeleteSnapshot(tmpDir, "../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal attempt")
	}

	err = DeleteSnapshot(tmpDir, "foo/bar")
	if err == nil {
		t.Error("expected error for path with slash")
	}
}

// TestSuggestGitIgnore tests the .gitignore suggestion logic.
func TestSuggestGitIgnore(t *testing.T) {
	tmpDir := t.TempDir()

	// No .gitignore - should suggest
	suggestion := SuggestGitIgnore(tmpDir)
	if suggestion == "" {
		t.Error("expected suggestion when no .gitignore exists")
	}

	// Create .gitignore without entry - should suggest
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	suggestion = SuggestGitIgnore(tmpDir)
	if suggestion == "" {
		t.Error("expected suggestion when .gitignore doesn't have entry")
	}

	// Add the entry - should not suggest
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("*.log\n.bazelbump/snapshots/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	suggestion = SuggestGitIgnore(tmpDir)
	if suggestion != "" {
		t.Errorf("expected no suggestion when .gitignore has entry, got: %s", suggestion)
	}
}

// TestMultipleFilesTracked tests tracking multiple files in one run.
func TestMultipleFilesTracked(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "subdir", "file2.txt")
	os.MkdirAll(filepath.Dir(file2), 0755)

	if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}

	tracker, _ := NewTracker(TrackerOptions{ProjectPath: tmpDir})
	ctx := context.Background()
	tracker.BeforeRun(ctx, "test-recipe")

	tracker.TrackFile(file1)
	tracker.TrackFile(file2)

	// Modify both
	os.WriteFile(file1, []byte("modified1"), 0644)
	os.WriteFile(file2, []byte("modified2"), 0644)

	tracker.AfterRun(ctx, true)

	// Verify last run tracked both files
	info, _ := tracker.GetLastRun()
	if len(info.ModifiedFiles) != 2 {
		t.Errorf("expected 2 modified files, got %d", len(info.ModifiedFiles))
	}

	// Undo should restore both
	tracker.Undo("")

	c1, _ := os.ReadFile(file1)
	c2, _ := os.ReadFile(file2)

	if string(c1) != "content1" {
		t.Errorf("file1 not restored, got %q", string(c1))
	}
	if string(c2) != "content2" {
		t.Errorf("file2 not restored, got %q", string(c2))
	}
}

// Helper functions

func createGitRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	cmd.Run()

	// Initial commit (needed for some operations)
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitkeep"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, ".gitkeep")
	gitCommit(t, tmpDir, "initial")

	return tmpDir
}

func gitAdd(t *testing.T, dir, file string) {
	t.Helper()
	cmd := exec.Command("git", "add", file)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
}

func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
}
