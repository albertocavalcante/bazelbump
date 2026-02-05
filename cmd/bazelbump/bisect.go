// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/albertocavalcante/bazelbump/bisect"
)

func runBisect(args []string) error {
	fs := flag.NewFlagSet("bisect", flag.ExitOnError)

	var (
		fromVersion    = fs.String("from", "", "Known-good Bazel version (required)")
		toVersion      = fs.String("to", "", "Known-bad or target Bazel version (required)")
		testCmd        = fs.String("test", "bazel build //...", "Test command to run at each version")
		projectPath    = fs.String("path", ".", "Project directory")
		timeout        = fs.Duration("timeout", 5*time.Minute, "Timeout for each test")
		skipPrerelease = fs.Bool("skip-prerelease", true, "Skip prerelease versions")
		maxSteps       = fs.Int("max-steps", 50, "Maximum bisect iterations")
		verbose        = fs.Bool("v", false, "Verbose output")
		jsonOutput     = fs.Bool("json", false, "Output results as JSON")
	)

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Find the first Bazel version that breaks your build.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from <version> --to <version> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  # Find which version between 7.0.0 and 8.0.0 breaks the build")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0 --to 8.0.0")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # Use a custom test command")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0 --to 8.0.0 --test \"bazel test //...\"")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # Include prerelease versions")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0 --to 8.0.0 --skip-prerelease=false")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Notes:")
		fmt.Fprintln(os.Stderr, "  - Requires bazelisk to be installed for version switching")
		fmt.Fprintln(os.Stderr, "  - The --from version must pass the test command")
		fmt.Fprintln(os.Stderr, "  - Uses binary search to minimize the number of test runs")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate required flags
	if *fromVersion == "" {
		return fmt.Errorf("--from is required")
	}
	if *toVersion == "" {
		return fmt.Errorf("--to is required")
	}

	// Load flag database
	db, err := loadFlagDB()
	if err != nil {
		return fmt.Errorf("load flag database: %w", err)
	}

	// Validate versions exist in database
	if !db.HasVersion(*fromVersion) {
		return fmt.Errorf("unknown version: %s (use 'bazelbump flags --list-versions' to see available versions)", *fromVersion)
	}
	if !db.HasVersion(*toVersion) {
		return fmt.Errorf("unknown version: %s (use 'bazelbump flags --list-versions' to see available versions)", *toVersion)
	}

	// Check bazelisk is available
	if err := checkBazelisk(); err != nil {
		return err
	}

	// Create bisector
	cfg := bisect.Config{
		FromVersion:    *fromVersion,
		ToVersion:      *toVersion,
		TestCommand:    *testCmd,
		ProjectPath:    *projectPath,
		Timeout:        *timeout,
		SkipPrerelease: *skipPrerelease,
		MaxSteps:       *maxSteps,
		Verbose:        *verbose,
	}

	if !*jsonOutput {
		cfg.OnProgress = func(p bisect.Progress) {
			status := "PASS"
			if !p.Passed {
				status = "FAIL"
			}
			fmt.Printf("Testing %s... %s (%s)\n", p.CurrentVersion, status, p.Duration.Round(time.Millisecond))
			if *verbose && !p.Passed {
				fmt.Printf("  Output: %s\n", truncate(p.Output, 200))
			}
		}
	}

	b := bisect.New(db, cfg)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, stopping bisect...")
		cancel()
	}()

	// Run bisect
	if !*jsonOutput {
		fmt.Printf("Bisecting between %s and %s...\n", *fromVersion, *toVersion)
		fmt.Printf("Test command: %s\n", *testCmd)
		fmt.Println()
	}

	result, err := b.Run(ctx)
	if err != nil {
		return fmt.Errorf("bisect failed: %w", err)
	}

	// Output results
	if *jsonOutput {
		return outputBisectJSON(result)
	}

	fmt.Println()
	fmt.Print(bisect.FormatResult(result))

	// Suggest next steps
	if result.FirstBadVersion != "" {
		fmt.Println()
		fmt.Println("Suggested next steps:")
		fmt.Printf("  1. bazelbump analyze --from %s --to %s\n", result.LastGoodVersion, result.FirstBadVersion)
		fmt.Printf("  2. bazelbump diff --from %s --to %s\n", result.LastGoodVersion, result.FirstBadVersion)
	}

	return nil
}

func checkBazelisk() error {
	// Check if bazelisk or bazel is available
	// bazelisk responds to USE_BAZEL_VERSION env var
	path, err := findExecutable("bazelisk")
	if err != nil {
		path, err = findExecutable("bazel")
		if err != nil {
			return fmt.Errorf("bazelisk not found: install from https://github.com/bazelbuild/bazelisk")
		}
	}

	if verbose {
		fmt.Printf("Using: %s\n", path)
	}

	return nil
}

func findExecutable(name string) (string, error) {
	// Simple PATH lookup
	pathEnv := os.Getenv("PATH")
	paths := splitPath(pathEnv)

	for _, dir := range paths {
		full := dir + "/" + name
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}

	return "", fmt.Errorf("%s not found in PATH", name)
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return splitPathSeparator(path)
}

func splitPathSeparator(path string) []string {
	// Unix uses :
	sep := ":"
	var result []string
	start := 0
	for i := 0; i < len(path); i++ {
		if string(path[i]) == sep {
			if i > start {
				result = append(result, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

func outputBisectJSON(result *bisect.Result) error {
	// Simple JSON output
	fmt.Println("{")
	fmt.Printf("  \"first_bad_version\": %q,\n", result.FirstBadVersion)
	fmt.Printf("  \"last_good_version\": %q,\n", result.LastGoodVersion)
	fmt.Printf("  \"steps_taken\": %d,\n", result.StepsTaken)

	fmt.Println("  \"tested_versions\": [")
	for i, tv := range result.TestedVersions {
		comma := ","
		if i == len(result.TestedVersions)-1 {
			comma = ""
		}
		fmt.Printf("    {\"version\": %q, \"passed\": %t, \"duration_ms\": %d}%s\n",
			tv.Version, tv.Passed, tv.Duration.Milliseconds(), comma)
	}
	fmt.Println("  ]")

	fmt.Println("}")
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var verbose bool

func printBisectUsage() {
	fmt.Println(`Find the first Bazel version that breaks your build.

Usage:
  bazelbump bisect --from <version> --to <version> [flags]

Use "bazelbump bisect --help" for more information.`)
}
