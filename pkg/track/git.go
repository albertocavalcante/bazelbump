// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package track

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gitTracker implements the Tracker interface using git for tracking.
type gitTracker struct {
	opts          TrackerOptions
	recipeName    string
	startTime     time.Time
	modifiedFiles map[string]bool
	headCommit    string
	isDirty       bool
}

// newGitTracker creates a new git-based tracker.
func newGitTracker(opts TrackerOptions) (*gitTracker, error) {
	return &gitTracker{
		opts:          opts,
		modifiedFiles: make(map[string]bool),
	}, nil
}

// Strategy returns "git".
func (t *gitTracker) Strategy() string {
	return "git"
}

// BeforeRun checks git status and warns if dirty.
func (t *gitTracker) BeforeRun(ctx context.Context, recipeName string) error {
	t.recipeName = recipeName
	t.startTime = time.Now()
	t.modifiedFiles = make(map[string]bool)

	// Get HEAD commit
	commit, err := runGitCommand(t.opts.ProjectPath, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get HEAD commit: %w", err)
	}
	t.headCommit = strings.TrimSpace(commit)

	// Check if working tree is dirty
	status, err := runGitCommand(t.opts.ProjectPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	t.isDirty = len(strings.TrimSpace(status)) > 0

	if t.isDirty {
		t.opts.Logger("Warning: git working tree has uncommitted changes. "+
			"Undo may not work correctly if you have local modifications to files "+
			"that will be modified by this recipe.")
	}

	return nil
}

// TrackFile records a file that will be modified.
func (t *gitTracker) TrackFile(path string) error {
	// Normalize path to be relative to project root
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", path, err)
	}

	relPath, err := filepath.Rel(t.opts.ProjectPath, absPath)
	if err != nil {
		relPath = absPath
	}

	t.modifiedFiles[relPath] = true
	return nil
}

// AfterRun writes tracking info to .bazelbump/last-run.json.
func (t *gitTracker) AfterRun(ctx context.Context, success bool) error {
	if t.opts.DryRun {
		return nil
	}

	files := make([]string, 0, len(t.modifiedFiles))
	for f := range t.modifiedFiles {
		files = append(files, f)
	}

	info := &RunInfo{
		Recipe:        t.recipeName,
		Timestamp:     t.startTime,
		ModifiedFiles: files,
		Strategy:      "git",
		Success:       success,
		GitCommit:     t.headCommit,
		GitDirty:      t.isDirty,
	}

	return WriteLastRun(t.opts.ProjectPath, info)
}

// GetLastRun returns information about the last recipe run.
func (t *gitTracker) GetLastRun() (*RunInfo, error) {
	return ReadLastRun(t.opts.ProjectPath)
}

// Undo reverts changes by running git checkout on tracked files.
func (t *gitTracker) Undo(snapshotID string) error {
	// snapshotID is ignored for git strategy
	info, err := t.GetLastRun()
	if err != nil {
		return err
	}

	if info.Strategy != "git" {
		return fmt.Errorf("last run used %s strategy, not git", info.Strategy)
	}

	if len(info.ModifiedFiles) == 0 {
		return fmt.Errorf("no modified files to undo")
	}

	// Check if HEAD has moved since the run
	currentHead, err := runGitCommand(t.opts.ProjectPath, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current HEAD: %w", err)
	}
	currentHead = strings.TrimSpace(currentHead)

	if currentHead != info.GitCommit {
		return fmt.Errorf("HEAD has moved since the run (was %s, now %s). "+
			"Cannot safely undo. Consider using git revert or manual cleanup.",
			info.GitCommit[:8], currentHead[:8])
	}

	// Run git checkout on each modified file
	args := append([]string{"checkout", "--"}, info.ModifiedFiles...)
	_, err = runGitCommand(t.opts.ProjectPath, args...)
	if err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	return nil
}

// ListSnapshots returns empty slice since git strategy doesn't use snapshots.
func (t *gitTracker) ListSnapshots() ([]SnapshotInfo, error) {
	return nil, nil
}

// runGitCommand runs a git command and returns the output.
func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(errMsg))
	}

	return stdout.String(), nil
}

// IsGitInstalled checks if git is available.
func IsGitInstalled() bool {
	cmd := exec.Command("git", "--version")
	return cmd.Run() == nil
}

// GetGitStatus returns the current git status for display.
func GetGitStatus(projectPath string) (clean bool, branch string, err error) {
	// Get current branch
	branchOut, err := runGitCommand(projectPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return false, "", err
	}
	branch = strings.TrimSpace(branchOut)

	// Check if clean
	status, err := runGitCommand(projectPath, "status", "--porcelain")
	if err != nil {
		return false, branch, err
	}
	clean = len(strings.TrimSpace(status)) == 0

	return clean, branch, nil
}
