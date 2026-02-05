// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package bisect provides binary search functionality to find
// the first Bazel version that breaks a build.
package bisect

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/albertocavalcante/bazelbump/flagdb"
	"github.com/albertocavalcante/bazelbump/internal/version"
)

// Config configures a bisect operation.
type Config struct {
	// FromVersion is the known-good version (required).
	FromVersion string

	// ToVersion is the known-bad or target version (required).
	ToVersion string

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
	result := &Result{}

	// Get versions in range
	versions, err := b.getVersionsInRange()
	if err != nil {
		return nil, fmt.Errorf("get versions: %w", err)
	}

	if len(versions) < 2 {
		return nil, fmt.Errorf("need at least 2 versions to bisect, got %d", len(versions))
	}

	// Verify from version passes
	fromResult := b.testVersion(ctx, b.config.FromVersion)
	result.TestedVersions = append(result.TestedVersions, fromResult)
	result.StepsTaken++

	if !fromResult.Passed {
		return nil, fmt.Errorf("from version %s does not pass the test - cannot bisect", b.config.FromVersion)
	}

	// Verify to version fails (or find if it passes)
	toResult := b.testVersion(ctx, b.config.ToVersion)
	result.TestedVersions = append(result.TestedVersions, toResult)
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

		testResult := b.testVersion(ctx, testVer)
		result.TestedVersions = append(result.TestedVersions, testResult)
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

	// Get flag changes between good and bad versions
	if b.db != nil {
		diff, err := b.db.Diff(result.LastGoodVersion, result.FirstBadVersion)
		if err == nil {
			result.FlagChanges = diff
		}
	}

	return result, nil
}

// getVersionsInRange returns all versions between from and to (inclusive).
func (b *Bisector) getVersionsInRange() ([]string, error) {
	allVersions := b.db.Versions()

	fromVer, err := version.Parse(b.config.FromVersion)
	if err != nil {
		return nil, fmt.Errorf("parse from version: %w", err)
	}

	toVer, err := version.Parse(b.config.ToVersion)
	if err != nil {
		return nil, fmt.Errorf("parse to version: %w", err)
	}

	var result []string
	for _, v := range allVersions {
		if v.Compare(fromVer) < 0 || v.Compare(toVer) > 0 {
			continue
		}

		if b.config.SkipPrerelease && v.IsPrerelease() {
			continue
		}

		result = append(result, v.String())
	}

	return result, nil
}

// testVersion tests a specific Bazel version.
func (b *Bisector) testVersion(ctx context.Context, ver string) TestedVersion {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, "bash", "-c", b.config.TestCommand)
	cmd.Dir = b.config.ProjectPath

	// Set USE_BAZEL_VERSION for bazelisk
	cmd.Env = append(os.Environ(), "USE_BAZEL_VERSION="+ver)

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	passed := err == nil

	return TestedVersion{
		Version:  ver,
		Passed:   passed,
		Output:   string(output),
		Duration: duration,
	}
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

	if r.Error != nil {
		fmt.Fprintf(&sb, "Bisect failed: %v\n", r.Error)
		return sb.String()
	}

	if r.FirstBadVersion == "" {
		fmt.Fprintf(&sb, "No breakage found! Version %s passes.\n", r.LastGoodVersion)
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
