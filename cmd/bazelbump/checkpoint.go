// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/albertocavalcante/bazelbump/pkg/state"
)

func runCheckpoint(args []string) error {
	if len(args) < 1 {
		printCheckpointUsage()
		return fmt.Errorf("checkpoint subcommand required")
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list":
		return runCheckpointList(subargs)
	case "restore":
		return runCheckpointRestore(subargs)
	case "help", "-h", "--help":
		printCheckpointUsage()
		return nil
	default:
		printCheckpointUsage()
		return fmt.Errorf("unknown checkpoint subcommand: %s", subcmd)
	}
}

func printCheckpointUsage() {
	fmt.Fprintf(os.Stderr, `Manage bazelbump checkpoints.

Checkpoints are savepoints created during recipe execution that allow
resuming interrupted runs or rolling back to a previous state.

%s
  bazelbump checkpoint <subcommand> [flags]

%s
  list      List checkpoints for a run
  restore   Restore execution state from a checkpoint

%s
  bazelbump checkpoint list
  bazelbump checkpoint list --run <run-id>
  bazelbump checkpoint restore <checkpoint-id>

%s
  Checkpoints are created automatically at regular intervals (default: every 100 files)
  and at step boundaries. You can also create manual checkpoints during execution.

  Types of checkpoints:
    auto           Created automatically at file intervals
    step_boundary  Created between recipe steps
    manual         Created by explicit user command

Use "bazelbump checkpoint <subcommand> --help" for more information.
`, color(colorBold, "Usage:"),
		color(colorBold, "Subcommands:"),
		color(colorBold, "Examples:"),
		color(colorBold, "Checkpoint Types:"))
}

func runCheckpointList(args []string) error {
	fs := flag.NewFlagSet("checkpoint list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `List checkpoints for a run.

%s
  bazelbump checkpoint list [flags]

%s
  --run      Run ID (default: most recent run)
  --path     Path to project directory (default: current directory)
  --json     Output results as JSON
  --help     Show this help

%s
  bazelbump checkpoint list
  bazelbump checkpoint list --run abc123
  bazelbump checkpoint list --json
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	runID := fs.String("run", "", "Run ID (default: most recent run)")
	projectPath := fs.String("path", ".", "Path to project directory")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check if state management is available
	if !state.Available() {
		fmt.Printf("%s State management is not available in this build.\n",
			color(colorYellow, "[INFO]"))
		fmt.Printf("\nThis build was compiled without SQLite support (nostate build tag).\n")
		fmt.Printf("Checkpoint management requires a full build with state management enabled.\n")
		return nil
	}

	// Open state database
	dbPath := state.StateDBPath(*projectPath)
	mgr, err := state.New(dbPath)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			fmt.Printf("%s No state database found in this project.\n",
				color(colorYellow, "[INFO]"))
			return nil
		}
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer mgr.Close()

	// Get run ID
	targetRunID := *runID
	if targetRunID == "" {
		// Get most recent run
		runs, err := mgr.ListRuns(state.ListRunsOptions{Limit: 1})
		if err != nil {
			return fmt.Errorf("failed to get recent run: %w", err)
		}
		if len(runs) == 0 {
			fmt.Printf("%s No runs found.\n", color(colorYellow, "[INFO]"))
			return nil
		}
		targetRunID = runs[0].ID
	}

	// Get run info for display
	run, err := mgr.GetRun(targetRunID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", targetRunID)
	}

	// List checkpoints
	checkpoints, err := mgr.ListCheckpoints(targetRunID)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if len(checkpoints) == 0 {
		fmt.Printf("%s No checkpoints found for run %s.\n",
			color(colorYellow, "[INFO]"), truncateID(targetRunID, 12))
		return nil
	}

	if *jsonOutput {
		return outputCheckpointListJSON(run, checkpoints)
	}

	outputCheckpointListText(run, checkpoints)
	return nil
}

// checkpointListOutput is the JSON format for checkpoint listing.
type checkpointListOutput struct {
	RunID       string             `json:"run_id"`
	Recipe      string             `json:"recipe"`
	Checkpoints []checkpointDetail `json:"checkpoints"`
}

type checkpointDetail struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	StepID      string `json:"step_id"`
	StepIndex   int    `json:"step_index"`
	FileIndex   int    `json:"file_index"`
	CreatedAt   string `json:"created_at"`
	Description string `json:"description,omitempty"`
}

func outputCheckpointListJSON(run *state.Run, checkpoints []*state.Checkpoint) error {
	out := checkpointListOutput{
		RunID:       run.ID,
		Recipe:      run.RecipeName,
		Checkpoints: make([]checkpointDetail, 0, len(checkpoints)),
	}

	for _, cp := range checkpoints {
		out.Checkpoints = append(out.Checkpoints, checkpointDetail{
			ID:          cp.ID,
			Type:        string(cp.Type),
			StepID:      cp.StepID,
			StepIndex:   cp.StepIndex,
			FileIndex:   cp.FileIndex,
			CreatedAt:   cp.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Description: cp.Description,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputCheckpointListText(run *state.Run, checkpoints []*state.Checkpoint) {
	fmt.Printf("%s Checkpoints for %s (%s)\n\n",
		color(colorBlue, "==>"),
		color(colorCyan, run.RecipeName),
		truncateID(run.ID, 12))

	// Print header
	fmt.Printf("  %-12s  %-15s  %-20s  %-10s  %s\n",
		"ID", "TYPE", "STEP", "FILE", "CREATED")
	fmt.Printf("  %s\n", strings.Repeat("-", 70))

	for _, cp := range checkpoints {
		typeColor := colorReset
		switch cp.Type {
		case state.CheckpointAuto:
			typeColor = colorCyan
		case state.CheckpointManual:
			typeColor = colorGreen
		case state.CheckpointStepBoundary:
			typeColor = colorYellow
		}

		stepStr := cp.StepID
		if len(stepStr) > 20 {
			stepStr = stepStr[:17] + "..."
		}

		fileStr := fmt.Sprintf("%d", cp.FileIndex)
		if run.TotalFiles > 0 {
			fileStr = fmt.Sprintf("%d/%d", cp.FileIndex, run.TotalFiles)
		}

		fmt.Printf("  %-12s  %s%-15s%s  %-20s  %-10s  %s\n",
			truncateID(cp.ID, 12),
			typeColor, string(cp.Type), colorReset,
			stepStr,
			fileStr,
			cp.CreatedAt.Format("15:04:05"))
	}

	fmt.Printf("\n  Total: %d checkpoint(s)\n", len(checkpoints))
	fmt.Printf("\n  Use 'bazelbump checkpoint restore <id>' to restore.\n")
}

func runCheckpointRestore(args []string) error {
	fs := flag.NewFlagSet("checkpoint restore", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Restore execution state from a checkpoint.

%s
  bazelbump checkpoint restore [flags] <checkpoint-id>

%s
  --path     Path to project directory (default: current directory)
  --force    Skip confirmation prompt
  --help     Show this help

%s
  bazelbump checkpoint restore abc123
  bazelbump checkpoint restore --force abc123

%s
  Restoring a checkpoint will:
  1. Load the execution state from the checkpoint
  2. Allow resuming the recipe run from that point

  Note: This does NOT restore file contents. Use 'bazelbump undo' to revert
  file changes, or use your version control system.
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"),
			color(colorBold, "What Happens:"))
	}

	projectPath := fs.String("path", ".", "Path to project directory")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("checkpoint ID required")
	}

	checkpointID := fs.Arg(0)

	// Check if state management is available
	if !state.Available() {
		fmt.Printf("%s State management is not available in this build.\n",
			color(colorYellow, "[INFO]"))
		fmt.Printf("\nThis build was compiled without SQLite support (nostate build tag).\n")
		return nil
	}

	// Open state database
	dbPath := state.StateDBPath(*projectPath)
	mgr, err := state.New(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer mgr.Close()

	// Get checkpoint
	cp, err := mgr.GetCheckpoint(checkpointID)
	if err != nil {
		return fmt.Errorf("failed to get checkpoint: %w", err)
	}
	if cp == nil {
		return fmt.Errorf("checkpoint not found: %s", checkpointID)
	}

	// Get run info
	run, err := mgr.GetRun(cp.RunID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found for checkpoint")
	}

	// Show what will happen
	fmt.Printf("%s Restore checkpoint\n\n", color(colorBlue, "==>"))
	fmt.Printf("  Checkpoint:  %s\n", cp.ID)
	fmt.Printf("  Type:        %s\n", cp.Type)
	fmt.Printf("  Created:     %s\n", cp.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\n")
	fmt.Printf("  Run:         %s\n", run.ID)
	fmt.Printf("  Recipe:      %s\n", run.RecipeName)
	fmt.Printf("  Step:        %s (index %d)\n", cp.StepID, cp.StepIndex)
	fmt.Printf("  File:        %d\n", cp.FileIndex)

	if !*force {
		fmt.Printf("\n%s This will prepare the run for resumption from this checkpoint.\n",
			color(colorYellow, "[WARN]"))
		fmt.Printf("File contents are NOT restored. Use 'bazelbump undo' or git for that.\n")
		fmt.Printf("\nContinue? [y/N] ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Printf("Cancelled.\n")
			return nil
		}
	}

	// Update run status to allow resumption
	if err := mgr.UpdateRunStatus(cp.RunID, state.RunStatusInterrupted); err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}

	// Log the restore action
	if err := mgr.LogAction(cp.RunID, state.AuditCheckpointRestored, state.AuditLogOptions{
		Description: fmt.Sprintf("Restored to checkpoint %s", cp.ID),
	}); err != nil {
		// Non-fatal, just log it
		fmt.Printf("Warning: failed to log restore action: %v\n", err)
	}

	fmt.Printf("\n%s Checkpoint restored.\n", color(colorGreen, "[OK]"))
	fmt.Printf("\n  To resume: bazelbump recipe run %s --resume\n", run.RecipeName)

	return nil
}

// truncateID shortens an ID for display.
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}
