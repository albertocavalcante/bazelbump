// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package track provides undo/snapshot functionality for bazelbump operations.
//
// The tracker system supports two strategies:
//   - Git strategy: For git repositories, tracks modified files and uses
//     git checkout to undo changes.
//   - File strategy: For non-git directories, uses copy-on-write snapshots
//     to enable undo functionality.
//
// The tracker writes state to .bazelbump/last-run.json and snapshots to
// .bazelbump/snapshots/{timestamp}/.
package track

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Tracker provides undo/snapshot functionality for recipe execution.
// It tracks which files are modified during recipe execution and enables
// undoing those changes.
type Tracker interface {
	// BeforeRun is called before recipe execution starts.
	// It checks git status (if applicable) and warns if the working tree is dirty.
	BeforeRun(ctx context.Context, recipeName string) error

	// TrackFile is called when a file is about to be modified.
	// For git strategy: records the file path
	// For file strategy: creates a backup copy if not already backed up
	TrackFile(path string) error

	// AfterRun is called after recipe execution completes.
	// It writes the tracking info to .bazelbump/last-run.json.
	AfterRun(ctx context.Context, success bool) error

	// GetLastRun returns information about the last recipe run.
	GetLastRun() (*RunInfo, error)

	// Undo reverts changes from the last run or a specific snapshot.
	// For git strategy: runs git checkout on tracked files
	// For file strategy: restores files from snapshot
	Undo(snapshotID string) error

	// ListSnapshots returns available snapshots (file strategy only).
	ListSnapshots() ([]SnapshotInfo, error)

	// Strategy returns the tracking strategy name ("git" or "file").
	Strategy() string
}

// RunInfo contains information about a recipe execution.
type RunInfo struct {
	// Recipe is the name of the recipe that was run.
	Recipe string `json:"recipe"`

	// Timestamp is when the run started.
	Timestamp time.Time `json:"timestamp"`

	// ModifiedFiles is the list of files that were modified.
	ModifiedFiles []string `json:"modified_files"`

	// Strategy is the tracking strategy used ("git" or "file").
	Strategy string `json:"strategy"`

	// Success indicates whether the recipe completed successfully.
	Success bool `json:"success"`

	// SnapshotID is the snapshot identifier (for file strategy).
	SnapshotID string `json:"snapshot_id,omitempty"`

	// GitCommit is the HEAD commit at the start of the run (for git strategy).
	GitCommit string `json:"git_commit,omitempty"`

	// GitDirty indicates if the working tree was dirty at the start.
	GitDirty bool `json:"git_dirty,omitempty"`
}

// SnapshotInfo contains information about a file strategy snapshot.
type SnapshotInfo struct {
	// ID is the snapshot identifier (timestamp-based).
	ID string `json:"id"`

	// Timestamp is when the snapshot was created.
	Timestamp time.Time `json:"timestamp"`

	// Recipe is the recipe name that created this snapshot.
	Recipe string `json:"recipe"`

	// FileCount is the number of files in the snapshot.
	FileCount int `json:"file_count"`

	// Path is the full path to the snapshot directory.
	Path string `json:"path"`
}

// TrackerOptions configures tracker behavior.
type TrackerOptions struct {
	// ProjectPath is the root directory of the project.
	ProjectPath string

	// ForceFileStrategy forces use of file strategy even in git repos.
	ForceFileStrategy bool

	// DryRun if true, doesn't create any files or modify state.
	DryRun bool

	// MaxSnapshots is the maximum number of snapshots to keep (default: 5).
	MaxSnapshots int

	// Verbose enables verbose logging.
	Verbose bool

	// Logger is the logger function (optional).
	Logger func(format string, args ...any)
}

// NewTracker creates a new tracker with auto-detected strategy.
// If the project is a git repository, it uses the git strategy.
// Otherwise, it falls back to the file strategy.
func NewTracker(opts TrackerOptions) (Tracker, error) {
	if opts.ProjectPath == "" {
		opts.ProjectPath = "."
	}

	absPath, err := filepath.Abs(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	opts.ProjectPath = absPath

	if opts.MaxSnapshots <= 0 {
		opts.MaxSnapshots = 5
	}

	if opts.Logger == nil {
		opts.Logger = func(format string, args ...any) {} // no-op
	}

	// Check if we should use git strategy
	if !opts.ForceFileStrategy && isGitRepo(opts.ProjectPath) {
		return newGitTracker(opts)
	}

	return newFileTracker(opts)
}

// BazelbumpDir returns the path to the .bazelbump directory.
func BazelbumpDir(projectPath string) string {
	return filepath.Join(projectPath, ".bazelbump")
}

// SnapshotsDir returns the path to the snapshots directory.
func SnapshotsDir(projectPath string) string {
	return filepath.Join(BazelbumpDir(projectPath), "snapshots")
}

// LastRunPath returns the path to the last-run.json file.
func LastRunPath(projectPath string) string {
	return filepath.Join(BazelbumpDir(projectPath), "last-run.json")
}

// EnsureBazelbumpDir creates the .bazelbump directory if it doesn't exist.
func EnsureBazelbumpDir(projectPath string) error {
	dir := BazelbumpDir(projectPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create .bazelbump directory: %w", err)
	}
	return nil
}

// WriteLastRun writes the run info to .bazelbump/last-run.json.
func WriteLastRun(projectPath string, info *RunInfo) error {
	if err := EnsureBazelbumpDir(projectPath); err != nil {
		return err
	}

	path := LastRunPath(projectPath)
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal run info: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write last-run.json: %w", err)
	}

	return nil
}

// ReadLastRun reads the run info from .bazelbump/last-run.json.
func ReadLastRun(projectPath string) (*RunInfo, error) {
	path := LastRunPath(projectPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no previous run found (no .bazelbump/last-run.json)")
		}
		return nil, fmt.Errorf("failed to read last-run.json: %w", err)
	}

	var info RunInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse last-run.json: %w", err)
	}

	return &info, nil
}

// isGitRepo checks if the given directory is inside a git repository.
func isGitRepo(path string) bool {
	_, err := runGitCommand(path, "rev-parse", "--git-dir")
	return err == nil
}

// GitIgnoreEntry is the line to add to .gitignore for bazelbump.
const GitIgnoreEntry = "# bazelbump snapshots\n.bazelbump/snapshots/\n"

// SuggestGitIgnore prints a suggestion to add .bazelbump/snapshots to .gitignore.
func SuggestGitIgnore(projectPath string) string {
	gitignorePath := filepath.Join(projectPath, ".gitignore")

	// Check if .gitignore exists and already has the entry
	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		if containsLine(string(content), ".bazelbump/snapshots/") ||
			containsLine(string(content), ".bazelbump/") ||
			containsLine(string(content), ".bazelbump") {
			return "" // Already ignored
		}
	}

	return fmt.Sprintf("Consider adding to .gitignore:\n%s", GitIgnoreEntry)
}

// containsLine checks if the content contains a line with the given text.
func containsLine(content, line string) bool {
	for _, l := range splitLines(content) {
		if l == line || l == line+"/" {
			return true
		}
	}
	return false
}

// splitLines splits content into lines, handling different line endings.
func splitLines(content string) []string {
	var lines []string
	var current []byte
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, string(current))
			current = nil
		} else if content[i] == '\r' {
			// Skip \r, will be followed by \n
			continue
		} else {
			current = append(current, content[i])
		}
	}
	if len(current) > 0 {
		lines = append(lines, string(current))
	}
	return lines
}
