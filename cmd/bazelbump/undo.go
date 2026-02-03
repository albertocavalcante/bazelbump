// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/albertocavalcante/bazelbump/pkg/track"
)

func runUndo(args []string) error {
	fs := flag.NewFlagSet("undo", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Undo the last bazelbump run.

Reverts all file changes made by the last recipe execution.
For git repositories, uses git checkout to restore files.
For non-git directories, restores from snapshot backup.

%s
  bazelbump undo [flags]

%s
  --path       Path to project directory (default: current directory)
  --snapshot   Restore a specific snapshot (file strategy only)
  --dry-run    Show what would be undone without making changes
  --force      Force undo even if warnings would normally prevent it
  --help       Show this help

%s
  # Undo the last run
  bazelbump undo

  # Preview what would be undone
  bazelbump undo --dry-run

  # Restore a specific snapshot
  bazelbump undo --snapshot 20240115-143022

%s
  For git strategy:
    - Uses 'git checkout' to restore files to their state before the run
    - Will fail if HEAD has moved since the run (new commits made)

  For file strategy:
    - Restores files from the snapshot backup
    - Can restore from any available snapshot using --snapshot
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"),
			color(colorBold, "Notes:"))
	}

	projectPath := fs.String("path", ".", "Path to project directory")
	snapshotID := fs.String("snapshot", "", "Restore a specific snapshot (file strategy only)")
	dryRun := fs.Bool("dry-run", false, "Show what would be undone without making changes")
	force := fs.Bool("force", false, "Force undo even if warnings would normally prevent it")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Read last run info to determine strategy
	info, err := track.ReadLastRun(*projectPath)
	if err != nil {
		if strings.Contains(err.Error(), "no previous run found") {
			fmt.Printf("%s No previous bazelbump run found to undo.\n", color(colorYellow, "[INFO]"))
			return nil
		}
		return err
	}

	// Validate
	if len(info.ModifiedFiles) == 0 {
		fmt.Printf("%s No files were modified in the last run. Nothing to undo.\n",
			color(colorYellow, "[INFO]"))
		return nil
	}

	// Show what will be undone
	fmt.Printf("%s Undoing %s run from %s\n\n",
		color(colorBlue, "==>"),
		info.Recipe,
		info.Timestamp.Format("2006-01-02 15:04:05"))

	fmt.Printf("Files to restore (%d):\n", len(info.ModifiedFiles))
	for _, f := range info.ModifiedFiles {
		fmt.Printf("  - %s\n", f)
	}
	fmt.Println()

	// Strategy-specific warnings
	if info.Strategy == "git" {
		if info.GitDirty && !*force {
			fmt.Printf("%s The working tree was dirty when the recipe ran.\n",
				color(colorYellow, "[WARN]"))
			fmt.Printf("    Some changes may have been made manually before the run.\n")
			fmt.Printf("    Use --force to proceed anyway, or manually review changes first.\n")
			if !*dryRun {
				return fmt.Errorf("undo aborted due to dirty working tree warning")
			}
		}

		// Check if HEAD has moved
		if !*dryRun {
			_, _, err := track.GetGitStatus(*projectPath)
			if err != nil {
				return fmt.Errorf("failed to check git status: %w", err)
			}
		}
	}

	if *dryRun {
		fmt.Printf("%s Dry run complete. Would restore %d file(s).\n",
			color(colorGreen, "[OK]"),
			len(info.ModifiedFiles))
		fmt.Printf("    Run without --dry-run to actually undo changes.\n")
		return nil
	}

	// Create tracker and perform undo
	tracker, err := track.NewTracker(track.TrackerOptions{
		ProjectPath: *projectPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create tracker: %w", err)
	}

	// Verify strategy matches
	if tracker.Strategy() != info.Strategy {
		return fmt.Errorf("current strategy (%s) differs from run strategy (%s)",
			tracker.Strategy(), info.Strategy)
	}

	// Perform undo
	if err := tracker.Undo(*snapshotID); err != nil {
		return fmt.Errorf("undo failed: %w", err)
	}

	fmt.Printf("%s Successfully restored %d file(s).\n",
		color(colorGreen, "[OK]"),
		len(info.ModifiedFiles))

	return nil
}
