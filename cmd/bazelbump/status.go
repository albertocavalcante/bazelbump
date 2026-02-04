// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/albertocavalcante/bazelbump/pkg/state"
	"github.com/albertocavalcante/bazelbump/pkg/track"
)

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Show status of last bazelbump run.

Displays information about the most recent recipe execution including
which files were modified and whether undo is available.

%s
  bazelbump status [flags]

%s
  --path   Path to project directory (default: current directory)
  --json   Output results as JSON
  --help   Show this help

%s
  bazelbump status
  bazelbump status --json
  bazelbump status --path /path/to/project
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	projectPath := fs.String("path", ".", "Path to project directory")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Read last run info
	info, err := track.ReadLastRun(*projectPath)
	if err != nil {
		if strings.Contains(err.Error(), "no previous run found") {
			fmt.Printf("%s No previous bazelbump run found in this project.\n",
				color(colorYellow, "[INFO]"))
			fmt.Printf("\nRun a recipe first:\n")
			fmt.Printf("  bazelbump recipe run <recipe-name>\n")
			return nil
		}
		return err
	}

	if *jsonOutput {
		return outputStatusJSON(info)
	}

	outputStatusText(info, *projectPath)
	return nil
}

// statusOutput is the JSON format for status command.
type statusOutput struct {
	Recipe        string   `json:"recipe"`
	Timestamp     string   `json:"timestamp"`
	TimestampUnix int64    `json:"timestamp_unix"`
	Strategy      string   `json:"strategy"`
	Success       bool     `json:"success"`
	ModifiedFiles []string `json:"modified_files"`
	FileCount     int      `json:"file_count"`
	SnapshotID    string   `json:"snapshot_id,omitempty"`
	GitCommit     string   `json:"git_commit,omitempty"`
	GitDirty      bool     `json:"git_dirty,omitempty"`
	UndoAvailable bool     `json:"undo_available"`
}

func outputStatusJSON(info *track.RunInfo) error {
	out := statusOutput{
		Recipe:        info.Recipe,
		Timestamp:     info.Timestamp.Format(time.RFC3339),
		TimestampUnix: info.Timestamp.Unix(),
		Strategy:      info.Strategy,
		Success:       info.Success,
		ModifiedFiles: info.ModifiedFiles,
		FileCount:     len(info.ModifiedFiles),
		SnapshotID:    info.SnapshotID,
		GitCommit:     info.GitCommit,
		GitDirty:      info.GitDirty,
		UndoAvailable: len(info.ModifiedFiles) > 0,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputStatusText(info *track.RunInfo, projectPath string) {
	// Header
	fmt.Printf("%s Last bazelbump run\n\n", color(colorBlue, "==>"))

	// Recipe info
	fmt.Printf("  Recipe:    %s\n", color(colorCyan, info.Recipe))
	fmt.Printf("  Time:      %s (%s ago)\n",
		info.Timestamp.Format("2006-01-02 15:04:05"),
		formatDuration(time.Since(info.Timestamp)))

	// Status
	if info.Success {
		fmt.Printf("  Status:    %s\n", color(colorGreen, "Success"))
	} else {
		fmt.Printf("  Status:    %s\n", color(colorRed, "Failed"))
	}

	// Strategy info
	fmt.Printf("  Strategy:  %s\n", info.Strategy)
	if info.Strategy == "git" {
		if info.GitCommit != "" {
			shortCommit := info.GitCommit
			if len(shortCommit) > 8 {
				shortCommit = shortCommit[:8]
			}
			fmt.Printf("  Git HEAD:  %s", shortCommit)
			if info.GitDirty {
				fmt.Printf(" %s", color(colorYellow, "(dirty)"))
			}
			fmt.Println()
		}
	} else if info.Strategy == "file" {
		if info.SnapshotID != "" {
			fmt.Printf("  Snapshot:  %s\n", info.SnapshotID)
		}
	}

	// Modified files
	fmt.Printf("\n%s %d file(s)\n", color(colorBold, "Modified files:"), len(info.ModifiedFiles))
	if len(info.ModifiedFiles) > 0 {
		// Show up to 10 files
		maxShow := 10
		for i, f := range info.ModifiedFiles {
			if i >= maxShow {
				fmt.Printf("  ... and %d more\n", len(info.ModifiedFiles)-maxShow)
				break
			}
			fmt.Printf("  - %s\n", f)
		}
	} else {
		fmt.Printf("  (none)\n")
	}

	// Undo hint
	if len(info.ModifiedFiles) > 0 {
		fmt.Printf("\n%s\n", color(colorBold, "To undo:"))
		fmt.Printf("  bazelbump undo\n")
	}

	// Show state manager info if available
	if state.Available() {
		showStateManagerStatus(projectPath, info.Recipe)
	}
}

// showStateManagerStatus displays additional status from the state manager.
func showStateManagerStatus(projectPath, recipe string) {
	dbPath := state.StateDBPath(projectPath)
	mgr, err := state.New(dbPath)
	if err != nil {
		// No state database yet, that's fine
		return
	}
	defer mgr.Close()

	// Get latest run for this recipe
	run, err := mgr.GetLatestRun(recipe)
	if err != nil || run == nil {
		return
	}

	// Show checkpoint info if run was interrupted
	if run.Status == state.RunStatusInterrupted || run.Status == state.RunStatusRunning {
		cp, err := mgr.GetLatestCheckpoint(run.ID)
		if err == nil && cp != nil {
			fmt.Printf("\n%s\n", color(colorBold, "Checkpoint available:"))
			fmt.Printf("  Step:      %s (file %d/%d)\n", cp.StepID, cp.FileIndex, run.TotalFiles)
			fmt.Printf("  Created:   %s\n", cp.CreatedAt.Format("15:04:05"))
			fmt.Printf("\n  To resume: bazelbump recipe run %s --resume\n", recipe)
		}
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
