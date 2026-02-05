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
//	apply     Apply transforms to fix detected flag issues
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
	"time"

	"github.com/albertocavalcante/bazelbump/analysis"
	"github.com/albertocavalcante/bazelbump/flagdb"
	"github.com/albertocavalcante/bazelbump/pkg/state"
	"github.com/albertocavalcante/bazelbump/recipe"
	recipestar "github.com/albertocavalcante/bazelbump/recipe/starlark"
	"github.com/albertocavalcante/bazelbump/transform"
	"github.com/albertocavalcante/go-bcr"
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
	case "apply":
		err = runApply(args)
	case "recipe":
		err = runRecipe(args)
	case "status":
		err = runStatus(args)
	case "history":
		err = runHistory(args)
	case "checkpoint":
		err = runCheckpoint(args)
	case "undo":
		err = runUndo(args)
	case "snapshot":
		err = runSnapshot(args)
	case "bisect":
		err = runBisect(args)
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
  analyze     Analyze a project for Bazel upgrade issues
  apply       Apply transforms to fix detected flag issues
  bisect      Find the first Bazel version that breaks your build
  diff        Show flag changes between Bazel versions
  flags       List flags for a Bazel version
  recipe      Run migration recipes
  status      Show status of last bazelbump run
  history     Show run history
  checkpoint  Manage checkpoints for resumable runs
  undo        Undo the last bazelbump run
  snapshot    Manage snapshots (for non-git directories)
  version     Show bazelbump version

%s
  bazelbump analyze --to 9.0.0
  bazelbump analyze --from 8.0.0 --to 9.0.0 --path /path/to/project
  bazelbump apply --to 9.0.0 --dry-run
  bazelbump bisect --from 7.0.0 --to 8.0.0
  bazelbump diff --from 8.0.0 --to 9.0.0
  bazelbump flags --version 9.0.0 --incompatible
  bazelbump recipe list
  bazelbump recipe run autoload-migration --dry-run

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

// --- apply command ---

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Apply transforms to fix detected flag issues.

%s
  bazelbump apply --to <version> [flags]

%s
  --to         Target Bazel version (required)
  --from       Source Bazel version (default: auto-detect)
  --path       Path to project directory (default: current directory)
  --dry-run    Show what would change without applying
  --help       Show this help

%s
  bazelbump apply --to 9.0.0 --dry-run
  bazelbump apply --to 9.0.0
  bazelbump apply --from 8.0.0 --to 9.0.0 --path /path/to/project
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	toVersion := fs.String("to", "", "Target Bazel version (required)")
	fromVersion := fs.String("from", "", "Source Bazel version")
	projectPath := fs.String("path", ".", "Path to project directory")
	dryRun := fs.Bool("dry-run", false, "Show what would change without applying")

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

	// Step 1: Run analysis to get issues
	fmt.Printf("%s Analyzing upgrade from Bazel %s to %s\n",
		color(colorBlue, "==>"),
		color(colorCyan, *fromVersion),
		color(colorCyan, *toVersion))
	fmt.Printf("    Project: %s\n", *projectPath)

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

	fmt.Printf("    Files scanned: %d\n\n", result.FilesScanned)

	if !result.HasIssues() {
		fmt.Printf("%s No issues found! Nothing to apply.\n", color(colorGreen, "[OK]"))
		return nil
	}

	// Step 2: Generate patches
	fmt.Printf("%s Found %d issue(s), generating patches...\n",
		color(colorBlue, "==>"),
		len(result.FlagIssues))

	// Convert []FlagIssue to []*FlagIssue for GeneratePatches
	var issues []*analysis.FlagIssue
	for i := range result.FlagIssues {
		issues = append(issues, &result.FlagIssues[i])
	}

	patches, err := transform.GeneratePatches(issues)
	if err != nil {
		return fmt.Errorf("failed to generate patches: %w", err)
	}

	if len(patches) == 0 {
		fmt.Printf("%s No automatic fixes available for the detected issues.\n", color(colorYellow, "[WARN]"))
		fmt.Println("    Some issues may require manual intervention.")
		return nil
	}

	// Step 3: Show patches and apply (or dry-run)
	if *dryRun {
		fmt.Printf("\n%s Dry run - showing patches that would be applied:\n\n",
			color(colorBlue, "==>"))
		if err := transform.ApplyPatches(patches, true); err != nil {
			return fmt.Errorf("failed to show patches: %w", err)
		}
		fmt.Printf("\n%s Dry run complete. %d patch(es) would be applied.\n",
			color(colorGreen, "[OK]"),
			len(patches))
		fmt.Println("    Run without --dry-run to apply changes.")
	} else {
		fmt.Printf("\n%s Applying %d patch(es)...\n\n",
			color(colorBlue, "==>"),
			len(patches))

		// Show what will be changed
		filesModified := make(map[string]int)
		for _, p := range patches {
			filesModified[p.File]++
		}

		for file, count := range filesModified {
			fmt.Printf("    %s: %d change(s)\n", file, count)
		}
		fmt.Println()

		if err := transform.ApplyPatches(patches, false); err != nil {
			return fmt.Errorf("failed to apply patches: %w", err)
		}

		fmt.Printf("%s Successfully applied %d patch(es) to %d file(s).\n",
			color(colorGreen, "[OK]"),
			len(patches),
			len(filesModified))
	}

	return nil
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

// --- recipe command ---

func runRecipe(args []string) error {
	if len(args) < 1 {
		printRecipeUsage()
		return fmt.Errorf("recipe subcommand required")
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list":
		return runRecipeList(subargs)
	case "show":
		return runRecipeShow(subargs)
	case "run":
		return runRecipeRun(subargs)
	case "init":
		return runRecipeInit(subargs)
	case "validate":
		return runRecipeValidate(subargs)
	case "builtins":
		return runRecipeBuiltins(subargs)
	case "test":
		return runRecipeTest(subargs)
	case "help", "-h", "--help":
		printRecipeUsage()
		return nil
	default:
		printRecipeUsage()
		return fmt.Errorf("unknown recipe subcommand: %s", subcmd)
	}
}

func printRecipeUsage() {
	fmt.Fprintf(os.Stderr, `Run migration recipes.

Recipes are multi-step Starlark scripts that automate complex migrations
such as adding load() statements for native rules or updating MODULE.bazel.

%s
  bazelbump recipe <subcommand> [flags]

%s
  list      List available recipes (built-in and discovered)
  show      Show recipe details including steps
  run       Run a recipe against your project
  init      Create a new recipe from a template
  validate  Validate a recipe file without running it
  builtins  List available Starlark builtins
  test      Run tests for recipes (requires skytest)

%s
  bazelbump recipe list
  bazelbump recipe show autoload-migration
  bazelbump recipe run autoload-migration --dry-run
  bazelbump recipe run ./custom-recipe.star --dry-run
  bazelbump recipe init my-recipe --template flag-cleanup
  bazelbump recipe validate ./my-recipe.star
  bazelbump recipe builtins
  bazelbump recipe builtins ensure_load
  bazelbump recipe test
  bazelbump recipe test --coverage

%s
  .bazelbump/recipes/*.star    Project recipes (highest priority)
  recipes/*.star               Project recipes (alternate location)
  ~/.config/bazelbump/recipes  User recipes
  (built-in)                   Embedded in bazelbump

Use "bazelbump recipe <subcommand> --help" for more information.
See "bazelbump help recipes" or docs/user/recipes.md for full documentation.
`, color(colorBold, "Usage:"),
		color(colorBold, "Subcommands:"),
		color(colorBold, "Examples:"),
		color(colorBold, "Recipe Discovery:"))
}

func runRecipeList(args []string) error {
	fs := flag.NewFlagSet("recipe list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `List available recipes.

Shows all recipes from built-in sources and discovered locations.
Recipes are discovered from project directories and user config.

%s
  bazelbump recipe list [flags]

%s
  --json   Output results as JSON
  --path   Path to project directory (default: current directory)
  --help   Show this help

%s
  bazelbump recipe list
  bazelbump recipe list --json
  bazelbump recipe list --path ~/myproject
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	jsonOutput := fs.Bool("json", false, "Output results as JSON")
	projectPath := fs.String("path", ".", "Path to project directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get built-in recipes
	engine := recipe.NewEngine()
	builtinRecipes := engine.List()

	// Discover Starlark recipes
	discoveredRecipes, err := recipestar.DiscoverRecipes(*projectPath)
	if err != nil {
		// Log warning but continue with built-in recipes
		if os.Getenv("BAZELBUMP_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "warning: recipe discovery failed: %v\n", err)
		}
	}

	// Combine all recipes into unified list
	allRecipes := combineRecipeLists(builtinRecipes, discoveredRecipes)

	if *jsonOutput {
		return outputCombinedRecipeListJSON(allRecipes)
	}

	outputCombinedRecipeListText(allRecipes, *projectPath)
	return nil
}

// combinedRecipe represents a recipe from any source for unified listing.
type combinedRecipe struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Source      string `json:"source"` // "builtin", "project", "user"
	Path        string `json:"path,omitempty"`
}

// combineRecipeLists merges built-in and discovered recipes.
func combineRecipeLists(builtin []recipe.RecipeSummary, discovered []recipestar.DiscoveredRecipe) []combinedRecipe {
	seen := make(map[string]bool)
	var result []combinedRecipe

	// Add discovered recipes first (they have priority for same names)
	for _, r := range discovered {
		if !seen[r.Name] {
			seen[r.Name] = true
			result = append(result, combinedRecipe{
				Name:        r.Name,
				Version:     r.Version,
				Description: r.Description,
				Source:      r.Source,
				Path:        r.Path,
			})
		}
	}

	// Add built-in recipes
	for _, r := range builtin {
		if !seen[r.Name] {
			seen[r.Name] = true
			result = append(result, combinedRecipe{
				Name:        r.Name,
				Version:     r.Version,
				Description: r.Description,
				Source:      "builtin",
			})
		}
	}

	return result
}

func outputCombinedRecipeListJSON(recipes []combinedRecipe) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(recipes)
}

func outputCombinedRecipeListText(recipes []combinedRecipe, projectPath string) {
	if len(recipes) == 0 {
		fmt.Println("No recipes available.")
		fmt.Println("\nRecipe search paths:")
		for _, p := range recipestar.GetDiscoveryPaths(projectPath) {
			fmt.Printf("  - %s\n", p)
		}
		return
	}

	fmt.Printf("%s Available recipes:\n\n", color(colorBlue, "==>"))

	// Calculate column widths
	maxNameLen := 4   // "NAME"
	maxSourceLen := 6 // "SOURCE"
	for _, r := range recipes {
		if len(r.Name) > maxNameLen {
			maxNameLen = len(r.Name)
		}
		if len(r.Source) > maxSourceLen {
			maxSourceLen = len(r.Source)
		}
	}

	// Print header
	fmt.Printf("  %-*s  %-*s  %-8s  %s\n", maxNameLen, "NAME", maxSourceLen, "SOURCE", "VERSION", "DESCRIPTION")
	fmt.Printf("  %s  %s  %s  %s\n", strings.Repeat("-", maxNameLen), strings.Repeat("-", maxSourceLen), "--------", "-----------")

	// Print recipes
	for _, r := range recipes {
		desc := r.Description
		if len(desc) > 45 {
			desc = desc[:42] + "..."
		}

		sourceColor := colorReset
		switch r.Source {
		case "builtin":
			sourceColor = colorCyan
		case "project":
			sourceColor = colorGreen
		case "user":
			sourceColor = colorYellow
		}

		fmt.Printf("  %-*s  %s%-*s%s  %-8s  %s\n",
			maxNameLen, r.Name,
			sourceColor, maxSourceLen, r.Source, colorReset,
			r.Version, desc)
	}

	fmt.Printf("\nTotal: %d recipe(s)\n", len(recipes))

	// Show search paths
	fmt.Printf("\n%s\n", color(colorBold, "Recipe search paths:"))
	for _, p := range recipestar.GetDiscoveryPaths(projectPath) {
		fmt.Printf("  - %s\n", p)
	}
}

// Keep original functions for backward compatibility
func outputRecipeListJSON(recipes []recipe.RecipeSummary) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(recipes)
}

func outputRecipeListText(recipes []recipe.RecipeSummary) {
	if len(recipes) == 0 {
		fmt.Println("No recipes available.")
		return
	}

	fmt.Printf("%s Available recipes:\n\n", color(colorBlue, "==>"))

	// Calculate column widths
	maxNameLen := 4 // "NAME"
	for _, r := range recipes {
		if len(r.Name) > maxNameLen {
			maxNameLen = len(r.Name)
		}
	}

	// Print header
	fmt.Printf("  %-*s  %-8s  %s\n", maxNameLen, "NAME", "VERSION", "DESCRIPTION")
	fmt.Printf("  %s  %s  %s\n", strings.Repeat("-", maxNameLen), "--------", "-----------")

	// Print recipes
	for _, r := range recipes {
		desc := r.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Printf("  %-*s  %-8s  %s\n", maxNameLen, r.Name, r.Version, desc)
	}

	fmt.Printf("\nTotal: %d recipe(s)\n", len(recipes))
}

func runRecipeShow(args []string) error {
	fs := flag.NewFlagSet("recipe show", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Show recipe details.

%s
  bazelbump recipe show [flags] <name>
  bazelbump recipe show [flags] ./path/to/recipe.star

%s
  --json   Output results as JSON
  --path   Path to project directory (default: current directory)
  --help   Show this help

%s
  bazelbump recipe show autoload-migration
  bazelbump recipe show --path /path/to/project my-recipe
  bazelbump recipe show ./recipes/custom.star
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	jsonOutput := fs.Bool("json", false, "Output results as JSON")
	projectPath := fs.String("path", ".", "Path to project directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("recipe name required")
	}

	name := fs.Arg(0)

	// First try to find as a discovered/starlark recipe
	discovered, err := recipestar.ResolveRecipe(name, *projectPath)
	if err == nil {
		// Found a Starlark recipe, show its details
		return showStarlarkRecipe(discovered, *jsonOutput)
	}

	// Fall back to built-in recipes
	engine := recipe.NewEngine()
	r, ok := engine.Get(name)
	if !ok {
		// Recipe not found anywhere
		return fmt.Errorf("unknown recipe: %s\n\nUse 'bazelbump recipe list' to see available recipes", name)
	}

	if *jsonOutput {
		return outputRecipeShowJSON(r)
	}

	outputRecipeShowText(r)
	return nil
}

// showStarlarkRecipe displays details of a discovered Starlark recipe.
func showStarlarkRecipe(discovered *recipestar.DiscoveredRecipe, jsonOutput bool) error {
	// Parse the recipe to get full details
	executor := recipestar.NewExecutor(".", recipestar.Options{})
	starlarkRecipe, err := executor.Interpret(discovered.Path)
	if err != nil {
		// Fall back to basic info from discovery
		if jsonOutput {
			return outputDiscoveredRecipeJSON(discovered)
		}
		outputDiscoveredRecipeText(discovered)
		return nil
	}

	if jsonOutput {
		return outputStarlarkRecipeJSON(starlarkRecipe, discovered)
	}

	outputStarlarkRecipeText(starlarkRecipe, discovered)
	return nil
}

// outputDiscoveredRecipeJSON outputs basic discovered recipe info as JSON.
func outputDiscoveredRecipeJSON(r *recipestar.DiscoveredRecipe) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// outputDiscoveredRecipeText outputs basic discovered recipe info as text.
func outputDiscoveredRecipeText(r *recipestar.DiscoveredRecipe) {
	fmt.Printf("%s Recipe: %s\n\n", color(colorBlue, "==>"), color(colorBold, r.Name))
	fmt.Printf("  Source:      %s\n", r.Source)
	fmt.Printf("  Path:        %s\n", r.Path)
	if r.Version != "" {
		fmt.Printf("  Version:     %s\n", r.Version)
	}
	if r.Description != "" {
		fmt.Printf("  Description: %s\n", r.Description)
	}
	fmt.Printf("\n  (Run 'bazelbump recipe run %s --dry-run' to preview)\n", r.Name)
}

// starlarkRecipeOutput is used for JSON serialization of Starlark recipes.
type starlarkRecipeOutput struct {
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	Description     string               `json:"description"`
	Source          string               `json:"source"`
	Path            string               `json:"path"`
	MinBazelVersion string               `json:"min_bazel_version,omitempty"`
	MaxBazelVersion string               `json:"max_bazel_version,omitempty"`
	Requires        []string             `json:"requires,omitempty"`
	Steps           []starlarkStepOutput `json:"steps"`
}

type starlarkStepOutput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// outputStarlarkRecipeJSON outputs a fully parsed Starlark recipe as JSON.
func outputStarlarkRecipeJSON(r *recipestar.Recipe, discovered *recipestar.DiscoveredRecipe) error {
	// Convert capabilities to strings
	var requires []string
	for _, cap := range r.Requires {
		requires = append(requires, string(cap))
	}

	output := starlarkRecipeOutput{
		Name:            r.Name,
		Version:         r.Version,
		Description:     r.Description,
		Source:          discovered.Source,
		Path:            discovered.Path,
		MinBazelVersion: r.MinBazelVersion,
		MaxBazelVersion: r.MaxBazelVersion,
		Requires:        requires,
	}

	for _, step := range r.Steps {
		stepType := "unknown"
		switch step.(type) {
		case *recipestar.ScanStep:
			stepType = "scan"
		case *recipestar.EditStep:
			stepType = "edit"
		case *recipestar.VerifyStep:
			stepType = "verify"
		}

		output.Steps = append(output.Steps, starlarkStepOutput{
			ID:          step.GetID(),
			Name:        step.GetName(),
			Type:        stepType,
			Description: step.GetDescription(),
			DependsOn:   step.GetDependsOn(),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// outputStarlarkRecipeText outputs a fully parsed Starlark recipe as text.
func outputStarlarkRecipeText(r *recipestar.Recipe, discovered *recipestar.DiscoveredRecipe) {
	fmt.Printf("%s Recipe: %s\n\n", color(colorBlue, "==>"), color(colorBold, r.Name))

	// Show source info
	sourceColor := colorReset
	switch discovered.Source {
	case "project":
		sourceColor = colorGreen
	case "user":
		sourceColor = colorYellow
	case "explicit":
		sourceColor = colorCyan
	}
	fmt.Printf("  Source:      %s%s%s\n", sourceColor, discovered.Source, colorReset)
	fmt.Printf("  Path:        %s\n", discovered.Path)
	fmt.Printf("  Version:     %s\n", r.Version)
	fmt.Printf("  Description: %s\n", r.Description)

	if r.MinBazelVersion != "" || r.MaxBazelVersion != "" {
		fmt.Printf("  Bazel:       ")
		if r.MinBazelVersion != "" && r.MaxBazelVersion != "" {
			fmt.Printf("%s - %s\n", r.MinBazelVersion, r.MaxBazelVersion)
		} else if r.MinBazelVersion != "" {
			fmt.Printf(">= %s\n", r.MinBazelVersion)
		} else {
			fmt.Printf("<= %s\n", r.MaxBazelVersion)
		}
	}

	// Show required capabilities
	if len(r.Requires) > 0 {
		fmt.Printf("  Requires:    ")
		for i, cap := range r.Requires {
			if i > 0 {
				fmt.Printf(", ")
			}
			info := recipestar.GetCapabilityInfo(cap)
			capColor := colorYellow
			if info != nil && info.Risk == "high" {
				capColor = colorRed
			}
			fmt.Printf("%s", color(capColor, string(cap)))
		}
		fmt.Println()
	}

	fmt.Printf("\n%s (%d total)\n", color(colorBold, "Steps:"), len(r.Steps))

	for i, step := range r.Steps {
		stepType := "unknown"
		switch step.(type) {
		case *recipestar.ScanStep:
			stepType = "scan"
		case *recipestar.EditStep:
			stepType = "edit"
		case *recipestar.VerifyStep:
			stepType = "verify"
		}

		fmt.Printf("\n  %d. %s (%s)\n", i+1, color(colorCyan, step.GetName()), stepType)
		fmt.Printf("     ID: %s\n", step.GetID())
		if step.GetDescription() != "" {
			fmt.Printf("     %s\n", step.GetDescription())
		}
		if len(step.GetDependsOn()) > 0 {
			fmt.Printf("     Depends on: %s\n", strings.Join(step.GetDependsOn(), ", "))
		}
	}
}

func outputRecipeShowJSON(r *recipe.Recipe) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func outputRecipeShowText(r *recipe.Recipe) {
	fmt.Printf("%s Recipe: %s\n\n", color(colorBlue, "==>"), color(colorBold, r.Name))
	fmt.Printf("  Version:     %s\n", r.Version)
	fmt.Printf("  Description: %s\n", r.Description)

	if r.MinBazelVersion != "" || r.MaxBazelVersion != "" {
		fmt.Printf("  Bazel:       ")
		if r.MinBazelVersion != "" && r.MaxBazelVersion != "" {
			fmt.Printf("%s - %s\n", r.MinBazelVersion, r.MaxBazelVersion)
		} else if r.MinBazelVersion != "" {
			fmt.Printf(">= %s\n", r.MinBazelVersion)
		} else {
			fmt.Printf("<= %s\n", r.MaxBazelVersion)
		}
	}

	fmt.Printf("\n%s (%d total)\n", color(colorBold, "Steps:"), len(r.Steps))

	for i, step := range r.Steps {
		stepType := getStepType(&step)
		fmt.Printf("\n  %d. %s (%s)\n", i+1, color(colorCyan, step.Name), stepType)
		fmt.Printf("     ID: %s\n", step.ID)
		if step.Description != "" {
			fmt.Printf("     %s\n", step.Description)
		}
		if len(step.DependsOn) > 0 {
			fmt.Printf("     Depends on: %s\n", strings.Join(step.DependsOn, ", "))
		}
	}
}

func getStepType(step *recipe.Step) string {
	if step.Scan != nil {
		return "scan"
	}
	if step.Edit != nil {
		return "edit"
	}
	if step.Verify != nil {
		return "verify"
	}
	return "unknown"
}

func runRecipeRun(args []string) error {
	fs := flag.NewFlagSet("recipe run", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Run a recipe.

Executes a recipe against your project, making the changes defined in the
recipe's steps. Always run with --dry-run first to preview changes.

%s
  bazelbump recipe run [flags] <name>
  bazelbump recipe run [flags] ./path/to/recipe.star

%s
  --dry-run              Show what would change without applying (recommended first)
  --path                 Path to project directory (default: current directory)
  --step                 Run only a specific step (by ID)
  --verbose              Show verbose output
  --json                 Output results as JSON (machine-readable)
  --no-color             Disable colored output
  --resume               Resume from the last interrupted run
  --resume-run <id>      Resume from a specific run ID
  --checkpoint-interval  Files between auto checkpoints (default: 100, 0 to disable)
  --no-checkpoint        Disable checkpointing
  --progress <mode>      Progress output: auto, terminal, json, none (default: auto)

  %s
  --approve              Auto-approve declared capabilities (for CI/scripts)
  --trust                Trust all capabilities without prompting
  --allow <caps>         Allow specific capabilities (comma-separated: exec,bcr)
  --deny <caps>          Deny specific capabilities (overrides --allow and --approve)
  --help                 Show this help

%s
  # Preview changes (always do this first)
  bazelbump recipe run --dry-run autoload-migration

  # Apply changes
  bazelbump recipe run autoload-migration

  # Run against a specific project
  bazelbump recipe run --path /path/to/project autoload-migration

  # Run a custom recipe file
  bazelbump recipe run --dry-run ./my-recipe.star

  # Run only the detection step
  bazelbump recipe run --step detect-native-rules autoload-migration

  # Get machine-readable output
  bazelbump recipe run --dry-run --json autoload-migration

  # Resume an interrupted run
  bazelbump recipe run --resume autoload-migration

  # Resume a specific run
  bazelbump recipe run --resume-run abc123 autoload-migration

  # Auto-approve recipe's declared capabilities
  bazelbump recipe run --approve format-build

  # Allow specific capabilities
  bazelbump recipe run --allow=exec,bcr my-recipe

%s
  If a step fails, the recipe's on_failure mode determines behavior:
  - rollback: Revert all changes and stop (default)
  - stop:     Stop but keep changes made so far
  - continue: Log error and continue with next step

%s
  Checkpoints are created automatically during execution to allow resuming
  interrupted runs. They are created:
  - Every N files (default: 100, configurable with --checkpoint-interval)
  - At step boundaries
  - When the process receives SIGINT/SIGTERM

  Use 'bazelbump checkpoint list' to see available checkpoints.
  Use 'bazelbump status' to see if a run can be resumed.

%s
  Recipes can declare capabilities they require (exec, bcr). By default, you'll
  be prompted to approve these capabilities interactively (30s timeout, auto-deny).
  Use --approve to auto-approve declared capabilities in CI/scripts.
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Capabilities:"),
			color(colorBold, "Examples:"),
			color(colorBold, "Failure Handling:"),
			color(colorBold, "Checkpointing:"),
			color(colorBold, "Capability System:"))
	}

	dryRun := fs.Bool("dry-run", false, "Show what would change without applying")
	projectPath := fs.String("path", ".", "Path to project directory")
	stepID := fs.String("step", "", "Run only a specific step")
	verbose := fs.Bool("verbose", false, "Show verbose output")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")
	noColor := fs.Bool("no-color", false, "Disable colored output")
	resume := fs.Bool("resume", false, "Resume from the last interrupted run")
	resumeRun := fs.String("resume-run", "", "Resume from a specific run ID")
	checkpointInterval := fs.Int("checkpoint-interval", 100, "Files between auto checkpoints (0 to disable)")
	noCheckpoint := fs.Bool("no-checkpoint", false, "Disable checkpointing")
	progressMode := fs.String("progress", "auto", "Progress output: auto, terminal, json, none")
	approve := fs.Bool("approve", false, "Auto-approve declared capabilities")
	trust := fs.Bool("trust", false, "Trust all capabilities (CI/automation)")
	allow := fs.String("allow", "", "Allow specific capabilities (comma-separated)")
	deny := fs.String("deny", "", "Deny specific capabilities (comma-separated)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("recipe name required")
	}

	name := fs.Arg(0)

	// Build state options for Starlark recipes
	stateRunOpts := &starlarkStateOptions{
		resume:             *resume,
		resumeRun:          *resumeRun,
		checkpointInterval: *checkpointInterval,
		noCheckpoint:       *noCheckpoint,
		progressMode:       *progressMode,
	}

	// Build capability options
	capOpts := &capabilityOptions{
		approve: *approve,
		trust:   *trust,
		allow:   *allow,
		deny:    *deny,
	}

	// Try to resolve as a discovered Starlark recipe first
	discovered, resolveErr := recipestar.ResolveRecipe(name, *projectPath)
	if resolveErr == nil {
		// Found a Starlark recipe - run it
		return runStarlarkRecipeWithState(discovered.Path, *projectPath, *dryRun, *stepID, *verbose, *jsonOutput, *noColor, stateRunOpts, capOpts)
	}

	// Fall back to built-in Go recipes
	engine := recipe.NewEngine()

	// Verify recipe exists in built-in recipes
	if _, ok := engine.Get(name); !ok {
		// Neither discovered nor built-in
		return fmt.Errorf("unknown recipe: %s\n\nUse 'bazelbump recipe list' to see available recipes\n\nRecipe search paths:\n%s",
			name, formatSearchPaths(*projectPath))
	}

	opts := recipe.Options{
		ProjectPath: *projectPath,
		DryRun:      *dryRun,
		StepID:      *stepID,
		Verbose:     *verbose,
	}

	result, err := engine.Run(name, opts)
	if err != nil {
		return fmt.Errorf("recipe execution failed: %w", err)
	}

	if *jsonOutput {
		return outputRecipeRunJSON(result)
	}

	outputRecipeRunText(result, *dryRun)

	if !result.Success {
		os.Exit(1)
	}
	return nil
}

func runRecipeValidate(args []string) error {
	fs := flag.NewFlagSet("recipe validate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Validate a recipe file without running it.

Checks the recipe for syntax errors, structural issues, and potential problems
without making any changes to your project.

%s
  bazelbump recipe validate [flags] <recipe-file>
  bazelbump recipe validate [flags] <recipe-name>

%s
  --json   Output results as JSON
  --path   Path to project directory (default: current directory)
  --help   Show this help

%s
  bazelbump recipe validate ./my-recipe.star
  bazelbump recipe validate ./recipes/autoload.star --json
  bazelbump recipe validate autoload-migration

%s
  - Syntax is valid Starlark
  - Recipe metadata is complete (name, version)
  - All steps have unique IDs
  - Step dependencies form a valid DAG (no cycles)
  - All operations reference valid builtins
  - File patterns and regex patterns are valid
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"),
			color(colorBold, "Validation Checks:"))
	}

	jsonOutput := fs.Bool("json", false, "Output results as JSON")
	projectPath := fs.String("path", ".", "Path to project directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("recipe file required")
	}

	recipeArg := fs.Arg(0)

	// Resolve the recipe path
	var absPath string
	var err error

	// Check if it's an explicit path or a recipe name
	if strings.HasSuffix(recipeArg, ".star") || strings.Contains(recipeArg, "/") || strings.Contains(recipeArg, string(os.PathSeparator)) {
		// Explicit path
		absPath, err = filepath.Abs(recipeArg)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
	} else {
		// Recipe name - try to resolve it
		discovered, resolveErr := recipestar.ResolveRecipe(recipeArg, *projectPath)
		if resolveErr != nil {
			return fmt.Errorf("unknown recipe: %s\n\nUse 'bazelbump recipe list' to see available recipes", recipeArg)
		}
		absPath = discovered.Path
	}

	// Check file exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("recipe file not found: %s", recipeArg)
	}

	// Perform validation
	validationResult := validateRecipeFile(absPath, *projectPath)

	if *jsonOutput {
		return outputValidationJSON(validationResult)
	}

	outputValidationText(validationResult)

	if !validationResult.Valid {
		os.Exit(1)
	}
	return nil
}

// validationResult holds the results of recipe validation.
type validationResult struct {
	File        string             `json:"file"`
	Valid       bool               `json:"valid"`
	RecipeName  string             `json:"recipe_name,omitempty"`
	Version     string             `json:"version,omitempty"`
	Checks      []validationCheck  `json:"checks"`
	Errors      []validationIssue  `json:"errors,omitempty"`
	Warnings    []validationIssue  `json:"warnings,omitempty"`
}

// validationCheck represents a single validation check.
type validationCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// validationIssue represents an error or warning during validation.
type validationIssue struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// validateRecipeFile performs comprehensive validation of a recipe file.
func validateRecipeFile(path, projectPath string) *validationResult {
	result := &validationResult{
		File:   path,
		Valid:  true,
		Checks: make([]validationCheck, 0),
	}

	// Create interpreter for parsing
	interp := recipestar.NewInterpreter(recipestar.WithProjectPath(projectPath))

	// Phase 1: Syntax validation
	recipe, parseErr := interp.LoadFile(path)
	if parseErr != nil {
		result.Valid = false
		result.Checks = append(result.Checks, validationCheck{
			Name:    "Syntax valid",
			Passed:  false,
			Message: parseErr.Error(),
		})
		result.Errors = append(result.Errors, validationIssue{
			Message: parseErr.Error(),
			Hint:    "Check the Starlark syntax and ensure all required arguments are provided",
		})
		return result
	}

	result.Checks = append(result.Checks, validationCheck{
		Name:   "Syntax valid",
		Passed: true,
	})

	// Store recipe metadata
	result.RecipeName = recipe.Name
	result.Version = recipe.Version

	// Phase 2: Recipe metadata validation
	metadataValid := true
	var metadataMsg string
	if recipe.Name == "" {
		metadataValid = false
		metadataMsg = "recipe name is missing"
	} else if recipe.Version == "" {
		metadataValid = false
		metadataMsg = "recipe version is missing"
	}

	if metadataValid {
		result.Checks = append(result.Checks, validationCheck{
			Name:   "Recipe metadata complete (name, version)",
			Passed: true,
		})
	} else {
		result.Valid = false
		result.Checks = append(result.Checks, validationCheck{
			Name:    "Recipe metadata complete (name, version)",
			Passed:  false,
			Message: metadataMsg,
		})
		result.Errors = append(result.Errors, validationIssue{
			Message: metadataMsg,
			Hint:    "Add missing 'name' and 'version' arguments to the recipe() call",
		})
	}

	// Phase 3: Step ID uniqueness
	stepIDs := make(map[string]bool)
	duplicateIDs := make([]string, 0)
	for _, step := range recipe.Steps {
		id := step.GetID()
		if stepIDs[id] {
			duplicateIDs = append(duplicateIDs, id)
		}
		stepIDs[id] = true
	}

	if len(duplicateIDs) == 0 {
		result.Checks = append(result.Checks, validationCheck{
			Name:   "All steps have unique IDs",
			Passed: true,
		})
	} else {
		result.Valid = false
		result.Checks = append(result.Checks, validationCheck{
			Name:    "All steps have unique IDs",
			Passed:  false,
			Message: fmt.Sprintf("duplicate IDs: %s", strings.Join(duplicateIDs, ", ")),
		})
		result.Errors = append(result.Errors, validationIssue{
			Message: fmt.Sprintf("Duplicate step IDs found: %s", strings.Join(duplicateIDs, ", ")),
			Hint:    "Each step must have a unique 'id' parameter",
		})
	}

	// Phase 4: Dependency validation (DAG check)
	depErrors := validateDependencies(recipe.Steps, stepIDs)
	if len(depErrors) == 0 {
		result.Checks = append(result.Checks, validationCheck{
			Name:   "Step dependencies form valid DAG",
			Passed: true,
		})
	} else {
		result.Valid = false
		result.Checks = append(result.Checks, validationCheck{
			Name:    "Step dependencies form valid DAG",
			Passed:  false,
			Message: depErrors[0].Message,
		})
		result.Errors = append(result.Errors, depErrors...)
	}

	// Phase 5: Operation validation using the executor's validate method
	executor := recipestar.NewExecutor(projectPath, recipestar.Options{})
	validationErrors := executor.ValidateRecipe(recipe)

	// Filter out errors we've already reported
	var opErrors []string
	for _, e := range validationErrors {
		// Skip metadata errors (already reported)
		if strings.Contains(e, "recipe name") || strings.Contains(e, "recipe version") {
			continue
		}
		// Skip duplicate ID errors (already reported)
		if strings.Contains(e, "duplicate step ID") {
			continue
		}
		// Skip dependency errors (already reported)
		if strings.Contains(e, "depends on unknown") || strings.Contains(e, "circular") {
			continue
		}
		opErrors = append(opErrors, e)
	}

	if len(opErrors) == 0 {
		result.Checks = append(result.Checks, validationCheck{
			Name:   "All operations reference valid builtins",
			Passed: true,
		})
	} else {
		result.Valid = false
		result.Checks = append(result.Checks, validationCheck{
			Name:    "All operations reference valid builtins",
			Passed:  false,
			Message: fmt.Sprintf("%d operation error(s)", len(opErrors)),
		})
		for _, e := range opErrors {
			result.Errors = append(result.Errors, validationIssue{
				Message: e,
			})
		}
	}

	// Add warnings for potential issues
	warnings := checkForWarnings(recipe)
	result.Warnings = warnings

	return result
}

// validateDependencies checks for missing dependencies and cycles.
func validateDependencies(steps []recipestar.Step, stepIDs map[string]bool) []validationIssue {
	var issues []validationIssue

	// Build dependency graph
	graph := make(map[string][]string)
	for _, step := range steps {
		id := step.GetID()
		deps := step.GetDependsOn()
		graph[id] = deps

		// Check for unknown dependencies
		for _, dep := range deps {
			if !stepIDs[dep] {
				availableSteps := make([]string, 0, len(stepIDs))
				for s := range stepIDs {
					availableSteps = append(availableSteps, s)
				}
				slices.Sort(availableSteps)
				issues = append(issues, validationIssue{
					Message: fmt.Sprintf("Step %q depends on unknown step %q", id, dep),
					Hint:    fmt.Sprintf("Available steps: %s", strings.Join(availableSteps, ", ")),
				})
			}
		}
	}

	// Check for cycles using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cyclePath []string

	var hasCycle func(id string) bool
	hasCycle = func(id string) bool {
		visited[id] = true
		recStack[id] = true
		cyclePath = append(cyclePath, id)

		for _, dep := range graph[id] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				cyclePath = append(cyclePath, dep)
				return true
			}
		}

		recStack[id] = false
		cyclePath = cyclePath[:len(cyclePath)-1]
		return false
	}

	for _, step := range steps {
		id := step.GetID()
		if !visited[id] {
			cyclePath = nil
			if hasCycle(id) {
				issues = append(issues, validationIssue{
					Message: fmt.Sprintf("Circular dependency detected: %s", strings.Join(cyclePath, " -> ")),
					Hint:    "Remove the circular dependency by restructuring step dependencies",
				})
				break
			}
		}
	}

	return issues
}

// checkForWarnings identifies potential issues that don't prevent execution.
func checkForWarnings(recipe *recipestar.Recipe) []validationIssue {
	var warnings []validationIssue

	// Warning: No description
	if recipe.Description == "" {
		warnings = append(warnings, validationIssue{
			Message: "Recipe has no description",
			Hint:    "Add a 'description' parameter to help users understand what this recipe does",
		})
	}

	// Warning: Steps without names
	for _, step := range recipe.Steps {
		if step.GetName() == "" {
			warnings = append(warnings, validationIssue{
				Message: fmt.Sprintf("Step %q has no display name", step.GetID()),
				Hint:    "Add a 'name' parameter for better output readability",
			})
		}
	}

	return warnings
}

// outputValidationJSON outputs validation results as JSON.
func outputValidationJSON(result *validationResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// outputValidationText outputs validation results as human-readable text.
func outputValidationText(result *validationResult) {
	fmt.Printf("Validating %s...\n\n", filepath.Base(result.File))

	// Print each check
	for _, check := range result.Checks {
		if check.Passed {
			fmt.Printf("%s %s\n", color(colorGreen, "OK"), check.Name)
		} else {
			fmt.Printf("%s %s\n", color(colorRed, "X"), check.Name)
			if check.Message != "" {
				fmt.Printf("   %s\n", check.Message)
			}
		}
	}

	// Print detailed errors
	if len(result.Errors) > 0 {
		fmt.Println()
		for _, err := range result.Errors {
			fmt.Printf("%s %s\n", color(colorRed, "X"), err.Message)
			if err.Hint != "" {
				fmt.Printf("\n  %s %s\n", color(colorBlue, "hint:"), err.Hint)
			}
		}
	}

	// Print warnings
	if len(result.Warnings) > 0 {
		fmt.Println()
		fmt.Printf("%s\n", color(colorBold, "Warnings:"))
		for _, warn := range result.Warnings {
			fmt.Printf("  %s %s\n", color(colorYellow, "!"), warn.Message)
			if warn.Hint != "" {
				fmt.Printf("    %s %s\n", color(colorBlue, "hint:"), warn.Hint)
			}
		}
	}

	// Print summary
	fmt.Println()
	if result.Valid {
		if result.RecipeName != "" {
			fmt.Printf("Recipe %q v%s is valid!\n",
				color(colorGreen, result.RecipeName),
				result.Version)
		} else {
			fmt.Printf("%s Recipe is valid!\n", color(colorGreen, "[OK]"))
		}
	} else {
		errorCount := len(result.Errors)
		if errorCount == 0 {
			errorCount = 1 // At least one check failed
		}
		fmt.Printf("Found %d error(s). Recipe is invalid.\n", errorCount)
	}
}

// formatSearchPaths formats the recipe search paths for error messages.
func formatSearchPaths(projectPath string) string {
	paths := recipestar.GetDiscoveryPaths(projectPath)
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString("  - ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	return sb.String()
}

// starlarkStateOptions holds state management options for recipe execution.
type starlarkStateOptions struct {
	resume             bool
	resumeRun          string
	checkpointInterval int
	noCheckpoint       bool
	progressMode       string
}

// capabilityOptions holds capability-related flags.
type capabilityOptions struct {
	approve bool   // Auto-approve declared capabilities
	trust   bool   // Trust all capabilities
	allow   string // Comma-separated list of capabilities to allow
	deny    string // Comma-separated list of capabilities to deny
}

// runStarlarkRecipe runs a Starlark .star recipe file (legacy, without state management).
func runStarlarkRecipe(path, projectPath string, dryRun bool, stepID string, verbose, jsonOutput, noColor bool) error {
	return runStarlarkRecipeWithState(path, projectPath, dryRun, stepID, verbose, jsonOutput, noColor, nil, nil)
}

// runStarlarkRecipeWithState runs a Starlark .star recipe file with state management.
func runStarlarkRecipeWithState(path, projectPath string, dryRun bool, stepID string, verbose, jsonOutput, noColor bool, stateOpts *starlarkStateOptions, capOpts *capabilityOptions) error {
	// Resolve path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check file exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("recipe file not found: %s", path)
	}

	// Phase 1: Parse recipe (without capabilities) to get requirements
	parseInterp := recipestar.NewInterpreter(recipestar.WithProjectPath(projectPath))
	recipe, parseErr := parseInterp.LoadFile(absPath)
	if parseErr != nil {
		return fmt.Errorf("failed to parse recipe: %w", parseErr)
	}

	// Phase 2: Build capability config from flags and resolve capabilities
	capConfig, err := resolveCapabilities(recipe, capOpts, verbose)
	if err != nil {
		return err
	}

	// Phase 3: Build interpreter options with approved capabilities
	interpOpts := recipestar.WithCapabilities(projectPath, capConfig)

	// Add BCR client if bcr capability is granted
	if capConfig.IsGranted(recipestar.CapabilityBCR) {
		interpOpts = append(interpOpts, recipestar.WithBCRClient(bcr.New()))
	}

	// Base options
	baseOpts := recipestar.Options{
		DryRun:  dryRun,
		Verbose: verbose,
		StepID:  stepID,
	}

	// Build executor options for state management
	var execOpts []recipestar.ExecutorOption

	// Initialize state manager if available and not dry run
	if state.Available() && stateOpts != nil {
		// Initialize state manager
		dbPath := state.StateDBPath(projectPath)
		mgr, err := state.New(dbPath)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to initialize state manager: %v\n", err)
			}
		} else {
			// Note: We need to defer Close() somewhere appropriate
			// For now, we'll rely on the process ending to close it
			execOpts = append(execOpts, recipestar.WithStateManager(mgr))

			// Configure checkpointing
			if !stateOpts.noCheckpoint && !dryRun {
				cpConfig := recipestar.DefaultCheckpointConfig()
				cpConfig.AutoInterval = stateOpts.checkpointInterval
				if stateOpts.checkpointInterval <= 0 {
					cpConfig.Enabled = false
				}
				execOpts = append(execOpts, recipestar.WithCheckpointConfig(cpConfig))
			}

			// Handle resume
			if stateOpts.resume || stateOpts.resumeRun != "" {
				resumeRunID := stateOpts.resumeRun
				if resumeRunID == "" {
					// Find the latest interrupted run for this recipe
					latestRun, runErr := mgr.GetLatestRun(recipe.Name)
					if runErr == nil && latestRun != nil &&
						(latestRun.Status == state.RunStatusInterrupted || latestRun.Status == state.RunStatusRunning) {
						resumeRunID = latestRun.ID
					}
				}
				if resumeRunID != "" {
					execOpts = append(execOpts, recipestar.WithResume(resumeRunID, ""))
					if verbose {
						fmt.Printf("Resuming run: %s\n", resumeRunID)
					}
				} else if stateOpts.resume {
					return fmt.Errorf("no interrupted run found to resume")
				}
			}

			// Configure progress reporter based on mode
			var progressReporter recipestar.ProgressReporter
			switch stateOpts.progressMode {
			case "terminal":
				progressReporter = newTerminalProgressReporter(verbose)
			case "json":
				progressReporter = newJSONProgressReporter()
			case "none":
				progressReporter = &recipestar.NoopProgressReporter{}
			case "auto":
				if jsonOutput {
					progressReporter = newJSONProgressReporter()
				} else if isTerminal() {
					progressReporter = newTerminalProgressReporter(verbose)
				} else {
					progressReporter = &recipestar.NoopProgressReporter{}
				}
			default:
				progressReporter = &recipestar.NoopProgressReporter{}
			}
			execOpts = append(execOpts, recipestar.WithProgressReporter(progressReporter))
		}
	}

	// Phase 4: Create executor with capabilities and execute
	var result *recipestar.Result
	if len(execOpts) > 0 {
		executor := recipestar.NewExecutorWithOptions(projectPath, baseOpts, execOpts...)
		// Re-parse with capabilities enabled
		execInterp := recipestar.NewInterpreter(interpOpts...)
		recipe, parseErr = execInterp.LoadFile(absPath)
		if parseErr != nil {
			return fmt.Errorf("failed to parse recipe: %w", parseErr)
		}
		result, err = executor.ExecuteWithState(recipe, baseOpts)
	} else {
		// Fall back to regular executor without state management
		// Create executor with interpreter that has capabilities
		executor := recipestar.NewExecutorWithInterpreter(projectPath, baseOpts, recipestar.NewInterpreter(interpOpts...))
		result, err = executor.ExecuteFromFile(absPath, baseOpts)
	}

	if err != nil {
		return fmt.Errorf("recipe execution failed: %w", err)
	}

	if jsonOutput {
		return outputRecipeRunDiffJSON(result, dryRun)
	}

	outputRecipeRunDiffText(result, dryRun, noColor)

	if !result.Success {
		os.Exit(1)
	}
	return nil
}

// resolveCapabilities builds a capability config from flags and prompts if needed.
func resolveCapabilities(recipe *recipestar.Recipe, capOpts *capabilityOptions, verbose bool) (*recipestar.CapabilityConfig, error) {
	config := recipestar.NewCapabilityConfig()

	if capOpts == nil {
		capOpts = &capabilityOptions{}
	}

	// Handle --trust: grant all capabilities
	if capOpts.trust {
		config.TrustAll = true
	}

	// Handle --allow: grant specific capabilities
	if capOpts.allow != "" {
		allowed, err := recipestar.ParseCapabilities(capOpts.allow)
		if err != nil {
			return nil, fmt.Errorf("invalid --allow value: %w", err)
		}
		config.Grant(allowed...)
	}

	// Handle --deny: deny specific capabilities (takes precedence)
	if capOpts.deny != "" {
		denied, err := recipestar.ParseCapabilities(capOpts.deny)
		if err != nil {
			return nil, fmt.Errorf("invalid --deny value: %w", err)
		}
		config.Deny(denied...)
	}

	// Handle --approve: auto-approve declared capabilities
	if capOpts.approve {
		config.Grant(recipe.Requires...)
	}

	// Check if any required capabilities are missing
	missing := config.RequiredButNotGranted(recipe.Requires)
	if len(missing) == 0 {
		return config, nil
	}

	// Prompt for missing capabilities if interactive
	if !isTerminal() {
		// Non-interactive: fail with instructions
		return nil, fmt.Errorf("recipe %q requires capabilities: %s\n\nUse --approve to auto-approve, or --allow=%s to grant specific capabilities",
			recipe.Name,
			recipestar.FormatCapabilityList(missing),
			recipestar.FormatCapabilityList(missing))
	}

	// Interactive prompt with 30s timeout
	approved, err := promptForCapabilities(recipe.Name, missing)
	if err != nil {
		return nil, err
	}

	if approved {
		config.Grant(missing...)
	} else {
		return nil, fmt.Errorf("capabilities not approved")
	}

	return config, nil
}

// promptForCapabilities prompts the user to approve capabilities with a 30s timeout.
func promptForCapabilities(recipeName string, caps []recipestar.Capability) (bool, error) {
	fmt.Printf("\nRecipe %q requires:\n", recipeName)
	for _, cap := range caps {
		info := recipestar.GetCapabilityInfo(cap)
		if info != nil {
			riskColor := colorYellow
			if info.Risk == "high" {
				riskColor = colorRed
			} else if info.Risk == "low" {
				riskColor = colorGreen
			}
			fmt.Printf("  - %s: %s %s\n", color(colorBold, string(cap)), info.Description, color(riskColor, "("+info.Risk+" risk)"))
		} else {
			fmt.Printf("  - %s\n", cap)
		}
	}
	fmt.Printf("\nApprove? [y/N] (auto-deny in 30s): ")

	// Create a channel for the response
	responseCh := make(chan string, 1)

	go func() {
		var response string
		fmt.Scanln(&response)
		responseCh <- strings.TrimSpace(strings.ToLower(response))
	}()

	// Wait for response or timeout
	select {
	case response := <-responseCh:
		return response == "y" || response == "yes", nil
	case <-time.After(30 * time.Second):
		fmt.Println("\nTimeout - capabilities denied.")
		return false, nil
	}
}

// isTerminal returns true if stdout is a terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// terminalProgressReporter displays progress on a terminal.
type terminalProgressReporter struct {
	verbose bool
}

func newTerminalProgressReporter(verbose bool) *terminalProgressReporter {
	return &terminalProgressReporter{verbose: verbose}
}

func (t *terminalProgressReporter) RunStart(runID, recipeName string) {
	fmt.Printf("Starting run %s for %s\n", truncateID(runID, 8), recipeName)
}

func (t *terminalProgressReporter) RunComplete(runID string, success bool, durationMs int64) {
	if success {
		fmt.Printf("Run completed in %dms\n", durationMs)
	} else {
		fmt.Printf("Run failed after %dms\n", durationMs)
	}
}

func (t *terminalProgressReporter) RunError(runID string, err error) {
	fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
}

func (t *terminalProgressReporter) StepStart(stepName string, stepIndex, totalSteps int) {
	fmt.Printf("[%d/%d] %s...\n", stepIndex+1, totalSteps, stepName)
}

func (t *terminalProgressReporter) StepComplete(stepName string, success bool, description string) {
	if success {
		fmt.Printf("  Done: %s\n", description)
	}
}

func (t *terminalProgressReporter) FileStart(path string) {
	if t.verbose {
		fmt.Printf("  Processing: %s\n", path)
	}
}

func (t *terminalProgressReporter) FileChanged(path string) {
	if t.verbose {
		fmt.Printf("  Modified: %s\n", path)
	}
}

func (t *terminalProgressReporter) FileUnchanged(path string) {
	// Don't report unchanged files
}

func (t *terminalProgressReporter) FileSkipped(path, reason string) {
	if t.verbose {
		fmt.Printf("  Skipped: %s (%s)\n", path, reason)
	}
}

func (t *terminalProgressReporter) FileError(path string, err error) {
	fmt.Fprintf(os.Stderr, "  Error: %s: %v\n", path, err)
}

func (t *terminalProgressReporter) CheckpointCreated(checkpointID string, filesProcessed int) {
	if t.verbose {
		fmt.Printf("  Checkpoint created at %d files\n", filesProcessed)
	}
}

func (t *terminalProgressReporter) CheckpointRestored(checkpointID string) {
	fmt.Printf("Restored from checkpoint %s\n", truncateID(checkpointID, 8))
}

func (t *terminalProgressReporter) Info(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func (t *terminalProgressReporter) Warning(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

// jsonProgressReporter outputs progress events as JSON lines.
type jsonProgressReporter struct{}

func newJSONProgressReporter() *jsonProgressReporter {
	return &jsonProgressReporter{}
}

type progressEvent struct {
	Event   string `json:"event"`
	RunID   string `json:"run_id,omitempty"`
	Recipe  string `json:"recipe,omitempty"`
	Step    string `json:"step,omitempty"`
	File    string `json:"file,omitempty"`
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func (j *jsonProgressReporter) emitEvent(event string, data any) {
	e := progressEvent{Event: event, Data: data}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) RunStart(runID, recipeName string) {
	e := progressEvent{Event: "run_start", RunID: runID, Recipe: recipeName}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) RunComplete(runID string, success bool, durationMs int64) {
	e := progressEvent{Event: "run_complete", RunID: runID, Success: success, Data: map[string]int64{"duration_ms": durationMs}}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) RunError(runID string, err error) {
	e := progressEvent{Event: "run_error", RunID: runID, Message: err.Error()}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) StepStart(stepName string, stepIndex, totalSteps int) {
	e := progressEvent{Event: "step_start", Step: stepName, Data: map[string]int{"index": stepIndex, "total": totalSteps}}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) StepComplete(stepName string, success bool, description string) {
	e := progressEvent{Event: "step_complete", Step: stepName, Success: success, Message: description}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) FileStart(path string) {
	e := progressEvent{Event: "file_start", File: path}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) FileChanged(path string) {
	e := progressEvent{Event: "file_changed", File: path}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) FileUnchanged(path string) {
	e := progressEvent{Event: "file_unchanged", File: path}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) FileSkipped(path, reason string) {
	e := progressEvent{Event: "file_skipped", File: path, Message: reason}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) FileError(path string, err error) {
	e := progressEvent{Event: "file_error", File: path, Message: err.Error()}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) CheckpointCreated(checkpointID string, filesProcessed int) {
	e := progressEvent{Event: "checkpoint_created", Data: map[string]any{"checkpoint_id": checkpointID, "files_processed": filesProcessed}}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) CheckpointRestored(checkpointID string) {
	e := progressEvent{Event: "checkpoint_restored", Data: map[string]string{"checkpoint_id": checkpointID}}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) Info(format string, args ...any) {
	e := progressEvent{Event: "info", Message: fmt.Sprintf(format, args...)}
	json.NewEncoder(os.Stdout).Encode(e)
}

func (j *jsonProgressReporter) Warning(format string, args ...any) {
	e := progressEvent{Event: "warning", Message: fmt.Sprintf(format, args...)}
	json.NewEncoder(os.Stdout).Encode(e)
}

// convertStarlarkResult converts a starlark.Result to a recipe.Result.
func convertStarlarkResult(sr *recipestar.Result) *recipe.Result {
	result := &recipe.Result{
		Recipe:   sr.Recipe,
		Success:  sr.Success,
		StepsRun: sr.StepsRun,
		Error:    sr.Error,
	}

	for _, stepResult := range sr.StepResults {
		result.StepResults = append(result.StepResults, &recipe.StepResult{
			StepID:      stepResult.StepID,
			StepName:    stepResult.StepName,
			Success:     stepResult.Success,
			Description: stepResult.Description,
			Error:       stepResult.Error,
		})
	}

	for _, change := range sr.Changes {
		result.Changes = append(result.Changes, &recipe.FileChange{
			Path:        change.Path,
			Description: change.Description,
		})
	}

	return result
}

// recipeRunOutput is used for JSON serialization of recipe results
type recipeRunOutput struct {
	Recipe      string             `json:"recipe"`
	Success     bool               `json:"success"`
	StepsRun    int                `json:"steps_run"`
	StepResults []stepResultOutput `json:"step_results"`
	Changes     []changeInfoOutput `json:"changes"`
	Error       string             `json:"error,omitempty"`
}

type stepResultOutput struct {
	StepID      string `json:"step_id"`
	StepName    string `json:"step_name"`
	Success     bool   `json:"success"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
}

type changeInfoOutput struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

func outputRecipeRunJSON(result *recipe.Result) error {
	out := recipeRunOutput{
		Recipe:   result.Recipe,
		Success:  result.Success,
		StepsRun: result.StepsRun,
	}

	if result.Error != nil {
		out.Error = result.Error.Error()
	}

	for _, sr := range result.StepResults {
		sro := stepResultOutput{
			StepID:      sr.StepID,
			StepName:    sr.StepName,
			Success:     sr.Success,
			Description: sr.Description,
		}
		if sr.Error != nil {
			sro.Error = sr.Error.Error()
		}
		out.StepResults = append(out.StepResults, sro)
	}

	for _, c := range result.Changes {
		out.Changes = append(out.Changes, changeInfoOutput{
			Path:        c.Path,
			Description: c.Description,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputRecipeRunText(result *recipe.Result, dryRun bool) {
	modeStr := ""
	if dryRun {
		modeStr = " " + color(colorYellow, "(dry-run)")
	}

	fmt.Printf("%s Running recipe: %s%s\n\n", color(colorBlue, "==>"), color(colorBold, result.Recipe), modeStr)

	// Show step results
	for _, sr := range result.StepResults {
		status := color(colorGreen, "[OK]")
		if !sr.Success {
			status = color(colorRed, "[FAIL]")
		}

		fmt.Printf("  %s %s\n", status, sr.StepName)
		if sr.Description != "" && sr.Success {
			fmt.Printf("      %s\n", sr.Description)
		}
		if sr.Error != nil {
			fmt.Printf("      %s %s\n", color(colorRed, "error:"), sr.Error)
		}
	}

	// Show changes
	if len(result.Changes) > 0 {
		fmt.Printf("\n%s\n", color(colorBold, "Changes:"))
		for _, c := range result.Changes {
			if dryRun {
				fmt.Printf("  %s %s\n", color(colorYellow, "[would change]"), c.Path)
			} else {
				fmt.Printf("  %s %s\n", color(colorGreen, "[changed]"), c.Path)
			}
			fmt.Printf("      %s\n", c.Description)
		}
	}

	// Summary
	fmt.Println()
	if result.Success {
		if dryRun {
			if len(result.Changes) > 0 {
				fmt.Printf("%s Dry run complete. %d file(s) would be modified.\n",
					color(colorGreen, "[OK]"), len(result.Changes))
				fmt.Printf("    Run without --dry-run to apply changes.\n")
			} else {
				fmt.Printf("%s Dry run complete. No changes needed.\n", color(colorGreen, "[OK]"))
			}
		} else {
			if len(result.Changes) > 0 {
				fmt.Printf("%s Recipe completed successfully. %d file(s) modified.\n",
					color(colorGreen, "[OK]"), len(result.Changes))
			} else {
				fmt.Printf("%s Recipe completed successfully. No changes needed.\n",
					color(colorGreen, "[OK]"))
			}
		}
	} else {
		fmt.Printf("%s Recipe failed at step %d.\n", color(colorRed, "[FAIL]"), result.StepsRun)
		if result.Error != nil {
			fmt.Printf("    %s %s\n", color(colorRed, "error:"), result.Error)
		}
	}
}

// --- Diff-style output for Starlark recipe run ---

// diffJSONOutput is the JSON structure for recipe run with diff information.
type diffJSONOutput struct {
	Recipe  string           `json:"recipe"`
	DryRun  bool             `json:"dry_run"`
	Success bool             `json:"success"`
	Changes []diffJSONChange `json:"changes"`
	Summary diffJSONSummary  `json:"summary"`
	Error   string           `json:"error,omitempty"`
}

// diffJSONChange represents a single file change in JSON format.
type diffJSONChange struct {
	File  string         `json:"file"`
	Hunks []diffJSONHunk `json:"hunks"`
}

// diffJSONHunk represents a single hunk (diff region) in JSON format.
type diffJSONHunk struct {
	Line    int    `json:"line"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// diffJSONSummary provides summary statistics.
type diffJSONSummary struct {
	Files   int `json:"files"`
	Removed int `json:"removed"`
	Added   int `json:"added"`
}

// outputRecipeRunDiffJSON outputs the recipe result as JSON with diff information.
func outputRecipeRunDiffJSON(result *recipestar.Result, dryRun bool) error {
	out := diffJSONOutput{
		Recipe:  result.Recipe,
		DryRun:  dryRun,
		Success: result.Success,
		Changes: make([]diffJSONChange, 0),
	}

	if result.Error != nil {
		out.Error = result.Error.Error()
	}

	filesSet := make(map[string]bool)
	totalAdded := 0
	totalRemoved := 0

	for _, change := range result.Changes {
		if change == nil {
			continue
		}
		filesSet[change.Path] = true

		jsonChange := diffJSONChange{
			File:  change.Path,
			Hunks: make([]diffJSONHunk, 0),
		}

		for _, hunk := range change.Hunks {
			for _, line := range hunk.Lines {
				jsonChange.Hunks = append(jsonChange.Hunks, diffJSONHunk{
					Line:    line.LineNo,
					Type:    line.Type,
					Content: line.Content,
				})
				if line.Type == "add" {
					totalAdded++
				} else if line.Type == "remove" {
					totalRemoved++
				}
			}
		}

		out.Changes = append(out.Changes, jsonChange)
	}

	out.Summary = diffJSONSummary{
		Files:   len(filesSet),
		Removed: totalRemoved,
		Added:   totalAdded,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// outputRecipeRunDiffText outputs the recipe result with diff-style formatting.
func outputRecipeRunDiffText(result *recipestar.Result, dryRun, noColor bool) {
	colorFn := func(c, s string) string {
		if noColor || !useColor() {
			return s
		}
		return c + s + colorReset
	}

	modeStr := ""
	if dryRun {
		modeStr = " " + colorFn(colorYellow, "(dry-run)")
	}
	fmt.Printf("%s Running recipe: %s%s\n", colorFn(colorBlue, "==>"), colorFn(colorBold, result.Recipe), modeStr)

	if !result.Success {
		fmt.Printf("\n%s Recipe failed at step %d.\n", colorFn(colorRed, "[FAIL]"), result.StepsRun)
		if result.Error != nil {
			fmt.Printf("    %s %s\n", colorFn(colorRed, "error:"), result.Error)
		}
		return
	}

	if len(result.Changes) == 0 {
		fmt.Printf("\n%s No changes needed.\n", colorFn(colorGreen, "[OK]"))
		return
	}

	fmt.Println()
	totalAdded := 0
	totalRemoved := 0
	filesSet := make(map[string]bool)

	for _, change := range result.Changes {
		if change == nil {
			continue
		}
		filesSet[change.Path] = true

		fmt.Printf("%s\n", colorFn(colorBold, change.Path))
		lineLen := len(change.Path)
		if lineLen > 60 {
			lineLen = 60
		}
		fmt.Printf("%s\n", strings.Repeat("\u2500", lineLen))

		for _, hunk := range change.Hunks {
			fmt.Printf("%s\n", colorFn(colorCyan, fmt.Sprintf("@@ -%d,%d +%d,%d @@",
				hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount)))

			for _, line := range hunk.Lines {
				switch line.Type {
				case "add":
					fmt.Printf("%s %s\n", colorFn(colorGreen, "+"), colorFn(colorGreen, line.Content))
					totalAdded++
				case "remove":
					fmt.Printf("%s %s\n", colorFn(colorRed, "-"), colorFn(colorRed, line.Content))
					totalRemoved++
				case "context":
					fmt.Printf("  %s\n", line.Content)
				}
			}
		}
		fmt.Println()
	}

	fmt.Printf("%s\n", colorFn(colorBold, "Summary"))
	fmt.Printf("%s\n", strings.Repeat("\u2500", 7))
	fmt.Printf("  Files modified: %d\n", len(filesSet))
	fmt.Printf("  Lines removed:  %s\n", colorFn(colorRed, fmt.Sprintf("%d", totalRemoved)))
	fmt.Printf("  Lines added:    %s\n", colorFn(colorGreen, fmt.Sprintf("%d", totalAdded)))

	fmt.Println()
	if dryRun {
		fmt.Printf("Run without --dry-run to apply changes.\n")
	} else {
		fmt.Printf("%s Changes applied successfully.\n", colorFn(colorGreen, "[OK]"))
	}
}
