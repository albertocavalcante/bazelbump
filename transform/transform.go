// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package transform provides automated transformations for .bazelrc files
// to fix issues detected during Bazel upgrade analysis.
//
// The package supports three types of transforms:
//   - RemoveFlagTransform: Removes flags that no longer exist
//   - CommentFlagTransform: Comments out deprecated flags with explanations
//   - ReplaceFlagTransform: Replaces renamed flags with their new names
//
// Example usage:
//
//	patches, err := transform.GeneratePatches(issues)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Preview changes
//	if err := transform.ApplyPatches(patches, true); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Apply changes
//	if err := transform.ApplyPatches(patches, false); err != nil {
//	    log.Fatal(err)
//	}
package transform

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/albertocavalcante/bazelbump/analysis"
)

// Transform defines the interface for flag transformations.
type Transform interface {
	// Name returns a short identifier for the transform.
	Name() string
	// Description returns a human-readable description.
	Description() string
	// Apply applies the transform to file content and returns the modified content.
	Apply(content []byte) ([]byte, error)
	// CanApply returns true if this transform can handle the given issue.
	CanApply(issue *analysis.FlagIssue) bool
}

// Patch represents a single change to be applied to a file.
type Patch struct {
	File      string // Path to the file to modify
	OldLine   int    // Line number (1-indexed)
	OldText   string // Original line content
	NewText   string // New line content (empty means delete)
	Transform string // Name of the transform that generated this patch
}

// RemoveFlagTransform removes a flag from a .bazelrc file.
type RemoveFlagTransform struct {
	FlagName string // Flag to remove (without -- prefix)
}

// Name returns the transform identifier.
func (t *RemoveFlagTransform) Name() string {
	return "remove"
}

// Description returns a human-readable description.
func (t *RemoveFlagTransform) Description() string {
	return fmt.Sprintf("Remove flag --%s", t.FlagName)
}

// CanApply returns true for FlagRemoved issues matching the flag name.
func (t *RemoveFlagTransform) CanApply(issue *analysis.FlagIssue) bool {
	return issue.IssueType == analysis.FlagRemoved && issue.FlagName == t.FlagName
}

// Apply removes the flag from the content.
func (t *RemoveFlagTransform) Apply(content []byte) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	var result []string

	for _, line := range lines {
		newLine := t.removeFlagFromLine(line)
		if newLine != "" || !t.lineContainsOnlyFlag(line) {
			result = append(result, newLine)
		}
		// If line contained only the flag, we skip it entirely
	}

	return []byte(strings.Join(result, "\n")), nil
}

// removeFlagFromLine removes the flag from a single line.
// If the flag is the only content, returns empty string.
// If there are other flags, returns the line without this flag.
func (t *RemoveFlagTransform) removeFlagFromLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Skip empty lines and comments
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}

	// Build patterns to match the flag
	patterns := t.buildFlagPatterns()

	found := false
	result := line
	for _, pattern := range patterns {
		if pattern.MatchString(result) {
			found = true
			result = pattern.ReplaceAllString(result, "")
		}
	}

	if !found {
		return line
	}

	// Clean up extra whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	// If line is now just a command (e.g., "build"), it's effectively empty
	parts := strings.Fields(result)
	if len(parts) <= 1 {
		// Check if it's just a command or command:config
		if len(parts) == 1 && isCommand(parts[0]) {
			return ""
		}
	}

	return result
}

// lineContainsOnlyFlag checks if removing the flag would leave an empty line.
func (t *RemoveFlagTransform) lineContainsOnlyFlag(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}

	// After removing flag, check if only command remains
	result := t.removeFlagFromLine(line)
	result = strings.TrimSpace(result)

	if result == "" {
		return true
	}

	// Check if only a command remains
	parts := strings.Fields(result)
	return len(parts) == 1 && isCommand(parts[0])
}

// buildFlagPatterns builds regex patterns to match the flag.
func (t *RemoveFlagTransform) buildFlagPatterns() []*regexp.Regexp {
	// Escape special regex characters in flag name
	escaped := regexp.QuoteMeta(t.FlagName)

	patterns := []string{
		// --flag=value
		`--` + escaped + `=[^\s]*`,
		// --flag value (careful not to eat next flag)
		`--` + escaped + `(?:\s+[^-][^\s]*)?`,
		// --noflag
		`--no` + escaped + `\b`,
		// --flag (boolean)
		`--` + escaped + `\b`,
	}

	var compiled []*regexp.Regexp
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}

// CommentFlagTransform comments out a flag with an explanation.
type CommentFlagTransform struct {
	FlagName string // Flag to comment out (without -- prefix)
	Reason   string // Reason for commenting (e.g., "deprecated")
}

// Name returns the transform identifier.
func (t *CommentFlagTransform) Name() string {
	return "comment"
}

// Description returns a human-readable description.
func (t *CommentFlagTransform) Description() string {
	return fmt.Sprintf("Comment out flag --%s", t.FlagName)
}

// CanApply returns true for FlagDeprecated issues matching the flag name.
func (t *CommentFlagTransform) CanApply(issue *analysis.FlagIssue) bool {
	return issue.IssueType == analysis.FlagDeprecated && issue.FlagName == t.FlagName
}

// Apply comments out lines containing the flag.
func (t *CommentFlagTransform) Apply(content []byte) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	var result []string

	for _, line := range lines {
		if t.lineContainsFlag(line) {
			// Add deprecation comment before the line
			reason := t.Reason
			if reason == "" {
				reason = "deprecated"
			}
			result = append(result, fmt.Sprintf("# DEPRECATED: %s", reason))
			result = append(result, "# "+line)
		} else {
			result = append(result, line)
		}
	}

	return []byte(strings.Join(result, "\n")), nil
}

// lineContainsFlag checks if a line contains the flag.
func (t *CommentFlagTransform) lineContainsFlag(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Skip empty lines and comments
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}

	patterns := []string{
		`--` + regexp.QuoteMeta(t.FlagName) + `\b`,
		`--no` + regexp.QuoteMeta(t.FlagName) + `\b`,
	}

	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p, line); matched {
			return true
		}
	}

	return false
}

// ReplaceFlagTransform replaces one flag with another.
type ReplaceFlagTransform struct {
	OldFlag string // Flag to replace (without -- prefix)
	NewFlag string // Replacement flag (without -- prefix)
}

// Name returns the transform identifier.
func (t *ReplaceFlagTransform) Name() string {
	return "replace"
}

// Description returns a human-readable description.
func (t *ReplaceFlagTransform) Description() string {
	return fmt.Sprintf("Replace --%s with --%s", t.OldFlag, t.NewFlag)
}

// CanApply returns true if the issue suggests replacing this flag.
func (t *ReplaceFlagTransform) CanApply(issue *analysis.FlagIssue) bool {
	// Check if the issue is for our old flag and the suggestion mentions the new flag
	if issue.FlagName != t.OldFlag {
		return false
	}
	// Can apply to removed or deprecated flags that have a replacement
	return issue.IssueType == analysis.FlagRemoved || issue.IssueType == analysis.FlagDeprecated
}

// Apply replaces the old flag with the new flag.
func (t *ReplaceFlagTransform) Apply(content []byte) ([]byte, error) {
	oldEscaped := regexp.QuoteMeta(t.OldFlag)
	newFlag := t.NewFlag

	// Build replacement patterns
	replacements := []struct {
		pattern     *regexp.Regexp
		replacement string
	}{
		// --old_flag=value -> --new_flag=value
		{
			pattern:     regexp.MustCompile(`--` + oldEscaped + `=`),
			replacement: "--" + newFlag + "=",
		},
		// --noold_flag -> --nonew_flag
		{
			pattern:     regexp.MustCompile(`--no` + oldEscaped + `\b`),
			replacement: "--no" + newFlag,
		},
		// --old_flag -> --new_flag (standalone)
		{
			pattern:     regexp.MustCompile(`--` + oldEscaped + `\b`),
			replacement: "--" + newFlag,
		},
	}

	result := string(content)
	for _, r := range replacements {
		result = r.pattern.ReplaceAllString(result, r.replacement)
	}

	return []byte(result), nil
}

// GeneratePatches generates patches for the given issues.
// It selects appropriate transforms based on issue type and generates
// line-level patches that can be reviewed or applied.
func GeneratePatches(issues []*analysis.FlagIssue) ([]*Patch, error) {
	if len(issues) == 0 {
		return nil, nil
	}

	// Group issues by file
	byFile := make(map[string][]*analysis.FlagIssue)
	for _, issue := range issues {
		byFile[issue.File] = append(byFile[issue.File], issue)
	}

	var patches []*Patch

	for filePath, fileIssues := range byFile {
		// Read the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filePath, err)
		}

		lines := strings.Split(string(content), "\n")

		// Group issues by line number
		byLine := make(map[int][]*analysis.FlagIssue)
		for _, issue := range fileIssues {
			byLine[issue.Line] = append(byLine[issue.Line], issue)
		}

		// Generate patches for each line with issues
		for lineNum, lineIssues := range byLine {
			if lineNum < 1 || lineNum > len(lines) {
				continue
			}

			oldText := lines[lineNum-1]
			newText := oldText
			var transformNames []string

			for _, issue := range lineIssues {
				transform := selectTransform(issue)
				if transform == nil {
					continue
				}

				// Apply transform to current newText
				result, err := transform.Apply([]byte(newText))
				if err != nil {
					return nil, fmt.Errorf("apply %s to %s:%d: %w", transform.Name(), filePath, lineNum, err)
				}

				// For single-line transforms, extract just the relevant line
				resultLines := strings.Split(string(result), "\n")
				if len(resultLines) > 0 {
					// Handle comment transform which adds lines
					if _, ok := transform.(*CommentFlagTransform); ok {
						newText = string(result)
					} else {
						newText = resultLines[0]
					}
				}

				transformNames = append(transformNames, transform.Name())
			}

			// Only create patch if there's a change
			if newText != oldText {
				patches = append(patches, &Patch{
					File:      filePath,
					OldLine:   lineNum,
					OldText:   oldText,
					NewText:   newText,
					Transform: strings.Join(transformNames, ","),
				})
			}
		}
	}

	return patches, nil
}

// selectTransform selects the appropriate transform for an issue.
func selectTransform(issue *analysis.FlagIssue) Transform {
	switch issue.IssueType {
	case analysis.FlagRemoved:
		// Check if suggestion mentions a replacement
		if replacement := extractReplacement(issue.Suggestion); replacement != "" {
			return &ReplaceFlagTransform{
				OldFlag: issue.FlagName,
				NewFlag: replacement,
			}
		}
		return &RemoveFlagTransform{FlagName: issue.FlagName}

	case analysis.FlagDeprecated:
		// Check if suggestion mentions a replacement
		if replacement := extractReplacement(issue.Suggestion); replacement != "" {
			return &ReplaceFlagTransform{
				OldFlag: issue.FlagName,
				NewFlag: replacement,
			}
		}
		return &CommentFlagTransform{
			FlagName: issue.FlagName,
			Reason:   issue.Suggestion,
		}

	case analysis.FlagDefaultChanged:
		// Default changed flags typically don't need automatic transformation
		// User should review and decide
		return nil
	}

	return nil
}

// extractReplacement attempts to extract a replacement flag from a suggestion.
// Returns empty string if no replacement is suggested.
func extractReplacement(suggestion string) string {
	// Look for patterns like "use --new_flag instead" or "replaced by --new_flag"
	patterns := []string{
		`use --([a-z_][a-z0-9_]*) instead`,
		`replaced by --([a-z_][a-z0-9_]*)`,
		`migrate to --([a-z_][a-z0-9_]*)`,
		`->.*--([a-z_][a-z0-9_]*)`,
	}

	for _, p := range patterns {
		re := regexp.MustCompile(`(?i)` + p)
		if matches := re.FindStringSubmatch(suggestion); len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

// ApplyPatches applies the given patches to files.
// If dryRun is true, prints what would be changed without modifying files.
func ApplyPatches(patches []*Patch, dryRun bool) error {
	if len(patches) == 0 {
		return nil
	}

	// Group patches by file
	byFile := make(map[string][]*Patch)
	for _, patch := range patches {
		byFile[patch.File] = append(byFile[patch.File], patch)
	}

	for filePath, filePatches := range byFile {
		if dryRun {
			fmt.Printf("--- %s\n", filePath)
			fmt.Printf("+++ %s\n", filePath)
			for _, patch := range filePatches {
				fmt.Printf("@@ -%d +%d @@\n", patch.OldLine, patch.OldLine)
				fmt.Printf("-%s\n", patch.OldText)
				if patch.NewText != "" {
					// Handle multi-line new text (from comment transform)
					for _, line := range strings.Split(patch.NewText, "\n") {
						fmt.Printf("+%s\n", line)
					}
				}
			}
			continue
		}

		// Read the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", filePath, err)
		}

		// Apply patches
		lines := strings.Split(string(content), "\n")
		newLines, err := applyPatchesToLines(lines, filePatches)
		if err != nil {
			return fmt.Errorf("apply patches to %s: %w", filePath, err)
		}

		// Write back
		newContent := strings.Join(newLines, "\n")
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("write %s: %w", filePath, err)
		}
	}

	return nil
}

// applyPatchesToLines applies patches to lines, handling line additions/deletions.
func applyPatchesToLines(lines []string, patches []*Patch) ([]string, error) {
	// Sort patches by line number in reverse order
	// This way we can apply patches without invalidating line numbers
	sortedPatches := make([]*Patch, len(patches))
	copy(sortedPatches, patches)

	// Simple reverse sort by line number
	for i := 0; i < len(sortedPatches)-1; i++ {
		for j := i + 1; j < len(sortedPatches); j++ {
			if sortedPatches[i].OldLine < sortedPatches[j].OldLine {
				sortedPatches[i], sortedPatches[j] = sortedPatches[j], sortedPatches[i]
			}
		}
	}

	result := make([]string, len(lines))
	copy(result, lines)

	for _, patch := range sortedPatches {
		if patch.OldLine < 1 || patch.OldLine > len(result) {
			return nil, fmt.Errorf("invalid line number %d", patch.OldLine)
		}

		idx := patch.OldLine - 1

		if patch.NewText == "" {
			// Delete the line
			result = append(result[:idx], result[idx+1:]...)
		} else if strings.Contains(patch.NewText, "\n") {
			// Multi-line replacement (e.g., from comment transform)
			newLines := strings.Split(patch.NewText, "\n")
			// Replace single line with multiple lines
			// We must copy slices to avoid sharing the underlying array
			// which would cause corruption when append modifies it
			before := make([]string, idx)
			copy(before, result[:idx])
			after := make([]string, len(result)-idx-1)
			copy(after, result[idx+1:])
			result = append(before, newLines...)
			result = append(result, after...)
		} else {
			// Simple replacement
			result[idx] = patch.NewText
		}
	}

	return result, nil
}

// isCommand checks if a string is a bazelrc command.
func isCommand(s string) bool {
	// Remove config suffix if present
	cmd := s
	if idx := strings.Index(s, ":"); idx != -1 {
		cmd = s[:idx]
	}

	commands := []string{
		"build", "test", "run", "clean", "mobile-install",
		"info", "print_action", "config", "cquery", "aquery",
		"coverage", "fetch", "vendor", "common", "always",
		"startup", "import", "try-import", "try-import-if-bazel-version",
	}

	for _, c := range commands {
		if strings.EqualFold(cmd, c) {
			return true
		}
	}

	return false
}

// PrintPatches prints patches in a human-readable format.
func PrintPatches(patches []*Patch) {
	for _, p := range patches {
		fmt.Printf("%s:%d [%s]\n", p.File, p.OldLine, p.Transform)
		fmt.Printf("  - %s\n", p.OldText)
		if p.NewText == "" {
			fmt.Printf("  (delete line)\n")
		} else {
			for _, line := range strings.Split(p.NewText, "\n") {
				fmt.Printf("  + %s\n", line)
			}
		}
		fmt.Println()
	}
}

// ReadFileLines reads a file and returns its lines.
func ReadFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
