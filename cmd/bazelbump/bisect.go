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
	"github.com/albertocavalcante/bazelbump/internal/netconfig"
)

func runBisect(args []string) error {
	fs := flag.NewFlagSet("bisect", flag.ExitOnError)

	var (
		fromVersion    = fs.String("from", "", "Known-good Bazel version (required)")
		toVersion      = fs.String("to", "", "Known-bad or target Bazel version (required)")
		granularityStr = fs.String("granularity", "auto", "Bisect precision: auto, release, rc, commit")
		testCmd        = fs.String("test", "bazel build //...", "Test command to run at each version")
		projectPath    = fs.String("path", ".", "Project directory")
		timeout        = fs.Duration("timeout", 5*time.Minute, "Timeout for each test")
		skipPrerelease = fs.Bool("skip-prerelease", true, "Skip prerelease versions (release granularity only)")
		maxSteps       = fs.Int("max-steps", 50, "Maximum bisect iterations")
		verbose        = fs.Bool("v", false, "Verbose output")
		jsonOutput     = fs.Bool("json", false, "Output results as JSON")
		explain        = fs.Bool("explain", false, "Show what engine will be used without running")
		listVersions   = fs.Bool("list-versions", false, "List available versions for the granularity")
		// Network mode flags (air-gap aware)
		online  = fs.Bool("online", false, "Enable network access for GitHub API fallback")
		offline = fs.Bool("offline", false, "Force offline mode (no network access)")
	)

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Find the first Bazel version that breaks your build.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from <version> --to <version> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Granularity Levels:")
		fmt.Fprintln(os.Stderr, "  release  Bisect between release versions (7.0.0, 7.1.0)")
		fmt.Fprintln(os.Stderr, "           Engine: native (uses embedded flag database, works offline)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  rc       Bisect between release candidates (7.0.0-rc1, 7.0.0-rc2)")
		fmt.Fprintln(os.Stderr, "           Engine: native (fetches RC list from GitHub releases API)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  commit   Bisect between git commits (for maximum precision)")
		fmt.Fprintln(os.Stderr, "           Engine: bazelisk (delegated - uses bazelisk --bisect)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  auto     Auto-detect from version format (default)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Network Mode (air-gap support):")
		fmt.Fprintln(os.Stderr, "  Default: offline (uses only local flag database)")
		fmt.Fprintln(os.Stderr, "  --online    Enable GitHub API fallback for version validation")
		fmt.Fprintln(os.Stderr, "  --offline   Force offline mode (overrides BAZELBUMP_ONLINE)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Environment variables:")
		fmt.Fprintln(os.Stderr, "    BAZELBUMP_ONLINE=1   Enable network access")
		fmt.Fprintln(os.Stderr, "    BAZELBUMP_OFFLINE=1  Force offline (takes precedence)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  # Release-level bisect (fast, works offline)")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0 --to 8.0.0")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # Validate a new version via GitHub (online mode)")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 9.0.0 --to 9.1.0 --online")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # RC-level bisect (find which RC introduced breakage)")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0-rc1 --to 7.0.0 --granularity=rc")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # Commit-level bisect (maximum precision, uses bazelisk)")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0 --to 7.1.0 --granularity=commit")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # See what engine will be used without running")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --from 7.0.0 --to 8.0.0 --explain")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # List available versions")
		fmt.Fprintln(os.Stderr, "  bazelbump bisect --list-versions")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Prerequisites:")
		fmt.Fprintln(os.Stderr, "  - bazelisk must be installed for version switching")
		fmt.Fprintln(os.Stderr, "  - For commit-level: GITHUB_TOKEN recommended to avoid rate limiting")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate required flags (not required for --list-versions)
	if !*listVersions {
		if *fromVersion == "" {
			return fmt.Errorf("--from is required")
		}
		if *toVersion == "" {
			return fmt.Errorf("--to is required")
		}
	}

	// Validate mutually exclusive flags
	if *online && *offline {
		return fmt.Errorf("--online and --offline are mutually exclusive")
	}

	// Parse granularity
	granularity, err := bisect.ParseGranularity(*granularityStr)
	if err != nil {
		return err
	}

	// Auto-detect granularity if needed (for list-versions, default to release)
	if granularity == bisect.GranularityAuto {
		if *listVersions {
			granularity = bisect.GranularityRelease
		} else {
			granularity = bisect.DetectGranularity(*fromVersion, *toVersion)
		}
	}

	// Load flag database (needed for release engine and version validation)
	db, err := loadFlagDB()
	if err != nil {
		return fmt.Errorf("load flag database: %w", err)
	}

	// Handle --list-versions flag
	if *listVersions {
		versions, err := bisect.ListAvailableVersions(context.Background(), granularity, db)
		if err != nil {
			return err
		}
		fmt.Println(bisect.FormatAvailableVersions(versions))
		return nil
	}

	// Build network config (air-gap aware)
	netCfg := netconfig.Default()
	if *online {
		netCfg = netCfg.WithOnline()
	}
	if *offline {
		netCfg = netCfg.WithOffline()
	}

	// Validate versions exist and are in correct order
	validator := bisect.NewValidator(db, netCfg)
	warnings, err := validator.ValidateVersions(context.Background(), *fromVersion, *toVersion, granularity)
	if err != nil {
		return err
	}

	// Show warnings (e.g., "version exists but no flag data")
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Select engine to show info
	engine, err := bisect.SelectEngine(granularity, db)
	if err != nil {
		return fmt.Errorf("select engine: %w", err)
	}

	// Handle --explain flag
	if *explain {
		fmt.Printf("Bisect Configuration:\n")
		fmt.Printf("  From: %s\n", *fromVersion)
		fmt.Printf("  To: %s\n", *toVersion)
		fmt.Printf("  Granularity: %s\n", granularity)
		fmt.Printf("  Network mode: %s\n", netCfg.Mode)
		fmt.Println()
		fmt.Printf("Engine: %s\n", engine.Name())
		fmt.Printf("  %s\n", engine.Description())
		fmt.Printf("  Native: %t\n", engine.IsNative())
		fmt.Printf("  Requires network: %t\n", engine.RequiresNetwork())
		if !engine.IsNative() {
			fmt.Println()
			fmt.Println("Note: This granularity delegates to bazelisk for execution.")
		}
		return nil
	}

	// Check bazelisk is available
	if err := checkBazelisk(); err != nil {
		return err
	}

	// Create bisector config
	cfg := bisect.Config{
		FromVersion:    *fromVersion,
		ToVersion:      *toVersion,
		Granularity:    granularity,
		TestCommand:    *testCmd,
		ProjectPath:    *projectPath,
		Timeout:        *timeout,
		SkipPrerelease: *skipPrerelease,
		MaxSteps:       *maxSteps,
		Verbose:        *verbose,
	}

	if !*jsonOutput {
		cfg.OnProgress = func(p bisect.Progress) {
			status := color(colorGreen, "PASS")
			if !p.Passed {
				status = color(colorRed, "FAIL")
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
		fmt.Printf("Bisecting between %s and %s\n", *fromVersion, *toVersion)
		fmt.Printf("Engine: %s\n", engine.Name())
		fmt.Printf("Granularity: %s\n", granularity)
		fmt.Printf("Network mode: %s\n", netCfg.Mode)
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
	if result.FirstBadVersion != "" || (result.CommitResult != nil && result.CommitResult.FirstBadCommit != "") {
		fmt.Println()
		fmt.Print(bisect.FormatSuggestedNextSteps(result))
	}

	return nil
}

func checkBazelisk() error {
	// Check if bazelisk or bazel is available
	path, err := findExecutable("bazelisk")
	if err != nil {
		path, err = findExecutable("bazel")
		if err != nil {
			return fmt.Errorf("bazelisk not found: install from https://github.com/bazelbuild/bazelisk")
		}
	}
	_ = path // Used for verbose output if needed
	return nil
}

func findExecutable(name string) (string, error) {
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
	fmt.Println("{")
	fmt.Printf("  \"engine\": %q,\n", result.EngineName)
	fmt.Printf("  \"granularity\": %q,\n", result.Granularity.String())
	fmt.Printf("  \"is_native\": %t,\n", result.IsNative)
	fmt.Printf("  \"first_bad_version\": %q,\n", result.FirstBadVersion)
	fmt.Printf("  \"last_good_version\": %q,\n", result.LastGoodVersion)
	fmt.Printf("  \"steps_taken\": %d,\n", result.StepsTaken)

	if result.CommitResult != nil {
		fmt.Printf("  \"commit_result\": {\n")
		fmt.Printf("    \"first_bad_commit\": %q,\n", result.CommitResult.FirstBadCommit)
		fmt.Printf("    \"commit_url\": %q\n", result.CommitResult.CommitURL)
		fmt.Printf("  },\n")
	}

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
