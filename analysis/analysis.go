// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package analysis provides Bazel upgrade impact analysis.
//
// It scans .bazelrc files in a project and identifies flags that may be
// affected when upgrading from one Bazel version to another. Issues detected
// include removed flags, deprecated flags, and flags whose default values
// have changed.
//
// Example usage:
//
//	db, _ := flagdb.Load("data/flags")
//	analyzer := analysis.New(
//	    analysis.WithProjectPath("/path/to/project"),
//	    analysis.WithFromVersion("8.0.0"),
//	    analysis.WithToVersion("9.0.0"),
//	    analysis.WithFlagDB(db),
//	)
//	result, err := analyzer.Analyze()
//	for _, issue := range result.FlagIssues {
//	    fmt.Printf("%s:%d: %s %s\n", issue.File, issue.Line, issue.IssueType, issue.FlagName)
//	}
package analysis

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/albertocavalcante/bazelbump/bazelrc"
	"github.com/albertocavalcante/bazelbump/flagdb"
)

// IssueType categorizes the type of flag issue.
type IssueType string

const (
	// FlagRemoved indicates the flag doesn't exist in the target version.
	FlagRemoved IssueType = "removed"
	// FlagDeprecated indicates the flag is deprecated in the target version.
	FlagDeprecated IssueType = "deprecated"
	// FlagDefaultChanged indicates the flag's default value changed.
	FlagDefaultChanged IssueType = "default_changed"
)

// FlagIssue represents a potential issue with a flag used in a .bazelrc file.
type FlagIssue struct {
	FlagName   string    // Name of the flag (without -- prefix)
	IssueType  IssueType // Type of issue detected
	File       string    // Path to the .bazelrc file
	Line       int       // Line number in the file (1-indexed)
	Command    string    // Bazel command (build, test, etc.)
	Config     string    // Config name (empty for default)
	RawFlag    string    // Original flag string including value
	Suggestion string    // Actionable suggestion for resolving the issue
	OldDefault string    // Previous default value (for default_changed)
	NewDefault string    // New default value (for default_changed)
}

// AnalysisResult contains the results of analyzing a project for Bazel upgrade issues.
type AnalysisResult struct {
	ProjectPath  string      // Path to the analyzed project
	FromVersion  string      // Source Bazel version
	ToVersion    string      // Target Bazel version
	FlagIssues   []FlagIssue // List of flag issues found
	FilesScanned int         // Number of .bazelrc files scanned
}

// Analyzer performs Bazel upgrade impact analysis on a project.
type Analyzer struct {
	projectPath string
	fromVersion string
	toVersion   string
	flagDB      *flagdb.DB
}

// Option configures an Analyzer.
type Option func(*Analyzer)

// WithProjectPath sets the project path to analyze.
func WithProjectPath(path string) Option {
	return func(a *Analyzer) {
		a.projectPath = path
	}
}

// WithFromVersion sets the source Bazel version.
func WithFromVersion(version string) Option {
	return func(a *Analyzer) {
		a.fromVersion = version
	}
}

// WithToVersion sets the target Bazel version.
func WithToVersion(version string) Option {
	return func(a *Analyzer) {
		a.toVersion = version
	}
}

// WithFlagDB sets the flag database to use.
func WithFlagDB(db *flagdb.DB) Option {
	return func(a *Analyzer) {
		a.flagDB = db
	}
}

// New creates a new Analyzer with the given options.
func New(opts ...Option) *Analyzer {
	a := &Analyzer{
		projectPath: ".",
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Analyze performs the upgrade impact analysis and returns the results.
func (a *Analyzer) Analyze() (*AnalysisResult, error) {
	if a.flagDB == nil {
		return nil, fmt.Errorf("flag database is required")
	}
	if a.fromVersion == "" {
		return nil, fmt.Errorf("from version is required")
	}
	if a.toVersion == "" {
		return nil, fmt.Errorf("to version is required")
	}

	// Compute diff between versions
	diff, err := a.flagDB.Diff(a.fromVersion, a.toVersion)
	if err != nil {
		return nil, fmt.Errorf("compute version diff: %w", err)
	}

	// Build lookup maps for fast access
	removedFlags := make(map[string]*flagdb.Flag)
	for i := range diff.Removed {
		removedFlags[diff.Removed[i].Name] = &diff.Removed[i]
	}

	changedFlags := make(map[string]*flagdb.FlagChange)
	deprecatedFlags := make(map[string]*flagdb.FlagChange)
	defaultChangedFlags := make(map[string]*flagdb.FlagChange)

	for i := range diff.Changed {
		change := &diff.Changed[i]
		changedFlags[change.Name] = change
		switch change.Change {
		case flagdb.ChangeDeprecated:
			deprecatedFlags[change.Name] = change
		case flagdb.ChangeDefaultValue:
			defaultChangedFlags[change.Name] = change
		}
	}

	result := &AnalysisResult{
		ProjectPath: a.projectPath,
		FromVersion: a.fromVersion,
		ToVersion:   a.toVersion,
	}

	// Find all .bazelrc files
	bazelrcFiles, err := a.findBazelrcFiles()
	if err != nil {
		return nil, fmt.Errorf("find bazelrc files: %w", err)
	}

	result.FilesScanned = len(bazelrcFiles)

	// Analyze each file
	for _, filePath := range bazelrcFiles {
		issues, err := a.analyzeFile(filePath, removedFlags, deprecatedFlags, defaultChangedFlags)
		if err != nil {
			return nil, fmt.Errorf("analyze %s: %w", filePath, err)
		}
		result.FlagIssues = append(result.FlagIssues, issues...)
	}

	return result, nil
}

// findBazelrcFiles finds all .bazelrc files in the project.
func (a *Analyzer) findBazelrcFiles() ([]string, error) {
	var files []string

	err := filepath.WalkDir(a.projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (except the root if it starts with .)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != a.projectPath {
			return filepath.SkipDir
		}

		// Match .bazelrc files
		name := d.Name()
		if !d.IsDir() && (name == ".bazelrc" || strings.HasSuffix(name, ".bazelrc")) {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// analyzeFile analyzes a single .bazelrc file for flag issues.
func (a *Analyzer) analyzeFile(
	filePath string,
	removedFlags map[string]*flagdb.Flag,
	deprecatedFlags map[string]*flagdb.FlagChange,
	defaultChangedFlags map[string]*flagdb.FlagChange,
) ([]FlagIssue, error) {
	file, err := bazelrc.ParseFile(filePath)
	if err != nil {
		// Skip files that can't be parsed (may not exist or be invalid)
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var issues []FlagIssue

	for _, line := range file.Lines {
		if line.IsEmpty || len(line.Flags) == 0 {
			continue
		}

		for _, flag := range line.Flags {
			flagName := extractFlagName(flag)
			if flagName == "" {
				continue
			}

			// Check for removed flags
			if removed, ok := removedFlags[flagName]; ok {
				issues = append(issues, FlagIssue{
					FlagName:   flagName,
					IssueType:  FlagRemoved,
					File:       filePath,
					Line:       line.Number,
					Command:    line.Command,
					Config:     line.Config,
					RawFlag:    flag,
					Suggestion: a.generateRemovalSuggestion(removed),
				})
				continue
			}

			// Check for deprecated flags
			if change, ok := deprecatedFlags[flagName]; ok {
				issues = append(issues, FlagIssue{
					FlagName:   flagName,
					IssueType:  FlagDeprecated,
					File:       filePath,
					Line:       line.Number,
					Command:    line.Command,
					Config:     line.Config,
					RawFlag:    flag,
					Suggestion: a.generateDeprecationSuggestion(change),
				})
				continue
			}

			// Check for default value changes (incompatible flags)
			if change, ok := defaultChangedFlags[flagName]; ok {
				// Only report if this is an incompatible flag, as those are most impactful
				if change.To.IsIncompatibleChange() {
					issues = append(issues, FlagIssue{
						FlagName:   flagName,
						IssueType:  FlagDefaultChanged,
						File:       filePath,
						Line:       line.Number,
						Command:    line.Command,
						Config:     line.Config,
						RawFlag:    flag,
						Suggestion: a.generateDefaultChangeSuggestion(change),
						OldDefault: change.From.Default,
						NewDefault: change.To.Default,
					})
				}
			}
		}
	}

	return issues, nil
}

// extractFlagName extracts the flag name from a flag string.
// e.g., "--foo=bar" -> "foo", "--nofoo" -> "foo", "-k" -> "k"
func extractFlagName(flag string) string {
	// Remove leading dashes
	s := strings.TrimLeft(flag, "-")
	if s == "" {
		return ""
	}

	// Handle --no prefix for boolean flags
	if strings.HasPrefix(s, "no") && len(s) > 2 {
		rest := s[2:]
		if len(rest) > 0 && rest[0] >= 'a' && rest[0] <= 'z' {
			s = rest
		}
	}

	// Extract name before = or space
	if idx := strings.IndexAny(s, "= "); idx != -1 {
		s = s[:idx]
	}

	return s
}

// generateRemovalSuggestion generates a suggestion for a removed flag.
func (a *Analyzer) generateRemovalSuggestion(flag *flagdb.Flag) string {
	if flag.IsIncompatibleChange() {
		return fmt.Sprintf("Remove --%s; the incompatible behavior is now the default in Bazel %s", flag.Name, a.toVersion)
	}
	if flag.IsExperimental() {
		return fmt.Sprintf("Remove --%s; this experimental flag was removed in Bazel %s", flag.Name, a.toVersion)
	}
	return fmt.Sprintf("Remove --%s; this flag was removed in Bazel %s", flag.Name, a.toVersion)
}

// generateDeprecationSuggestion generates a suggestion for a deprecated flag.
func (a *Analyzer) generateDeprecationSuggestion(change *flagdb.FlagChange) string {
	if change.To.Description != "" && strings.Contains(strings.ToLower(change.To.Description), "deprecated") {
		return fmt.Sprintf("Flag --%s is deprecated in Bazel %s: %s", change.Name, a.toVersion, change.To.Description)
	}
	return fmt.Sprintf("Flag --%s is deprecated in Bazel %s; consider removing it", change.Name, a.toVersion)
}

// generateDefaultChangeSuggestion generates a suggestion for a default value change.
func (a *Analyzer) generateDefaultChangeSuggestion(change *flagdb.FlagChange) string {
	if change.From.Default != change.To.Default {
		return fmt.Sprintf(
			"Default for --%s changed from %q to %q in Bazel %s; verify your code works with the new default or set explicitly",
			change.Name, change.From.Default, change.To.Default, a.toVersion,
		)
	}
	return fmt.Sprintf("Review --%s; its behavior may have changed in Bazel %s", change.Name, a.toVersion)
}

// HasIssues returns true if the analysis found any issues.
func (r *AnalysisResult) HasIssues() bool {
	return len(r.FlagIssues) > 0
}

// IssuesByType returns issues filtered by the given type.
func (r *AnalysisResult) IssuesByType(t IssueType) []FlagIssue {
	var filtered []FlagIssue
	for _, issue := range r.FlagIssues {
		if issue.IssueType == t {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

// IssuesByFile returns issues grouped by file path.
func (r *AnalysisResult) IssuesByFile() map[string][]FlagIssue {
	byFile := make(map[string][]FlagIssue)
	for _, issue := range r.FlagIssues {
		byFile[issue.File] = append(byFile[issue.File], issue)
	}
	return byFile
}
