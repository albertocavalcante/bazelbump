// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package track

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// fileTracker implements the Tracker interface using file-based snapshots.
type fileTracker struct {
	opts          TrackerOptions
	recipeName    string
	startTime     time.Time
	snapshotID    string
	snapshotDir   string
	modifiedFiles map[string]bool
	backedUp      map[string]bool // tracks which files have been backed up
}

// newFileTracker creates a new file-based tracker.
func newFileTracker(opts TrackerOptions) (*fileTracker, error) {
	return &fileTracker{
		opts:          opts,
		modifiedFiles: make(map[string]bool),
		backedUp:      make(map[string]bool),
	}, nil
}

// Strategy returns "file".
func (t *fileTracker) Strategy() string {
	return "file"
}

// BeforeRun prepares the snapshot directory.
func (t *fileTracker) BeforeRun(ctx context.Context, recipeName string) error {
	t.recipeName = recipeName
	t.startTime = time.Now()
	t.modifiedFiles = make(map[string]bool)
	t.backedUp = make(map[string]bool)

	// Generate snapshot ID based on timestamp
	t.snapshotID = t.startTime.Format("20060102-150405")
	t.snapshotDir = filepath.Join(SnapshotsDir(t.opts.ProjectPath), t.snapshotID)

	if !t.opts.DryRun {
		// Create snapshot directory
		if err := os.MkdirAll(t.snapshotDir, 0755); err != nil {
			return fmt.Errorf("failed to create snapshot directory: %w", err)
		}
	}

	return nil
}

// TrackFile backs up a file before modification (copy-on-write).
func (t *fileTracker) TrackFile(path string) error {
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

	// Only backup once per file (copy-on-write)
	if t.backedUp[relPath] {
		return nil
	}

	if t.opts.DryRun {
		t.backedUp[relPath] = true
		return nil
	}

	// Create backup if file exists
	if _, err := os.Stat(absPath); err == nil {
		backupPath := filepath.Join(t.snapshotDir, relPath)

		// Create backup directory structure
		backupDir := filepath.Dir(backupPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return fmt.Errorf("failed to create backup directory: %w", err)
		}

		// Copy file
		if err := copyFile(absPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup %s: %w", relPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}
	// If file doesn't exist, we record it but don't backup
	// (it's a new file that will be created)

	t.backedUp[relPath] = true
	return nil
}

// AfterRun writes tracking info and cleans up old snapshots.
func (t *fileTracker) AfterRun(ctx context.Context, success bool) error {
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
		Strategy:      "file",
		Success:       success,
		SnapshotID:    t.snapshotID,
	}

	// Write snapshot metadata
	metaPath := filepath.Join(t.snapshotDir, "metadata.json")
	metaData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot metadata: %w", err)
	}

	// Write last-run.json
	if err := WriteLastRun(t.opts.ProjectPath, info); err != nil {
		return err
	}

	// Clean up old snapshots
	if err := t.cleanupOldSnapshots(); err != nil {
		t.opts.Logger("Warning: failed to cleanup old snapshots: %v", err)
	}

	return nil
}

// GetLastRun returns information about the last recipe run.
func (t *fileTracker) GetLastRun() (*RunInfo, error) {
	return ReadLastRun(t.opts.ProjectPath)
}

// Undo restores files from a snapshot.
func (t *fileTracker) Undo(snapshotID string) error {
	var info *RunInfo
	var snapshotDir string

	if snapshotID == "" {
		// Use last run
		var err error
		info, err = t.GetLastRun()
		if err != nil {
			return err
		}
		if info.Strategy != "file" {
			return fmt.Errorf("last run used %s strategy, not file", info.Strategy)
		}
		snapshotID = info.SnapshotID
	}

	snapshotDir = filepath.Join(SnapshotsDir(t.opts.ProjectPath), snapshotID)

	// Check snapshot exists
	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %s not found", snapshotID)
	}

	// Load metadata if we don't have info
	if info == nil {
		metaPath := filepath.Join(snapshotDir, "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			return fmt.Errorf("failed to read snapshot metadata: %w", err)
		}
		info = &RunInfo{}
		if err := json.Unmarshal(data, info); err != nil {
			return fmt.Errorf("failed to parse snapshot metadata: %w", err)
		}
	}

	// Restore each modified file
	for _, relPath := range info.ModifiedFiles {
		backupPath := filepath.Join(snapshotDir, relPath)
		targetPath := filepath.Join(t.opts.ProjectPath, relPath)

		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			// No backup means it was a new file - delete it
			if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove new file %s: %w", relPath, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to stat backup %s: %w", relPath, err)
		} else {
			// Restore from backup
			if err := copyFile(backupPath, targetPath); err != nil {
				return fmt.Errorf("failed to restore %s: %w", relPath, err)
			}
		}
	}

	return nil
}

// ListSnapshots returns available snapshots.
func (t *fileTracker) ListSnapshots() ([]SnapshotInfo, error) {
	snapshotsDir := SnapshotsDir(t.opts.ProjectPath)

	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	var snapshots []SnapshotInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		snapshotDir := filepath.Join(snapshotsDir, entry.Name())
		metaPath := filepath.Join(snapshotDir, "metadata.json")

		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // Skip invalid snapshots
		}

		var info RunInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		// Count files in snapshot (excluding metadata.json)
		fileCount := 0
		filepath.Walk(snapshotDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if filepath.Base(path) != "metadata.json" {
				fileCount++
			}
			return nil
		})

		snapshots = append(snapshots, SnapshotInfo{
			ID:        entry.Name(),
			Timestamp: info.Timestamp,
			Recipe:    info.Recipe,
			FileCount: fileCount,
			Path:      snapshotDir,
		})
	}

	// Sort by timestamp, newest first
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

// cleanupOldSnapshots removes snapshots beyond MaxSnapshots.
func (t *fileTracker) cleanupOldSnapshots() error {
	snapshots, err := t.ListSnapshots()
	if err != nil {
		return err
	}

	if len(snapshots) <= t.opts.MaxSnapshots {
		return nil
	}

	// Delete oldest snapshots
	for i := t.opts.MaxSnapshots; i < len(snapshots); i++ {
		if err := os.RemoveAll(snapshots[i].Path); err != nil {
			return fmt.Errorf("failed to remove old snapshot %s: %w", snapshots[i].ID, err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Ensure destination directory exists
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// FormatSnapshotID formats a timestamp as a snapshot ID.
func FormatSnapshotID(t time.Time) string {
	return t.Format("20060102-150405")
}

// ParseSnapshotID parses a snapshot ID back to a timestamp.
func ParseSnapshotID(id string) (time.Time, error) {
	return time.Parse("20060102-150405", id)
}

// DeleteSnapshot removes a specific snapshot.
func DeleteSnapshot(projectPath, snapshotID string) error {
	// Validate the snapshot ID to prevent path traversal
	if strings.Contains(snapshotID, "/") || strings.Contains(snapshotID, "..") {
		return fmt.Errorf("invalid snapshot ID")
	}

	snapshotDir := filepath.Join(SnapshotsDir(projectPath), snapshotID)

	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %s not found", snapshotID)
	}

	return os.RemoveAll(snapshotDir)
}
