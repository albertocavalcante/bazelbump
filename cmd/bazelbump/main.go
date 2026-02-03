// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Command bazelbump analyzes Bazel projects for upgrade compatibility.
//
// Usage:
//
//	bazelbump <command> [flags]
//
// Commands:
//
//	analyze   Analyze a project for Bazel upgrade issues
//	diff      Show flag changes between Bazel versions
//	flags     List flags for a Bazel version
//	version   Show bazelbump version
//
// Examples:
//
//	bazelbump analyze --to 9.0.0
//	bazelbump diff --from 8.0.0 --to 9.0.0
//	bazelbump flags --version 9.0.0 --incompatible
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/albertocavalcante/bazelbump/analysis"
	"github.com/albertocavalcante/bazelbump/flagdb"
)

// Version info set by build flags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// useColor returns true if output should be colorized
func useColor() bool {
	// Check NO_COLOR env var (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// Check if stdout is a terminal
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func color(c, s string) string {
	if useColor() {
		return c + s + colorReset
	}
	return s
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Global flags
	flag.Usage = printUsage

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "analyze":
		err = runAnalyze(args)
	case "diff":
		err = runDiff(args)
	case "flags":
		err = runFlags(args)
	case "version", "-v", "--version":
		runVersion()
		return
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", color(colorRed, "error:"), err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s - Analyze Bazel projects for upgrade compatibility

%s
  bazelbump <command> [flags]

%s
  analyze   Analyze a project for Bazel upgrade issues
  diff      Show flag changes between Bazel versions
  flags     List flags for a Bazel version
  version   Show bazelbump version

%s
  bazelbump analyze --to 9.0.0
  bazelbump analyze --from 8.0.0 --to 9.0.0 --path /path/to/project
  bazelbump diff --from 8.0.0 --to 9.0.0
  bazelbump flags --version 9.0.0 --incompatible

Use "bazelbump <command> --help" for more information about a command.
`, color(colorBold, "bazelbump"),
		color(colorBold, "Usage:"),
		color(colorBold, "Commands:"),
		color(colorBold, "Examples:"))
}

// loadFlagDB loads the flag database from the data directory
func loadFlagDB() (*flagdb.DB, error) {
	// Try multiple locations for the flag data
	candidates := []string{
		// Relative to current directory
		"data/flags",
		// Relative to executable
		filepath.Join(filepath.Dir(os.Args[0]), "..", "data", "flags"),
		filepath.Join(filepath.Dir(os.Args[0]), "data", "flags"),
	}

	// Also check BAZELBUMP_DATA_DIR env var
	if dataDir := os.Getenv("BAZELBUMP_DATA_DIR"); dataDir != "" {
		candidates = append([]string{dataDir}, candidates...)
	}

	for _, path := range candidates {
		db, err := flagdb.Load(path)
		if err == nil {
			return db, nil
		}
	}

	return nil, fmt.Errorf("could not find flag data directory; set BAZELBUMP_DATA_DIR or run from project root")
}

// --- analyze command ---

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Analyze a project for Bazel upgrade issues.

%s
  bazelbump analyze --to <version> [flags]

%s
  --to         Target Bazel version (required)
  --from       Source Bazel version (default: auto-detect or use --to minus one major)
  --path       Path to project directory (default: current directory)
  --json       Output results as JSON
  --help       Show this help

%s
  bazelbump analyze --to 9.0.0
  bazelbump analyze --from 8.0.0 --to 9.0.0 --path /path/to/project
  bazelbump analyze --to 9.0.0 --json
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	toVersion := fs.String("to", "", "Target Bazel version (required)")
	fromVersion := fs.String("from", "", "Source Bazel version")
	projectPath := fs.String("path", ".", "Path to project directory")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *toVersion == "" {
		return fmt.Errorf("--to version is required")
	}

	db, err := loadFlagDB()
	if err != nil {
		return err
	}

	// If from version not specified, try to find the previous major version
	if *fromVersion == "" {
		*fromVersion = findPreviousVersion(db, *toVersion)
		if *fromVersion == "" {
			return fmt.Errorf("could not determine source version; please specify --from")
		}
	}

	// Validate versions exist
	if !db.HasVersion(*fromVersion) {
		return fmt.Errorf("unknown from version: %s", *fromVersion)
	}
	if !db.HasVersion(*toVersion) {
		return fmt.Errorf("unknown to version: %s", *toVersion)
	}

	analyzer := analysis.New(
		analysis.WithProjectPath(*projectPath),
		analysis.WithFromVersion(*fromVersion),
		analysis.WithToVersion(*toVersion),
		analysis.WithFlagDB(db),
	)

	result, err := analyzer.Analyze()
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if *jsonOutput {
		return outputAnalysisJSON(result)
	}

	outputAnalysisText(result)

	if result.HasIssues() {
		os.Exit(1)
	}
	return nil
}

// findPreviousVersion finds a reasonable "from" version given a "to" version
func findPreviousVersion(db *flagdb.DB, toVersion string) string {
	versions := db.Versions()
	if len(versions) == 0 {
		return ""
	}

	// Find the target version index
	var toIdx int = -1
	for i, v := range versions {
		if v.String() == toVersion {
			toIdx = i
			break
		}
	}

	if toIdx < 0 {
		return ""
	}

	// Try to find a version with a lower major number
	targetMajor := versions[toIdx].Major()
	for i := toIdx - 1; i >= 0; i-- {
		if versions[i].Major() < targetMajor {
			// Find the highest version of that major
			for j := i; j >= 0; j-- {
				if versions[j].Major() == versions[i].Major() {
					// Find the last .0.0 version of this major
					if versions[j].Minor() == 0 && versions[j].Patch() == 0 {
						return versions[j].String()
					}
				}
			}
			return versions[i].String()
		}
	}

	// If no previous major, just return the first version
	if toIdx > 0 {
		return versions[0].String()
	}
	return ""
}

// outputAnalysisJSON outputs the analysis result as JSON
func outputAnalysisJSON(result *analysis.AnalysisResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// outputAnalysisText outputs the analysis result as human-readable text
func outputAnalysisText(result *analysis.AnalysisResult) {
	fmt.Printf("%s Analyzing upgrade from Bazel %s to %s\n",
		color(colorBlue, "==>"),
		color(colorCyan, result.FromVersion),
		color(colorCyan, result.ToVersion))
	fmt.Printf("    Project: %s\n", result.ProjectPath)
	fmt.Printf("    Files scanned: %d\n\n", result.FilesScanned)

	if !result.HasIssues() {
		fmt.Printf("%s No issues found!\n", color(colorGreen, "[OK]"))
		return
	}

	// Group issues by file
	byFile := result.IssuesByFile()

	// Sort files for deterministic output
	var files []string
	for f := range byFile {
		files = append(files, f)
	}
	slices.Sort(files)

	for _, file := range files {
		issues := byFile[file]
		fmt.Printf("%s\n", color(colorBold, file))

		for _, issue := range issues {
			issueColor := colorYellow
			issueLabel := "warning"
			switch issue.IssueType {
			case analysis.FlagRemoved:
				issueColor = colorRed
				issueLabel = "removed"
			case analysis.FlagDeprecated:
				issueColor = colorYellow
				issueLabel = "deprecated"
			case analysis.FlagDefaultChanged:
				issueColor = colorCyan
				issueLabel = "changed"
			}

			fmt.Printf("  %s:%d: %s --%s\n",
				file, issue.Line,
				color(issueColor, issueLabel),
				issue.FlagName)

			if issue.Suggestion != "" {
				fmt.Printf("    %s %s\n", color(colorBlue, "hint:"), issue.Suggestion)
			}
		}
		fmt.Println()
	}

	// Summary
	removed := result.IssuesByType(analysis.FlagRemoved)
	deprecated := result.IssuesByType(analysis.FlagDeprecated)
	changed := result.IssuesByType(analysis.FlagDefaultChanged)

	fmt.Printf("%s Found %d issue(s): ",
		color(colorYellow, "[WARN]"),
		len(result.FlagIssues))

	var parts []string
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", len(removed)))
	}
	if len(deprecated) > 0 {
		parts = append(parts, fmt.Sprintf("%d deprecated", len(deprecated)))
	}
	if len(changed) > 0 {
		parts = append(parts, fmt.Sprintf("%d changed defaults", len(changed)))
	}
	fmt.Println(strings.Join(parts, ", "))
}

// --- diff command ---

func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Show flag changes between Bazel versions.

%s
  bazelbump diff --from <version> --to <version> [flags]

%s
  --from       Source Bazel version (required)
  --to         Target Bazel version (required)
  --json       Output results as JSON
  --help       Show this help

%s
  bazelbump diff --from 8.0.0 --to 9.0.0
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	fromVersion := fs.String("from", "", "Source Bazel version (required)")
	toVersion := fs.String("to", "", "Target Bazel version (required)")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *fromVersion == "" {
		return fmt.Errorf("--from version is required")
	}
	if *toVersion == "" {
		return fmt.Errorf("--to version is required")
	}

	db, err := loadFlagDB()
	if err != nil {
		return err
	}

	// Validate versions exist
	if !db.HasVersion(*fromVersion) {
		return fmt.Errorf("unknown from version: %s", *fromVersion)
	}
	if !db.HasVersion(*toVersion) {
		return fmt.Errorf("unknown to version: %s", *toVersion)
	}

	diff, err := db.Diff(*fromVersion, *toVersion)
	if err != nil {
		return fmt.Errorf("diff failed: %w", err)
	}

	if *jsonOutput {
		return outputDiffJSON(diff)
	}

	outputDiffText(diff)
	return nil
}

// diffOutput is used for JSON serialization
type diffOutput struct {
	FromVersion string        `json:"from_version"`
	ToVersion   string        `json:"to_version"`
	Added       []flagOutput  `json:"added"`
	Removed     []flagOutput  `json:"removed"`
	Changed     []changeOutput `json:"changed"`
}

type flagOutput struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type changeOutput struct {
	Name       string `json:"name"`
	ChangeType string `json:"change_type"`
	OldDefault string `json:"old_default,omitempty"`
	NewDefault string `json:"new_default,omitempty"`
}

func outputDiffJSON(diff *flagdb.Diff) error {
	out := diffOutput{
		FromVersion: diff.FromVersion,
		ToVersion:   diff.ToVersion,
	}

	for _, f := range diff.Added {
		out.Added = append(out.Added, flagOutput{
			Name:        f.Name,
			Type:        f.Type,
			Default:     f.Default,
			Description: f.Description,
		})
	}

	for _, f := range diff.Removed {
		out.Removed = append(out.Removed, flagOutput{
			Name:        f.Name,
			Type:        f.Type,
			Default:     f.Default,
			Description: f.Description,
		})
	}

	for _, c := range diff.Changed {
		co := changeOutput{
			Name:       c.Name,
			ChangeType: c.Change.String(),
		}
		if c.From != nil && c.To != nil {
			co.OldDefault = c.From.Default
			co.NewDefault = c.To.Default
		}
		out.Changed = append(out.Changed, co)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputDiffText(diff *flagdb.Diff) {
	fmt.Printf("%s Flag changes from Bazel %s to %s\n\n",
		color(colorBlue, "==>"),
		color(colorCyan, diff.FromVersion),
		color(colorCyan, diff.ToVersion))

	// Removed flags
	if len(diff.Removed) > 0 {
		fmt.Printf("%s (%d)\n", color(colorRed+colorBold, "Removed flags"), len(diff.Removed))
		// Sort for deterministic output
		removed := make([]flagdb.Flag, len(diff.Removed))
		copy(removed, diff.Removed)
		slices.SortFunc(removed, func(a, b flagdb.Flag) int {
			return strings.Compare(a.Name, b.Name)
		})
		for _, f := range removed {
			fmt.Printf("  %s --%s\n", color(colorRed, "-"), f.Name)
		}
		fmt.Println()
	}

	// Added flags
	if len(diff.Added) > 0 {
		fmt.Printf("%s (%d)\n", color(colorGreen+colorBold, "Added flags"), len(diff.Added))
		// Sort for deterministic output
		added := make([]flagdb.Flag, len(diff.Added))
		copy(added, diff.Added)
		slices.SortFunc(added, func(a, b flagdb.Flag) int {
			return strings.Compare(a.Name, b.Name)
		})
		for _, f := range added {
			fmt.Printf("  %s --%s\n", color(colorGreen, "+"), f.Name)
		}
		fmt.Println()
	}

	// Changed flags
	if len(diff.Changed) > 0 {
		fmt.Printf("%s (%d)\n", color(colorYellow+colorBold, "Changed flags"), len(diff.Changed))
		// Sort for deterministic output
		changed := make([]flagdb.FlagChange, len(diff.Changed))
		copy(changed, diff.Changed)
		slices.SortFunc(changed, func(a, b flagdb.FlagChange) int {
			return strings.Compare(a.Name, b.Name)
		})
		for _, c := range changed {
			changeDesc := c.Change.String()
			if c.Change == flagdb.ChangeDefaultValue && c.From != nil && c.To != nil {
				changeDesc = fmt.Sprintf("%s -> %s", c.From.Default, c.To.Default)
			}
			fmt.Printf("  %s --%s: %s\n", color(colorYellow, "~"), c.Name, changeDesc)
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("Summary: %d added, %d removed, %d changed\n",
		len(diff.Added), len(diff.Removed), len(diff.Changed))
}

// --- flags command ---

func runFlags(args []string) error {
	fs := flag.NewFlagSet("flags", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `List flags for a Bazel version.

%s
  bazelbump flags --version <version> [flags]

%s
  --version      Bazel version (required)
  --deprecated   Show only deprecated flags
  --incompatible Show only incompatible_* flags
  --experimental Show only experimental_* flags
  --json         Output results as JSON
  --help         Show this help

%s
  bazelbump flags --version 9.0.0
  bazelbump flags --version 9.0.0 --incompatible
  bazelbump flags --version 9.0.0 --deprecated --json
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	version := fs.String("version", "", "Bazel version (required)")
	deprecated := fs.Bool("deprecated", false, "Show only deprecated flags")
	incompatible := fs.Bool("incompatible", false, "Show only incompatible_* flags")
	experimental := fs.Bool("experimental", false, "Show only experimental_* flags")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *version == "" {
		return fmt.Errorf("--version is required")
	}

	db, err := loadFlagDB()
	if err != nil {
		return err
	}

	if !db.HasVersion(*version) {
		return fmt.Errorf("unknown version: %s", *version)
	}

	var flags []flagdb.Flag

	// Get flags based on filters
	if *deprecated {
		flags, err = db.DeprecatedFlags(*version)
	} else if *incompatible {
		flags, err = db.IncompatibleFlags(*version)
	} else if *experimental {
		flags, err = db.ExperimentalFlags(*version)
	} else {
		flags, err = db.Flags(*version)
	}

	if err != nil {
		return fmt.Errorf("failed to get flags: %w", err)
	}

	if *jsonOutput {
		return outputFlagsJSON(flags)
	}

	outputFlagsText(*version, flags, *deprecated, *incompatible, *experimental)
	return nil
}

func outputFlagsJSON(flags []flagdb.Flag) error {
	var out []flagOutput
	for _, f := range flags {
		out = append(out, flagOutput{
			Name:        f.Name,
			Type:        f.Type,
			Default:     f.Default,
			Description: f.Description,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputFlagsText(version string, flags []flagdb.Flag, deprecated, incompatible, experimental bool) {
	filterDesc := ""
	if deprecated {
		filterDesc = " (deprecated)"
	} else if incompatible {
		filterDesc = " (incompatible)"
	} else if experimental {
		filterDesc = " (experimental)"
	}

	fmt.Printf("%s Flags for Bazel %s%s: %d total\n\n",
		color(colorBlue, "==>"),
		color(colorCyan, version),
		filterDesc,
		len(flags))

	// Sort flags for deterministic output
	slices.SortFunc(flags, func(a, b flagdb.Flag) int {
		return strings.Compare(a.Name, b.Name)
	})

	for _, f := range flags {
		nameColor := colorReset
		if f.IsDeprecated() {
			nameColor = colorYellow
		} else if f.IsIncompatibleChange() {
			nameColor = colorRed
		} else if f.IsExperimental() {
			nameColor = colorCyan
		}

		fmt.Printf("  %s", color(nameColor, "--"+f.Name))
		if f.Type != "" {
			fmt.Printf(" (%s)", f.Type)
		}
		if f.Default != "" {
			fmt.Printf(" = %s", f.Default)
		}
		fmt.Println()

		if f.Description != "" {
			// Truncate long descriptions
			desc := f.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			fmt.Printf("      %s\n", desc)
		}
	}
}

// --- version command ---

func runVersion() {
	fmt.Printf("bazelbump %s\n", Version)
	if Commit != "unknown" {
		fmt.Printf("  commit: %s\n", Commit)
	}
	if BuildDate != "unknown" {
		fmt.Printf("  built:  %s\n", BuildDate)
	}
	fmt.Printf("  go:     %s\n", runtime.Version())
}
