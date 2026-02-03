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

	"github.com/albertocavalcante/bazelbump/analysis"
	"github.com/albertocavalcante/bazelbump/flagdb"
	"github.com/albertocavalcante/bazelbump/recipe"
	recipestar "github.com/albertocavalcante/bazelbump/recipe/starlark"
	"github.com/albertocavalcante/bazelbump/transform"
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
  apply     Apply transforms to fix detected flag issues
  diff      Show flag changes between Bazel versions
  flags     List flags for a Bazel version
  recipe    Run migration recipes
  version   Show bazelbump version

%s
  bazelbump analyze --to 9.0.0
  bazelbump analyze --from 8.0.0 --to 9.0.0 --path /path/to/project
  bazelbump apply --to 9.0.0 --dry-run
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

%s
  bazelbump recipe <subcommand> [flags]

%s
  list    List available recipes
  show    Show recipe details
  run     Run a recipe

%s
  bazelbump recipe list
  bazelbump recipe show autoload-migration
  bazelbump recipe run autoload-migration --dry-run
  bazelbump recipe run autoload-migration --step detect-native-rules

Use "bazelbump recipe <subcommand> --help" for more information.
`, color(colorBold, "Usage:"),
		color(colorBold, "Subcommands:"),
		color(colorBold, "Examples:"))
}

func runRecipeList(args []string) error {
	fs := flag.NewFlagSet("recipe list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `List available recipes.

%s
  bazelbump recipe list [flags]

%s
  --json   Output results as JSON
  --help   Show this help
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"))
	}

	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	engine := recipe.NewEngine()
	recipes := engine.List()

	if *jsonOutput {
		return outputRecipeListJSON(recipes)
	}

	outputRecipeListText(recipes)
	return nil
}

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
  bazelbump recipe show <name> [flags]

%s
  --json   Output results as JSON
  --help   Show this help

%s
  bazelbump recipe show autoload-migration
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("recipe name required")
	}

	name := fs.Arg(0)

	engine := recipe.NewEngine()
	r, ok := engine.Get(name)
	if !ok {
		return fmt.Errorf("unknown recipe: %s", name)
	}

	if *jsonOutput {
		return outputRecipeShowJSON(r)
	}

	outputRecipeShowText(r)
	return nil
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

%s
  bazelbump recipe run <name> [flags]

%s
  --dry-run    Show what would change without applying
  --path       Path to project directory (default: current directory)
  --step       Run only a specific step (by ID)
  --verbose    Show verbose output
  --json       Output results as JSON
  --help       Show this help

%s
  bazelbump recipe run autoload-migration
  bazelbump recipe run autoload-migration --dry-run
  bazelbump recipe run autoload-migration --path /path/to/project
  bazelbump recipe run autoload-migration --step detect-native-rules
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	dryRun := fs.Bool("dry-run", false, "Show what would change without applying")
	projectPath := fs.String("path", ".", "Path to project directory")
	stepID := fs.String("step", "", "Run only a specific step")
	verbose := fs.Bool("verbose", false, "Show verbose output")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("recipe name required")
	}

	name := fs.Arg(0)

	// Check if this is a .star file (Starlark recipe)
	if strings.HasSuffix(name, ".star") || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		return runStarlarkRecipe(name, *projectPath, *dryRun, *stepID, *verbose, *jsonOutput)
	}

	// Fall back to built-in Go recipes
	engine := recipe.NewEngine()

	// Verify recipe exists
	if _, ok := engine.Get(name); !ok {
		// Try to find a .star file with this name
		starPath := name + ".star"
		if _, err := os.Stat(starPath); err == nil {
			return runStarlarkRecipe(starPath, *projectPath, *dryRun, *stepID, *verbose, *jsonOutput)
		}
		// Check in examples/recipes
		examplesPath := filepath.Join("examples", "recipes", name+".star")
		if _, err := os.Stat(examplesPath); err == nil {
			return runStarlarkRecipe(examplesPath, *projectPath, *dryRun, *stepID, *verbose, *jsonOutput)
		}
		return fmt.Errorf("unknown recipe: %s (not a built-in recipe or .star file)", name)
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

// runStarlarkRecipe runs a Starlark .star recipe file.
func runStarlarkRecipe(path, projectPath string, dryRun bool, stepID string, verbose, jsonOutput bool) error {
	// Resolve path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check file exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("recipe file not found: %s", path)
	}

	// Create executor
	executor := recipestar.NewExecutor(projectPath, recipestar.Options{
		DryRun:  dryRun,
		Verbose: verbose,
		StepID:  stepID,
	})

	// Execute the recipe
	result, err := executor.ExecuteFromFile(absPath, recipestar.Options{
		DryRun:  dryRun,
		Verbose: verbose,
		StepID:  stepID,
	})
	if err != nil {
		return fmt.Errorf("recipe execution failed: %w", err)
	}

	// Convert to recipe.Result for output compatibility
	recipeResult := convertStarlarkResult(result)

	if jsonOutput {
		return outputRecipeRunJSON(recipeResult)
	}

	outputRecipeRunText(recipeResult, dryRun)

	if !result.Success {
		os.Exit(1)
	}
	return nil
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
