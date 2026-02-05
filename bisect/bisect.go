// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package bisect provides binary search functionality to find
// the first Bazel version that breaks a build.
package bisect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/albertocavalcante/bazelbump/flagdb"
)

// Config configures a bisect operation.
type Config struct {
	// FromVersion is the known-good version (required).
	FromVersion string

	// ToVersion is the known-bad or target version (required).
	ToVersion string

	// Granularity specifies the precision level.
	// Default: auto-detect from version format
	Granularity Granularity

	// TestCommand is the command to run at each version.
	// Default: "bazel build //..."
	TestCommand string

	// ProjectPath is the path to the project to test.
	// Default: current directory
	ProjectPath string

	// Timeout is the maximum time for each test.
	// Default: 5 minutes
	Timeout time.Duration

	// SkipPrerelease skips prerelease versions during bisect.
	// Only applies to release granularity.
	SkipPrerelease bool

	// MaxSteps limits the number of bisect iterations.
	// Default: 50
	MaxSteps int

	// Verbose enables detailed output.
	Verbose bool

	// OnProgress is called after each test with progress info.
	OnProgress func(Progress)
}

// Progress reports bisect progress.
type Progress struct {
	CurrentVersion string
	StepNumber     int
	TotalSteps     int
	Passed         bool
	Output         string
	Duration       time.Duration
}

// Result contains the bisect results.
type Result struct {
	// Engine info for transparency
	EngineName        string
	EngineDescription string
	IsNative          bool
	Granularity       Granularity

	// FirstBadVersion is the first version where the test failed.
	FirstBadVersion string

	// LastGoodVersion is the last version where the test passed.
	LastGoodVersion string

	// StepsTaken is the number of test iterations performed.
	StepsTaken int

	// FlagChanges lists flag changes between good and bad versions.
	FlagChanges *flagdb.Diff

	// Versions lists all versions tested with their results.
	TestedVersions []TestedVersion

	// Error is set if bisect failed to complete.
	Error error

	// For commit-level bisect (delegated)
	CommitResult *CommitBisectResult
}

// TestedVersion records a single version test.
type TestedVersion struct {
	Version  string
	Passed   bool
	Output   string
	Duration time.Duration
}

// Bisector performs binary search across Bazel versions.
type Bisector struct {
	db     *flagdb.DB
	config Config
}

// New creates a new Bisector with the given flag database and config.
func New(db *flagdb.DB, cfg Config) *Bisector {
	// Apply defaults
	if cfg.TestCommand == "" {
		cfg.TestCommand = "bazel build //..."
	}
	if cfg.ProjectPath == "" {
		cfg.ProjectPath = "."
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = 50
	}

	return &Bisector{
		db:     db,
		config: cfg,
	}
}

// Run executes the bisect operation.
func (b *Bisector) Run(ctx context.Context) (*Result, error) {
	// Determine granularity
	granularity := b.config.Granularity
	if granularity == GranularityAuto {
		granularity = DetectGranularity(b.config.FromVersion, b.config.ToVersion)
	}

	// Select engine based on granularity
	engine, err := SelectEngine(granularity, b.db)
	if err != nil {
		return nil, fmt.Errorf("select engine: %w", err)
	}

	result := &Result{
		EngineName:        engine.Name(),
		EngineDescription: engine.Description(),
		IsNative:          engine.IsNative(),
		Granularity:       granularity,
	}

	// For commit-level, delegate entirely to bazelisk
	if granularity == GranularityCommit {
		return b.runCommitBisect(ctx, result)
	}

	// For release/RC level, run native bisect
	return b.runNativeBisect(ctx, engine, result)
}

// runCommitBisect delegates to bazelisk for commit-level precision.
func (b *Bisector) runCommitBisect(ctx context.Context, result *Result) (*Result, error) {
	commitResult, err := RunBazeliskBisect(
		ctx,
		b.config.FromVersion,
		b.config.ToVersion,
		b.config.TestCommand,
		b.config.ProjectPath,
		nil, // onProgress
	)
	if err != nil {
		return nil, err
	}

	result.CommitResult = commitResult
	result.FirstBadVersion = commitResult.FirstBadCommit
	result.StepsTaken = commitResult.StepsTaken

	return result, nil
}

// runNativeBisect performs binary search using the selected engine.
func (b *Bisector) runNativeBisect(ctx context.Context, engine Engine, result *Result) (*Result, error) {
	// Get versions in range
	versions, err := engine.GetVersions(ctx, b.config.FromVersion, b.config.ToVersion)
	if err != nil {
		return nil, fmt.Errorf("get versions: %w", err)
	}

	if len(versions) < 2 {
		return nil, fmt.Errorf("need at least 2 versions to bisect, got %d", len(versions))
	}

	// Verify from version passes
	fromResult, err := engine.TestVersion(ctx, b.config.FromVersion, b.config.TestCommand, b.config.ProjectPath, b.config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("test from version: %w", err)
	}
	result.TestedVersions = append(result.TestedVersions, TestedVersion{
		Version:  fromResult.Version,
		Passed:   fromResult.Passed,
		Output:   fromResult.Output,
		Duration: fromResult.Duration,
	})
	result.StepsTaken++

	if !fromResult.Passed {
		return nil, fmt.Errorf("from version %s does not pass the test - cannot bisect", b.config.FromVersion)
	}

	// Verify to version fails (or find if it passes)
	toResult, err := engine.TestVersion(ctx, b.config.ToVersion, b.config.TestCommand, b.config.ProjectPath, b.config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("test to version: %w", err)
	}
	result.TestedVersions = append(result.TestedVersions, TestedVersion{
		Version:  toResult.Version,
		Passed:   toResult.Passed,
		Output:   toResult.Output,
		Duration: toResult.Duration,
	})
	result.StepsTaken++

	if toResult.Passed {
		// Target version passes - no breakage found
		result.LastGoodVersion = b.config.ToVersion
		return result, nil
	}

	// Binary search
	good := 0
	bad := len(versions) - 1

	for good+1 < bad && result.StepsTaken < b.config.MaxSteps {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, nil
		default:
		}

		mid := (good + bad) / 2
		testVer := versions[mid]

		testResult, err := engine.TestVersion(ctx, testVer, b.config.TestCommand, b.config.ProjectPath, b.config.Timeout)
		if err != nil {
			return nil, fmt.Errorf("test version %s: %w", testVer, err)
		}
		result.TestedVersions = append(result.TestedVersions, TestedVersion{
			Version:  testResult.Version,
			Passed:   testResult.Passed,
			Output:   testResult.Output,
			Duration: testResult.Duration,
		})
		result.StepsTaken++

		if b.config.OnProgress != nil {
			b.config.OnProgress(Progress{
				CurrentVersion: testVer,
				StepNumber:     result.StepsTaken,
				TotalSteps:     estimateSteps(len(versions)),
				Passed:         testResult.Passed,
				Output:         testResult.Output,
				Duration:       testResult.Duration,
			})
		}

		if testResult.Passed {
			good = mid
		} else {
			bad = mid
		}
	}

	result.FirstBadVersion = versions[bad]
	result.LastGoodVersion = versions[good]

	// Get flag changes between good and bad versions (only for release engine)
	if b.db != nil && result.IsNative {
		diff, err := b.db.Diff(result.LastGoodVersion, result.FirstBadVersion)
		if err == nil {
			result.FlagChanges = diff
		}
	}

	return result, nil
}

// estimateSteps estimates total steps for binary search.
func estimateSteps(n int) int {
	if n <= 1 {
		return 1
	}
	steps := 2 // Initial from/to tests
	for n > 1 {
		n = n / 2
		steps++
	}
	return steps
}

// FormatResult formats the bisect result for display.
func FormatResult(r *Result) string {
	var sb strings.Builder

	// Always show engine info for transparency
	if r.EngineName != "" {
		fmt.Fprintf(&sb, "Engine: %s\n", r.EngineName)
		fmt.Fprintf(&sb, "Granularity: %s\n", r.Granularity)
		if !r.IsNative {
			sb.WriteString("Note: This bisect delegates to bazelisk\n")
		}
		sb.WriteString("\n")
	}

	if r.Error != nil {
		fmt.Fprintf(&sb, "Bisect failed: %v\n", r.Error)
		return sb.String()
	}

	if r.FirstBadVersion == "" && r.LastGoodVersion != "" {
		fmt.Fprintf(&sb, "No breakage found! Version %s passes.\n", r.LastGoodVersion)
		return sb.String()
	}

	// Handle commit-level results differently
	if r.CommitResult != nil {
		fmt.Fprintf(&sb, "First bad commit: %s\n", r.CommitResult.FirstBadCommit)
		if r.CommitResult.CommitURL != "" {
			fmt.Fprintf(&sb, "View: %s\n", r.CommitResult.CommitURL)
		}
		fmt.Fprintf(&sb, "Steps taken: %d\n", r.CommitResult.StepsTaken)
		return sb.String()
	}

	fmt.Fprintf(&sb, "First bad version: %s\n", r.FirstBadVersion)
	fmt.Fprintf(&sb, "Last good version: %s\n", r.LastGoodVersion)
	fmt.Fprintf(&sb, "Steps taken: %d\n", r.StepsTaken)

	if r.FlagChanges != nil {
		sb.WriteString("\nFlag changes between versions:\n")

		if len(r.FlagChanges.Removed) > 0 {
			sb.WriteString("\n  Removed flags:\n")
			for _, f := range r.FlagChanges.Removed {
				fmt.Fprintf(&sb, "    - --%s\n", f.Name)
			}
		}

		if len(r.FlagChanges.Added) > 0 {
			sb.WriteString("\n  Added flags:\n")
			for _, f := range r.FlagChanges.Added {
				if f.IsIncompatibleChange() {
					fmt.Fprintf(&sb, "    + --%s (incompatible change)\n", f.Name)
				}
			}
		}

		// Show incompatible flag default changes
		var incompatibleChanges []flagdb.FlagChange
		for _, c := range r.FlagChanges.Changed {
			if c.Change == flagdb.ChangeDefaultValue && c.To.IsIncompatibleChange() {
				incompatibleChanges = append(incompatibleChanges, c)
			}
		}

		if len(incompatibleChanges) > 0 {
			sb.WriteString("\n  Incompatible flags with changed defaults:\n")
			for _, c := range incompatibleChanges {
				fmt.Fprintf(&sb, "    ! --%s: %s -> %s\n", c.Name, c.From.Default, c.To.Default)
			}
		}
	}

	return sb.String()
}

// FormatSuggestedNextSteps formats suggested next steps based on results.
func FormatSuggestedNextSteps(r *Result) string {
	var sb strings.Builder

	sb.WriteString("Suggested next steps:\n")

	// If we did release-level, suggest RC or commit level
	if r.Granularity == GranularityRelease && r.FirstBadVersion != "" && r.LastGoodVersion != "" {
		fmt.Fprintf(&sb, "  # For more precision, bisect at RC level:\n")
		fmt.Fprintf(&sb, "  bazelbump bisect --from %s --to %s --granularity=rc\n\n", r.LastGoodVersion, r.FirstBadVersion)

		fmt.Fprintf(&sb, "  # Or at commit level (requires bazelisk):\n")
		fmt.Fprintf(&sb, "  bazelbump bisect --from %s --to %s --granularity=commit\n\n", r.LastGoodVersion, r.FirstBadVersion)
	}

	// If we did RC-level, suggest commit level
	if r.Granularity == GranularityRC && r.FirstBadVersion != "" && r.LastGoodVersion != "" {
		fmt.Fprintf(&sb, "  # For commit-level precision:\n")
		fmt.Fprintf(&sb, "  bazelbump bisect --from %s --to %s --granularity=commit\n\n", r.LastGoodVersion, r.FirstBadVersion)
	}

	// Always suggest analyze and diff
	if r.FirstBadVersion != "" && r.LastGoodVersion != "" {
		fmt.Fprintf(&sb, "  # Analyze flag changes:\n")
		fmt.Fprintf(&sb, "  bazelbump analyze --from %s --to %s\n", r.LastGoodVersion, r.FirstBadVersion)
		fmt.Fprintf(&sb, "  bazelbump diff --from %s --to %s\n", r.LastGoodVersion, r.FirstBadVersion)
	}

	return sb.String()
}
