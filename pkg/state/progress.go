// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ProgressReporter receives execution events and displays progress.
// Implementations can provide different output formats (terminal, JSON, silent).
type ProgressReporter interface {
	// Run lifecycle
	OnRunStart(run *Run)
	OnRunComplete(run *Run, success bool)

	// Step lifecycle
	OnStepStart(stepID string, stepIndex, totalSteps int)
	OnStepComplete(stepID string, success bool)

	// File lifecycle
	OnFileStart(path string)
	OnFileComplete(path string, changed bool)

	// Progress updates
	OnProgress(processed, total int)

	// Checkpoints
	OnCheckpoint(cp *Checkpoint)

	// Messages
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// TerminalReporter provides pretty output for interactive terminals.
type TerminalReporter struct {
	out          io.Writer
	verbose      bool
	noColor      bool
	mu           sync.Mutex
	currentRun   *Run
	currentStep  string
	stepIndex    int
	totalSteps   int
	startTime    time.Time
	lastProgress int
}

// NewTerminalReporter creates a new terminal progress reporter.
func NewTerminalReporter(opts TerminalReporterOptions) *TerminalReporter {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}

	noColor := opts.NoColor
	if !noColor {
		noColor = os.Getenv("NO_COLOR") != ""
	}

	return &TerminalReporter{
		out:     out,
		verbose: opts.Verbose,
		noColor: noColor,
	}
}

// TerminalReporterOptions configures the terminal reporter.
type TerminalReporterOptions struct {
	Out     io.Writer
	Verbose bool
	NoColor bool
}

func (r *TerminalReporter) color(c, s string) string {
	if r.noColor {
		return s
	}
	return c + s + colorReset
}

func (r *TerminalReporter) printf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.out, format, args...)
}

// OnRunStart is called when a recipe run begins.
func (r *TerminalReporter) OnRunStart(run *Run) {
	r.mu.Lock()
	r.currentRun = run
	r.startTime = time.Now()
	r.mu.Unlock()

	mode := ""
	if run.DryRun {
		mode = r.color(colorYellow, " (dry-run)")
	}
	r.printf("%s Running recipe: %s%s\n",
		r.color(colorBlue, "==>"),
		r.color(colorBold, run.RecipeName),
		mode)
}

// OnRunComplete is called when a recipe run finishes.
func (r *TerminalReporter) OnRunComplete(run *Run, success bool) {
	elapsed := time.Since(r.startTime)

	r.printf("\n")
	if success {
		r.printf("%s Recipe completed successfully in %s\n",
			r.color(colorGreen, "[OK]"),
			formatDuration(elapsed))
		if run.ChangesCount > 0 {
			r.printf("    %d file(s) modified\n", run.ChangesCount)
		}
	} else {
		r.printf("%s Recipe failed after %s\n",
			r.color(colorRed, "[FAIL]"),
			formatDuration(elapsed))
		if run.ErrorMessage != "" {
			r.printf("    %s %s\n", r.color(colorRed, "error:"), run.ErrorMessage)
		}
	}
}

// OnStepStart is called when a step begins execution.
func (r *TerminalReporter) OnStepStart(stepID string, stepIndex, totalSteps int) {
	r.mu.Lock()
	r.currentStep = stepID
	r.stepIndex = stepIndex
	r.totalSteps = totalSteps
	r.lastProgress = 0
	r.mu.Unlock()

	r.printf("\n%s Step %d/%d: %s\n",
		r.color(colorBlue, "==>"),
		stepIndex+1, totalSteps,
		r.color(colorCyan, stepID))
}

// OnStepComplete is called when a step finishes.
func (r *TerminalReporter) OnStepComplete(stepID string, success bool) {
	if success {
		r.printf("    %s Step completed\n", r.color(colorGreen, "[OK]"))
	} else {
		r.printf("    %s Step failed\n", r.color(colorRed, "[FAIL]"))
	}
}

// OnFileStart is called when file processing begins.
func (r *TerminalReporter) OnFileStart(path string) {
	if r.verbose {
		r.printf("    %s %s\n", r.color(colorDim, "Processing:"), path)
	}
}

// OnFileComplete is called when file processing finishes.
func (r *TerminalReporter) OnFileComplete(path string, changed bool) {
	if r.verbose && changed {
		r.printf("    %s %s\n", r.color(colorGreen, "[changed]"), path)
	}
}

// OnProgress is called to update progress.
func (r *TerminalReporter) OnProgress(processed, total int) {
	r.mu.Lock()
	lastProgress := r.lastProgress
	r.lastProgress = processed
	r.mu.Unlock()

	// Only update at certain intervals to avoid too much output
	if total > 100 && processed-lastProgress < total/20 && processed != total {
		return
	}

	percent := 0
	if total > 0 {
		percent = processed * 100 / total
	}

	// Simple progress line
	r.printf("    Progress: %d/%d files (%d%%)\r", processed, total, percent)
	if processed == total {
		r.printf("\n")
	}
}

// OnCheckpoint is called when a checkpoint is created.
func (r *TerminalReporter) OnCheckpoint(cp *Checkpoint) {
	r.printf("    %s Checkpoint saved: %s (%s)\n",
		r.color(colorGreen, "[OK]"),
		cp.ID[:8],
		cp.Type)
}

// Info logs an informational message.
func (r *TerminalReporter) Info(format string, args ...any) {
	r.printf("%s %s\n", r.color(colorBlue, "[INFO]"), fmt.Sprintf(format, args...))
}

// Warn logs a warning message.
func (r *TerminalReporter) Warn(format string, args ...any) {
	r.printf("%s %s\n", r.color(colorYellow, "[WARN]"), fmt.Sprintf(format, args...))
}

// Error logs an error message.
func (r *TerminalReporter) Error(format string, args ...any) {
	r.printf("%s %s\n", r.color(colorRed, "[ERROR]"), fmt.Sprintf(format, args...))
}

// JSONReporter provides structured JSON output for CI/scripting.
type JSONReporter struct {
	out io.Writer
	enc *json.Encoder
	mu  sync.Mutex
}

// NewJSONReporter creates a new JSON progress reporter.
func NewJSONReporter(out io.Writer) *JSONReporter {
	if out == nil {
		out = os.Stdout
	}
	return &JSONReporter{
		out: out,
		enc: json.NewEncoder(out),
	}
}

// ProgressEvent is a JSON-serialized progress event.
type ProgressEvent struct {
	Event       string `json:"event"`
	Timestamp   string `json:"timestamp"`
	RunID       string `json:"run_id,omitempty"`
	Recipe      string `json:"recipe,omitempty"`
	StepID      string `json:"step_id,omitempty"`
	StepIndex   int    `json:"step_index,omitempty"`
	TotalSteps  int    `json:"total_steps,omitempty"`
	Path        string `json:"path,omitempty"`
	Changed     bool   `json:"changed,omitempty"`
	Processed   int    `json:"processed,omitempty"`
	Total       int    `json:"total,omitempty"`
	Success     bool   `json:"success,omitempty"`
	Message     string `json:"message,omitempty"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	CheckpointType string `json:"checkpoint_type,omitempty"`
}

func (r *JSONReporter) emit(event ProgressEvent) {
	event.Timestamp = time.Now().Format(time.RFC3339)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enc.Encode(event)
}

// OnRunStart emits a run_start event.
func (r *JSONReporter) OnRunStart(run *Run) {
	r.emit(ProgressEvent{
		Event:  "run_start",
		RunID:  run.ID,
		Recipe: run.RecipeName,
	})
}

// OnRunComplete emits a run_complete event.
func (r *JSONReporter) OnRunComplete(run *Run, success bool) {
	r.emit(ProgressEvent{
		Event:   "run_complete",
		RunID:   run.ID,
		Success: success,
		Message: run.ErrorMessage,
	})
}

// OnStepStart emits a step_start event.
func (r *JSONReporter) OnStepStart(stepID string, stepIndex, totalSteps int) {
	r.emit(ProgressEvent{
		Event:      "step_start",
		StepID:     stepID,
		StepIndex:  stepIndex,
		TotalSteps: totalSteps,
	})
}

// OnStepComplete emits a step_complete event.
func (r *JSONReporter) OnStepComplete(stepID string, success bool) {
	r.emit(ProgressEvent{
		Event:   "step_complete",
		StepID:  stepID,
		Success: success,
	})
}

// OnFileStart emits a file_start event.
func (r *JSONReporter) OnFileStart(path string) {
	r.emit(ProgressEvent{
		Event: "file_start",
		Path:  path,
	})
}

// OnFileComplete emits a file_complete event.
func (r *JSONReporter) OnFileComplete(path string, changed bool) {
	r.emit(ProgressEvent{
		Event:   "file_complete",
		Path:    path,
		Changed: changed,
	})
}

// OnProgress emits a progress event.
func (r *JSONReporter) OnProgress(processed, total int) {
	r.emit(ProgressEvent{
		Event:     "progress",
		Processed: processed,
		Total:     total,
	})
}

// OnCheckpoint emits a checkpoint event.
func (r *JSONReporter) OnCheckpoint(cp *Checkpoint) {
	r.emit(ProgressEvent{
		Event:          "checkpoint",
		CheckpointID:   cp.ID,
		CheckpointType: string(cp.Type),
	})
}

// Info emits an info event.
func (r *JSONReporter) Info(format string, args ...any) {
	r.emit(ProgressEvent{
		Event:   "info",
		Message: fmt.Sprintf(format, args...),
	})
}

// Warn emits a warn event.
func (r *JSONReporter) Warn(format string, args ...any) {
	r.emit(ProgressEvent{
		Event:   "warn",
		Message: fmt.Sprintf(format, args...),
	})
}

// Error emits an error event.
func (r *JSONReporter) Error(format string, args ...any) {
	r.emit(ProgressEvent{
		Event:   "error",
		Message: fmt.Sprintf(format, args...),
	})
}

// NoopReporter is a silent reporter that discards all events.
type NoopReporter struct{}

// NewNoopReporter creates a new no-op reporter.
func NewNoopReporter() *NoopReporter {
	return &NoopReporter{}
}

func (r *NoopReporter) OnRunStart(run *Run)                           {}
func (r *NoopReporter) OnRunComplete(run *Run, success bool)          {}
func (r *NoopReporter) OnStepStart(stepID string, stepIndex, totalSteps int) {}
func (r *NoopReporter) OnStepComplete(stepID string, success bool)    {}
func (r *NoopReporter) OnFileStart(path string)                       {}
func (r *NoopReporter) OnFileComplete(path string, changed bool)      {}
func (r *NoopReporter) OnProgress(processed, total int)               {}
func (r *NoopReporter) OnCheckpoint(cp *Checkpoint)                   {}
func (r *NoopReporter) Info(format string, args ...any)               {}
func (r *NoopReporter) Warn(format string, args ...any)               {}
func (r *NoopReporter) Error(format string, args ...any)              {}

// CompositeReporter sends events to multiple reporters.
type CompositeReporter struct {
	reporters []ProgressReporter
}

// NewCompositeReporter creates a reporter that sends to multiple destinations.
func NewCompositeReporter(reporters ...ProgressReporter) *CompositeReporter {
	return &CompositeReporter{reporters: reporters}
}

func (r *CompositeReporter) OnRunStart(run *Run) {
	for _, rep := range r.reporters {
		rep.OnRunStart(run)
	}
}

func (r *CompositeReporter) OnRunComplete(run *Run, success bool) {
	for _, rep := range r.reporters {
		rep.OnRunComplete(run, success)
	}
}

func (r *CompositeReporter) OnStepStart(stepID string, stepIndex, totalSteps int) {
	for _, rep := range r.reporters {
		rep.OnStepStart(stepID, stepIndex, totalSteps)
	}
}

func (r *CompositeReporter) OnStepComplete(stepID string, success bool) {
	for _, rep := range r.reporters {
		rep.OnStepComplete(stepID, success)
	}
}

func (r *CompositeReporter) OnFileStart(path string) {
	for _, rep := range r.reporters {
		rep.OnFileStart(path)
	}
}

func (r *CompositeReporter) OnFileComplete(path string, changed bool) {
	for _, rep := range r.reporters {
		rep.OnFileComplete(path, changed)
	}
}

func (r *CompositeReporter) OnProgress(processed, total int) {
	for _, rep := range r.reporters {
		rep.OnProgress(processed, total)
	}
}

func (r *CompositeReporter) OnCheckpoint(cp *Checkpoint) {
	for _, rep := range r.reporters {
		rep.OnCheckpoint(cp)
	}
}

func (r *CompositeReporter) Info(format string, args ...any) {
	for _, rep := range r.reporters {
		rep.Info(format, args...)
	}
}

func (r *CompositeReporter) Warn(format string, args ...any) {
	for _, rep := range r.reporters {
		rep.Warn(format, args...)
	}
}

func (r *CompositeReporter) Error(format string, args ...any) {
	for _, rep := range r.reporters {
		rep.Error(format, args...)
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// ProgressBar renders a simple progress bar string.
func ProgressBar(current, total, width int) string {
	if total <= 0 {
		return strings.Repeat(" ", width)
	}

	filled := current * width / total
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
		bar += strings.Repeat(" ", width-filled-1)
	}

	return "[" + bar + "]"
}
