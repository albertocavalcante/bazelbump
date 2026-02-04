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
)

func runHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Show bazelbump run history.

Displays information about previous recipe executions including
status, timing, and changes made.

%s
  bazelbump history [flags]

%s
  --run        Show details for a specific run ID
  --recipe     Filter by recipe name
  --since      Show runs since (e.g., "24h", "7d")
  --status     Filter by status (completed, failed, interrupted)
  --limit      Maximum number of runs to show (default: 10)
  --path       Path to project directory (default: current directory)
  --json       Output results as JSON
  --help       Show this help

%s
  bazelbump history
  bazelbump history --since 7d
  bazelbump history --recipe autoload-migration
  bazelbump history --run run-abc123
  bazelbump history --status failed --limit 5
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	runID := fs.String("run", "", "Show details for a specific run ID")
	recipeName := fs.String("recipe", "", "Filter by recipe name")
	since := fs.String("since", "", "Show runs since (e.g., '24h', '7d')")
	statusFilter := fs.String("status", "", "Filter by status")
	limit := fs.Int("limit", 10, "Maximum number of runs to show")
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
		fmt.Printf("Run history requires a full build with state management enabled.\n")
		return nil
	}

	// Open state database
	dbPath := state.StateDBPath(*projectPath)
	mgr, err := state.New(dbPath)
	if err != nil {
		// Database might not exist yet
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			fmt.Printf("%s No run history found in this project.\n",
				color(colorYellow, "[INFO]"))
			fmt.Printf("\nRun a recipe first:\n")
			fmt.Printf("  bazelbump recipe run <recipe-name>\n")
			return nil
		}
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer mgr.Close()

	// If specific run ID requested, show details
	if *runID != "" {
		return showRunDetails(mgr, *runID, *jsonOutput)
	}

	// Build list options
	opts := state.ListRunsOptions{
		Recipe: *recipeName,
		Limit:  *limit,
	}

	if *statusFilter != "" {
		opts.Status = state.RunStatus(*statusFilter)
	}

	if *since != "" {
		duration, err := parseDuration(*since)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		sinceTime := time.Now().Add(-duration)
		opts.Since = &sinceTime
	}

	// List runs
	runs, err := mgr.ListRuns(opts)
	if err != nil {
		return fmt.Errorf("failed to list runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Printf("%s No runs found matching the criteria.\n",
			color(colorYellow, "[INFO]"))
		return nil
	}

	if *jsonOutput {
		return outputHistoryJSON(runs)
	}

	outputHistoryText(runs)
	return nil
}

// showRunDetails displays detailed information about a specific run.
func showRunDetails(mgr state.Manager, runID string, jsonOutput bool) error {
	run, err := mgr.GetRun(runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Get checkpoints for this run
	checkpoints, err := mgr.ListCheckpoints(runID)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if jsonOutput {
		return outputRunDetailsJSON(run, checkpoints)
	}

	outputRunDetailsText(run, checkpoints)
	return nil
}

// historyOutput is the JSON format for history listing.
type historyOutput struct {
	Runs []historyRunOutput `json:"runs"`
}

// historyRunOutput is a single run in the history output.
type historyRunOutput struct {
	ID             string  `json:"id"`
	Recipe         string  `json:"recipe"`
	Version        string  `json:"version,omitempty"`
	Status         string  `json:"status"`
	StartedAt      string  `json:"started_at"`
	CompletedAt    string  `json:"completed_at,omitempty"`
	DurationMs     int64   `json:"duration_ms,omitempty"`
	TotalFiles     int     `json:"total_files"`
	ProcessedFiles int     `json:"processed_files"`
	ChangesCount   int     `json:"changes_count"`
	DryRun         bool    `json:"dry_run"`
	ErrorMessage   string  `json:"error_message,omitempty"`
}

func outputHistoryJSON(runs []*state.Run) error {
	out := historyOutput{
		Runs: make([]historyRunOutput, 0, len(runs)),
	}

	for _, run := range runs {
		r := historyRunOutput{
			ID:             run.ID,
			Recipe:         run.RecipeName,
			Version:        run.RecipeVersion,
			Status:         string(run.Status),
			StartedAt:      run.StartedAt.Format(time.RFC3339),
			TotalFiles:     run.TotalFiles,
			ProcessedFiles: run.ProcessedFiles,
			ChangesCount:   run.ChangesCount,
			DryRun:         run.DryRun,
			ErrorMessage:   run.ErrorMessage,
		}
		if run.CompletedAt != nil {
			r.CompletedAt = run.CompletedAt.Format(time.RFC3339)
			r.DurationMs = run.CompletedAt.Sub(run.StartedAt).Milliseconds()
		}
		out.Runs = append(out.Runs, r)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputHistoryText(runs []*state.Run) {
	fmt.Printf("%s Recent Runs:\n\n", color(colorBlue, "==>"))

	// Print header
	fmt.Printf("  %-12s  %-20s  %-12s  %-10s  %-10s  %s\n",
		"ID", "RECIPE", "STATUS", "FILES", "DURATION", "WHEN")
	fmt.Printf("  %s\n", strings.Repeat("-", 80))

	for _, run := range runs {
		// Truncate ID for display
		shortID := run.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		// Truncate recipe name
		recipe := run.RecipeName
		if len(recipe) > 20 {
			recipe = recipe[:17] + "..."
		}

		// Status with color
		status := string(run.Status)
		statusColor := colorReset
		switch run.Status {
		case state.RunStatusCompleted:
			statusColor = colorGreen
		case state.RunStatusFailed:
			statusColor = colorRed
		case state.RunStatusInterrupted:
			statusColor = colorYellow
		case state.RunStatusRunning:
			statusColor = colorCyan
		}

		// Files progress
		files := fmt.Sprintf("%d/%d", run.ProcessedFiles, run.TotalFiles)

		// Duration
		duration := "-"
		if run.CompletedAt != nil {
			duration = formatShortDuration(run.CompletedAt.Sub(run.StartedAt))
		} else if run.Status == state.RunStatusRunning {
			duration = formatShortDuration(time.Since(run.StartedAt)) + "..."
		}

		// When
		when := formatTimeAgo(run.StartedAt)

		// Add dry-run indicator
		if run.DryRun {
			status += " (dry)"
		}

		fmt.Printf("  %-12s  %-20s  %s%-12s%s  %-10s  %-10s  %s\n",
			shortID, recipe,
			statusColor, status, colorReset,
			files, duration, when)
	}

	fmt.Printf("\n  Total: %d run(s)\n", len(runs))
	fmt.Printf("\n  Use 'bazelbump history --run <id>' for details.\n")
}

// runDetailsOutput is the JSON format for run details.
type runDetailsOutput struct {
	Run         historyRunOutput        `json:"run"`
	Checkpoints []checkpointOutput      `json:"checkpoints"`
}

type checkpointOutput struct {
	ID        string `json:"id"`
	StepID    string `json:"step_id"`
	StepIndex int    `json:"step_index"`
	FileIndex int    `json:"file_index"`
	CreatedAt string `json:"created_at"`
	Type      string `json:"type"`
}

func outputRunDetailsJSON(run *state.Run, checkpoints []*state.Checkpoint) error {
	out := runDetailsOutput{
		Run: historyRunOutput{
			ID:             run.ID,
			Recipe:         run.RecipeName,
			Version:        run.RecipeVersion,
			Status:         string(run.Status),
			StartedAt:      run.StartedAt.Format(time.RFC3339),
			TotalFiles:     run.TotalFiles,
			ProcessedFiles: run.ProcessedFiles,
			ChangesCount:   run.ChangesCount,
			DryRun:         run.DryRun,
			ErrorMessage:   run.ErrorMessage,
		},
		Checkpoints: make([]checkpointOutput, 0, len(checkpoints)),
	}

	if run.CompletedAt != nil {
		out.Run.CompletedAt = run.CompletedAt.Format(time.RFC3339)
		out.Run.DurationMs = run.CompletedAt.Sub(run.StartedAt).Milliseconds()
	}

	for _, cp := range checkpoints {
		out.Checkpoints = append(out.Checkpoints, checkpointOutput{
			ID:        cp.ID,
			StepID:    cp.StepID,
			StepIndex: cp.StepIndex,
			FileIndex: cp.FileIndex,
			CreatedAt: cp.CreatedAt.Format(time.RFC3339),
			Type:      string(cp.Type),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputRunDetailsText(run *state.Run, checkpoints []*state.Checkpoint) {
	fmt.Printf("%s Run: %s\n\n", color(colorBlue, "==>"), run.ID)

	// Basic info
	fmt.Printf("  Recipe:      %s", color(colorCyan, run.RecipeName))
	if run.RecipeVersion != "" {
		fmt.Printf(" v%s", run.RecipeVersion)
	}
	fmt.Println()

	// Status with color
	statusColor := colorReset
	switch run.Status {
	case state.RunStatusCompleted:
		statusColor = colorGreen
	case state.RunStatusFailed:
		statusColor = colorRed
	case state.RunStatusInterrupted:
		statusColor = colorYellow
	case state.RunStatusRunning:
		statusColor = colorCyan
	}
	status := string(run.Status)
	if run.DryRun {
		status += " (dry-run)"
	}
	fmt.Printf("  Status:      %s%s%s\n", statusColor, status, colorReset)

	// Timing
	fmt.Printf("  Started:     %s (%s ago)\n",
		run.StartedAt.Format("2006-01-02 15:04:05"),
		formatTimeAgo(run.StartedAt))
	if run.CompletedAt != nil {
		duration := run.CompletedAt.Sub(run.StartedAt)
		fmt.Printf("  Duration:    %s\n", formatShortDuration(duration))
	}

	// Progress
	percent := 0
	if run.TotalFiles > 0 {
		percent = run.ProcessedFiles * 100 / run.TotalFiles
	}
	fmt.Printf("  Progress:    %d/%d files (%d%%)\n",
		run.ProcessedFiles, run.TotalFiles, percent)

	if run.CurrentStep != "" {
		fmt.Printf("  Current:     Step %s\n", run.CurrentStep)
	}

	// Git info
	if run.GitCommit != "" || run.GitBranch != "" {
		fmt.Printf("  Git:         ")
		if run.GitBranch != "" {
			fmt.Printf("%s", run.GitBranch)
		}
		if run.GitCommit != "" {
			shortCommit := run.GitCommit
			if len(shortCommit) > 8 {
				shortCommit = shortCommit[:8]
			}
			if run.GitBranch != "" {
				fmt.Printf(" @ %s", shortCommit)
			} else {
				fmt.Printf("%s", shortCommit)
			}
		}
		fmt.Println()
	}

	// Error
	if run.ErrorMessage != "" {
		fmt.Printf("\n  %s %s\n", color(colorRed, "Error:"), run.ErrorMessage)
	}

	// Changes
	if run.ChangesCount > 0 {
		fmt.Printf("\n  Files modified: %d\n", run.ChangesCount)
	}

	// Checkpoints
	if len(checkpoints) > 0 {
		fmt.Printf("\n%s\n", color(colorBold, "  Checkpoints:"))
		for _, cp := range checkpoints {
			shortID := cp.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			typeStr := string(cp.Type)
			fmt.Printf("    - %s: %s at step %s, file %d (%s)\n",
				shortID, typeStr, cp.StepID, cp.FileIndex,
				cp.CreatedAt.Format("15:04:05"))
		}
	}

	// Resume hint for interrupted runs
	if run.Status == state.RunStatusInterrupted || run.Status == state.RunStatusFailed {
		fmt.Printf("\n  To resume: bazelbump recipe run %s --resume\n", run.RecipeName)
	}
}

// parseDuration parses duration strings like "24h", "7d", "1w".
func parseDuration(s string) (time.Duration, error) {
	// Handle day/week suffixes
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		d, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, err
		}
		return d * 24, nil
	}
	if strings.HasSuffix(s, "w") {
		weeks := strings.TrimSuffix(s, "w")
		d, err := time.ParseDuration(weeks + "h")
		if err != nil {
			return 0, err
		}
		return d * 24 * 7, nil
	}

	return time.ParseDuration(s)
}

// formatShortDuration formats a duration in a compact way.
func formatShortDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// formatTimeAgo formats a time as "X ago".
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
