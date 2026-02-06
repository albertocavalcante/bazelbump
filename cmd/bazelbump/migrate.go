// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// incompatibleFlagPattern matches --incompatible_* flags in migrate output.
var incompatibleFlagPattern = regexp.MustCompile(`^\s*(--incompatible_\w+)`)

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)

	var (
		version     = fs.String("version", "", "Bazel version to test (required)")
		projectPath = fs.String("path", ".", "Project directory")
		testCmd     = fs.String("test", "build //...", "Bazel command to test (without 'bazel' prefix)")
		jsonOutput  = fs.Bool("json", false, "Output results as JSON")
		verbose     = fs.Bool("v", false, "Verbose output")
	)

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Test incompatible flags to find which ones break your build.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "This command uses bazelisk's --migrate feature to test each")
		fmt.Fprintln(os.Stderr, "--incompatible_* flag individually and report which pass/fail.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  bazelbump migrate --version <version> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  # Test all incompatible flags for Bazel 8.0.0")
		fmt.Fprintln(os.Stderr, "  bazelbump migrate --version 8.0.0")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # Use a custom test command")
		fmt.Fprintln(os.Stderr, "  bazelbump migrate --version 8.0.0 --test \"test //...\"")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Prerequisites:")
		fmt.Fprintln(os.Stderr, "  - bazelisk must be installed")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *version == "" {
		return fmt.Errorf("--version is required")
	}

	// Check bazelisk is available
	if err := checkBazelisk(); err != nil {
		return err
	}

	if !*jsonOutput {
		fmt.Printf("Testing incompatible flags for Bazel %s...\n", *version)
		fmt.Printf("Test command: bazel %s\n", *testCmd)
		fmt.Println()
	}

	// Run bazelisk --migrate
	result, err := runBazeliskMigrate(*version, *testCmd, *projectPath, *verbose)
	if err != nil {
		return fmt.Errorf("migrate failed: %w", err)
	}

	// Output results
	if *jsonOutput {
		return outputMigrateJSON(result)
	}

	outputMigrateText(result, *version)
	return nil
}

// MigrateResult contains the results of flag testing.
type MigrateResult struct {
	PassingFlags []string
	FailingFlags []string
	RawOutput    string
}

func runBazeliskMigrate(version, testCmd, projectPath string, verbose bool) (*MigrateResult, error) {
	// Build the command: bazelisk --migrate <testCmd>
	args := append([]string{"--migrate"}, strings.Fields(testCmd)...)

	cmd := exec.Command("bazelisk", args...)
	cmd.Dir = projectPath
	cmd.Env = append(os.Environ(), "USE_BAZEL_VERSION="+version)

	if verbose {
		fmt.Printf("Running: USE_BAZEL_VERSION=%s bazelisk %s\n", version, strings.Join(args, " "))
	}

	output, err := cmd.CombinedOutput()
	rawOutput := string(output)

	if verbose {
		fmt.Println("Raw output:")
		fmt.Println(rawOutput)
		fmt.Println()
	}

	// Parse the output to extract pass/fail flags
	result := parseMigrateOutput(rawOutput)
	result.RawOutput = rawOutput

	// Note: bazelisk --migrate may exit non-zero if some flags fail,
	// but we still want to report the results
	if err != nil && result.PassingFlags == nil && result.FailingFlags == nil {
		return nil, fmt.Errorf("bazelisk failed: %w\nOutput: %s", err, rawOutput)
	}

	return result, nil
}

func parseMigrateOutput(output string) *MigrateResult {
	result := &MigrateResult{}

	// Bazelisk --migrate output format:
	// --- Running with the following flags: ---
	// --incompatible_foo
	// --incompatible_bar
	// --- Migration result ---
	// Flags that succeeded:
	//   --incompatible_foo
	// Flags that failed:
	//   --incompatible_bar

	scanner := bufio.NewScanner(strings.NewReader(output))
	inSucceeded := false
	inFailed := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "Flags that succeeded") || strings.Contains(line, "succeeded:") {
			inSucceeded = true
			inFailed = false
			continue
		}
		if strings.Contains(line, "Flags that failed") || strings.Contains(line, "failed:") {
			inSucceeded = false
			inFailed = true
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "===") {
			inSucceeded = false
			inFailed = false
			continue
		}

		if matches := incompatibleFlagPattern.FindStringSubmatch(line); matches != nil {
			flag := matches[1]
			if inSucceeded {
				result.PassingFlags = append(result.PassingFlags, flag)
			} else if inFailed {
				result.FailingFlags = append(result.FailingFlags, flag)
			}
		}
	}

	return result
}

func outputMigrateText(result *MigrateResult, version string) {
	if len(result.PassingFlags) > 0 {
		fmt.Printf("%s Flags that PASS (safe to enable):\n", color(colorGreen, "✓"))
		for _, f := range result.PassingFlags {
			fmt.Printf("  %s %s\n", color(colorGreen, "✓"), f)
		}
		fmt.Println()
	}

	if len(result.FailingFlags) > 0 {
		fmt.Printf("%s Flags that FAIL (need fixes):\n", color(colorRed, "✗"))
		for _, f := range result.FailingFlags {
			fmt.Printf("  %s %s\n", color(colorRed, "✗"), f)
		}
		fmt.Println()
	}

	// Suggest .bazelrc additions
	if len(result.PassingFlags) > 0 || len(result.FailingFlags) > 0 {
		fmt.Println("Suggested .bazelrc additions:")
		fmt.Println()

		if len(result.PassingFlags) > 0 {
			fmt.Println("# Enable passing flags (ready to migrate)")
			for _, f := range result.PassingFlags {
				fmt.Printf("build %s\n", f)
			}
			fmt.Println()
		}

		if len(result.FailingFlags) > 0 {
			fmt.Println("# Disable failing flags until code is fixed")
			for _, f := range result.FailingFlags {
				// Convert --incompatible_foo to --noincompatible_foo
				noFlag := strings.Replace(f, "--incompatible_", "--noincompatible_", 1)
				fmt.Printf("build %s\n", noFlag)
			}
		}
	}

	if len(result.PassingFlags) == 0 && len(result.FailingFlags) == 0 {
		fmt.Println("No incompatible flags detected or tested.")
		fmt.Println()
		fmt.Println("This might mean:")
		fmt.Println("  - The Bazel version has no incompatible flags")
		fmt.Println("  - bazelisk --migrate couldn't discover flags")
		fmt.Println("  - The output format changed")
		fmt.Println()
		fmt.Println("Raw output:")
		fmt.Println(result.RawOutput)
	}
}

func outputMigrateJSON(result *MigrateResult) error {
	fmt.Println("{")
	fmt.Println("  \"passing_flags\": [")
	for i, f := range result.PassingFlags {
		comma := ","
		if i == len(result.PassingFlags)-1 {
			comma = ""
		}
		fmt.Printf("    %q%s\n", f, comma)
	}
	fmt.Println("  ],")
	fmt.Println("  \"failing_flags\": [")
	for i, f := range result.FailingFlags {
		comma := ","
		if i == len(result.FailingFlags)-1 {
			comma = ""
		}
		fmt.Printf("    %q%s\n", f, comma)
	}
	fmt.Println("  ]")
	fmt.Println("}")
	return nil
}

func printMigrateUsage() {
	fmt.Println(`Test incompatible flags to find which ones break your build.

Usage:
  bazelbump migrate --version <version> [flags]

Use "bazelbump migrate --help" for more information.`)
}
