// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	recipestar "github.com/albertocavalcante/bazelbump/recipe/starlark"
	"github.com/albertocavalcante/bazelbump/recipe/starlark/testprelude"
)

// testOptions holds the options for the test command.
type testOptions struct {
	projectPath  string
	coverage     bool
	coverprofile string
	jsonOutput   bool
	junitOutput  bool
	verbose      bool
	filter       string
	timeout      time.Duration
}

// testResult holds the result of running tests.
type testResult struct {
	TotalTests  int      `json:"total_tests"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	Skipped     int      `json:"skipped"`
	Duration    string   `json:"duration"`
	TestFiles   []string `json:"test_files"`
	Coverage    float64  `json:"coverage,omitempty"`
	Command     string   `json:"command,omitempty"`
	ErrorOutput string   `json:"error_output,omitempty"`
}

func runRecipeTest(args []string) error {
	fs := flag.NewFlagSet("recipe test", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Run tests for Starlark recipes.

Discovers and runs test files (*_test.star, test_*.star) using skytest.
Tests can verify recipe logic, operations, and behavior.

%s
  bazelbump recipe test [flags] [recipe|path...]

%s
  --path           Project directory (default: ".")
  --coverage       Enable coverage collection
  --coverprofile   Write coverage to file
  --json           JSON output
  --junit          JUnit XML output
  -v, --verbose    Verbose output
  --filter         Run tests matching pattern
  --timeout        Test timeout (default: "5m")
  --help           Show this help

%s
  bazelbump recipe test                    # Run all discovered tests
  bazelbump recipe test my-recipe          # Run tests for specific recipe
  bazelbump recipe test ./path/test.star   # Run specific test file
  bazelbump recipe test --coverage         # Run with coverage
  bazelbump recipe test --filter "test_*"  # Run tests matching pattern

%s
  Test files should be named *_test.star or test_*.star and placed
  alongside recipes in:
    .bazelbump/recipes/
    recipes/
    ~/.config/bazelbump/recipes/

%s
  The skytest CLI (https://github.com/albertocavalcante/sky) is required.
  Install with: go install github.com/albertocavalcante/sky/cmd/skytest@latest
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"),
			color(colorBold, "Test Discovery:"),
			color(colorBold, "Requirements:"))
	}

	projectPath := fs.String("path", ".", "Project directory")
	coverage := fs.Bool("coverage", false, "Enable coverage collection")
	coverprofile := fs.String("coverprofile", "", "Write coverage to file")
	jsonOutput := fs.Bool("json", false, "JSON output")
	junitOutput := fs.Bool("junit", false, "JUnit XML output")
	verbose := fs.Bool("v", false, "Verbose output")
	fs.BoolVar(verbose, "verbose", false, "Verbose output")
	filter := fs.String("filter", "", "Run tests matching pattern")
	timeoutStr := fs.String("timeout", "5m", "Test timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Parse timeout
	timeout, err := time.ParseDuration(*timeoutStr)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	opts := testOptions{
		projectPath:  *projectPath,
		coverage:     *coverage,
		coverprofile: *coverprofile,
		jsonOutput:   *jsonOutput,
		junitOutput:  *junitOutput,
		verbose:      *verbose,
		filter:       *filter,
		timeout:      timeout,
	}

	// Check if sk is installed
	if err := checkSkInstalled(); err != nil {
		return err
	}

	// Discover or resolve test files
	var testFiles []string
	if fs.NArg() > 0 {
		// Explicit paths/recipes provided
		testFiles, err = resolveTestFiles(fs.Args(), opts.projectPath)
		if err != nil {
			return err
		}
	} else {
		// Discover all test files
		testFiles, err = discoverTestFilesForTest(opts)
		if err != nil {
			return err
		}
	}

	if len(testFiles) == 0 {
		if opts.jsonOutput {
			result := testResult{
				TotalTests: 0,
				Passed:     0,
				Failed:     0,
				Skipped:    0,
				Duration:   "0s",
				TestFiles:  []string{},
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("%s No test files found.\n\n", color(colorYellow, "[INFO]"))
		fmt.Println("Test files should be named *_test.star or test_*.star")
		fmt.Println("and placed in:")
		for _, p := range recipestar.GetDiscoveryPaths(opts.projectPath) {
			fmt.Printf("  - %s\n", p)
		}
		return nil
	}

	// Generate prelude file
	preludePath, cleanup, err := generatePreludeFile()
	if err != nil {
		return fmt.Errorf("failed to generate prelude: %w", err)
	}
	defer cleanup()

	// Run tests with sk
	result, err := runSkTest(testFiles, preludePath, opts)
	if err != nil {
		return err
	}

	// Output results
	if opts.jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Text output
	outputTestResults(result, opts)

	if result.Failed > 0 {
		return fmt.Errorf("%d test(s) failed", result.Failed)
	}

	return nil
}

// checkSkInstalled verifies that the sk/skytest CLI is available.
func checkSkInstalled() error {
	// Try skytest first (newer name), then sk
	if _, err := exec.LookPath("skytest"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("sk"); err == nil {
		return nil
	}
	return fmt.Errorf(`skytest CLI not found

The 'skytest' CLI is required to run Starlark tests.
Install it with:

  go install github.com/albertocavalcante/sky/cmd/skytest@latest

Or build from source: https://github.com/albertocavalcante/sky`)
}

// getSkCommand returns the skytest command name (skytest or sk).
func getSkCommand() string {
	if _, err := exec.LookPath("skytest"); err == nil {
		return "skytest"
	}
	return "sk"
}

// discoverTestFilesForTest discovers test files based on options.
func discoverTestFilesForTest(opts testOptions) ([]string, error) {
	tests, err := recipestar.DiscoverTestFiles(recipestar.TestDiscoveryOptions{
		ProjectPath: opts.projectPath,
	})
	if err != nil {
		return nil, err
	}

	var files []string
	for _, t := range tests {
		// Apply filter if specified
		if opts.filter != "" {
			matched, err := filepath.Match(opts.filter, filepath.Base(t.Path))
			if err != nil {
				return nil, fmt.Errorf("invalid filter pattern: %w", err)
			}
			if !matched {
				continue
			}
		}
		files = append(files, t.Path)
	}

	return files, nil
}

// resolveTestFiles resolves the provided arguments to test file paths.
func resolveTestFiles(args []string, projectPath string) ([]string, error) {
	var testFiles []string

	for _, arg := range args {
		// Check if it's an explicit path
		if strings.HasSuffix(arg, ".star") || strings.Contains(arg, "/") {
			// Try to resolve as a direct path
			var absPath string
			if filepath.IsAbs(arg) {
				absPath = arg
			} else {
				var err error
				absPath, err = filepath.Abs(filepath.Join(projectPath, arg))
				if err != nil {
					return nil, err
				}
			}

			// Check if file exists
			if _, err := os.Stat(absPath); err != nil {
				if os.IsNotExist(err) {
					return nil, fmt.Errorf("test file not found: %s", arg)
				}
				return nil, err
			}

			testFiles = append(testFiles, absPath)
			continue
		}

		// Try to find tests for a recipe by name
		recipe, err := recipestar.ResolveRecipe(arg, projectPath)
		if err != nil {
			// Not a recipe, try as a test name pattern
			tests, discErr := recipestar.DiscoverTestFiles(recipestar.TestDiscoveryOptions{
				ProjectPath: projectPath,
			})
			if discErr != nil {
				return nil, err // Return original error
			}

			found := false
			for _, t := range tests {
				if t.Name == arg || t.RecipeName == arg {
					testFiles = append(testFiles, t.Path)
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("no tests found for: %s", arg)
			}
			continue
		}

		// Find tests for this recipe
		recipeTests, err := recipestar.DiscoverTestsForRecipe(recipe.Path)
		if err != nil {
			return nil, err
		}

		if len(recipeTests) == 0 {
			return nil, fmt.Errorf("no tests found for recipe: %s", arg)
		}

		for _, t := range recipeTests {
			testFiles = append(testFiles, t.Path)
		}
	}

	return testFiles, nil
}

// generatePreludeFile creates a temporary prelude file for tests.
func generatePreludeFile() (string, func(), error) {
	return testprelude.GenerateToFile()
}

// runSkTest executes skytest with the given test files.
func runSkTest(testFiles []string, preludePath string, opts testOptions) (*testResult, error) {
	skCmd := getSkCommand()

	// Build command args
	var args []string

	// Add prelude
	args = append(args, "--prelude", preludePath)

	// Add options
	if opts.coverage {
		args = append(args, "-coverage")
	}
	if opts.coverprofile != "" {
		args = append(args, "-coverprofile", opts.coverprofile)
	}
	if opts.verbose {
		args = append(args, "-v")
	}
	if opts.junitOutput {
		args = append(args, "-junit")
	}
	if opts.filter != "" {
		args = append(args, "-k", opts.filter)
	}
	if opts.timeout > 0 {
		args = append(args, "-timeout", opts.timeout.String())
	}

	// Add test files
	args = append(args, testFiles...)

	// Create command
	cmd := exec.Command(skCmd, args...)
	cmd.Dir = opts.projectPath

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set timeout context
	startTime := time.Now()

	// Run command
	err := cmd.Run()
	duration := time.Since(startTime)

	// Parse results from output
	result := &testResult{
		TestFiles:   testFiles,
		TotalTests:  len(testFiles),
		Duration:    duration.Round(time.Millisecond).String(),
		Command:     skCmd + " " + strings.Join(args, " "),
		ErrorOutput: stderr.String(),
	}

	// Parse sk output to get pass/fail counts
	// sk output format varies, so we do basic parsing
	output := stdout.String()
	result.Passed = strings.Count(output, "PASS")
	result.Failed = strings.Count(output, "FAIL")
	result.Skipped = strings.Count(output, "SKIP")

	// If we couldn't parse counts, estimate from exit code
	if result.Passed == 0 && result.Failed == 0 {
		if err == nil {
			result.Passed = len(testFiles)
		} else {
			result.Failed = len(testFiles)
		}
	}

	result.TotalTests = result.Passed + result.Failed + result.Skipped

	// Don't return error for test failures - that's handled by result.Failed
	if err != nil {
		// Check if it's a timeout
		if strings.Contains(err.Error(), "killed") || strings.Contains(err.Error(), "timeout") {
			result.ErrorOutput = fmt.Sprintf("test timed out after %s", opts.timeout)
		}
	}

	return result, nil
}

// outputTestResults prints test results to stdout.
func outputTestResults(result *testResult, opts testOptions) {
	if opts.verbose {
		fmt.Printf("%s Running %d test file(s)\n", color(colorBlue, "==>"), len(result.TestFiles))
		for _, f := range result.TestFiles {
			fmt.Printf("    %s\n", f)
		}
		fmt.Println()
	}

	// Summary
	if result.Failed > 0 {
		fmt.Printf("%s Tests: %d passed, %s, %d skipped (%s)\n",
			color(colorRed, "[FAIL]"),
			result.Passed,
			color(colorRed, fmt.Sprintf("%d failed", result.Failed)),
			result.Skipped,
			result.Duration)
	} else if result.Passed > 0 {
		fmt.Printf("%s Tests: %s, %d skipped (%s)\n",
			color(colorGreen, "[PASS]"),
			color(colorGreen, fmt.Sprintf("%d passed", result.Passed)),
			result.Skipped,
			result.Duration)
	} else {
		fmt.Printf("%s No tests executed (%s)\n",
			color(colorYellow, "[INFO]"),
			result.Duration)
	}

	// Show error output if present and verbose
	if opts.verbose && result.ErrorOutput != "" {
		fmt.Printf("\n%s\n", color(colorRed, "Error output:"))
		fmt.Println(result.ErrorOutput)
	}

	// Coverage info
	if opts.coverage && result.Coverage > 0 {
		fmt.Printf("\nCoverage: %.1f%%\n", result.Coverage)
	}
	if opts.coverprofile != "" {
		fmt.Printf("Coverage profile written to: %s\n", opts.coverprofile)
	}
}
