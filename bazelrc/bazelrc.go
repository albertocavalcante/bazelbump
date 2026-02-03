// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package bazelrc provides parsing for .bazelrc configuration files.
//
// The .bazelrc format is documented at https://bazel.build/run/bazelrc
//
// Example usage:
//
//	file, err := bazelrc.ParseFile(".bazelrc")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get all flags for the "build" command
//	flags := file.FlagsFor("build", "")
//
//	// Get flags for a specific config
//	flags := file.FlagsFor("build", "opt")
package bazelrc

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// File represents a parsed .bazelrc file.
type File struct {
	Path    string
	Lines   []Line
	Imports []Import
}

// Line represents a single line in a .bazelrc file.
type Line struct {
	Number  int      // 1-indexed line number
	Raw     string   // Original line content
	Command string   // Command (build, test, startup, common, always)
	Config  string   // Config name (empty for default)
	Flags   []string // Flag values
	Comment string   // Trailing comment
	IsEmpty bool     // Empty or whitespace-only line
}

// Import represents an import directive.
type Import struct {
	Line     int
	Path     string
	Optional bool  // try-import vs import
	Version  *VersionCondition // try-import-if-bazel-version
}

// VersionCondition represents a Bazel version condition for imports.
type VersionCondition struct {
	Operator string // >, >=, <, <=, ==, !=, ~
	Version  string
}

// ParseFile parses a .bazelrc file from the given path.
func ParseFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	file, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	file.Path = path
	return file, nil
}

// Parse parses a .bazelrc file from an io.Reader.
func Parse(r io.Reader) (*File, error) {
	file := &File{}
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		line := parseLine(lineNum, raw)
		file.Lines = append(file.Lines, line)

		// Extract imports
		if imp := extractImport(line); imp != nil {
			file.Imports = append(file.Imports, *imp)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return file, nil
}

// parseLine parses a single line of a .bazelrc file.
func parseLine(num int, raw string) Line {
	line := Line{
		Number: num,
		Raw:    raw,
	}

	// Trim and check for empty/comment
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		line.IsEmpty = true
		return line
	}
	if strings.HasPrefix(trimmed, "#") {
		line.IsEmpty = true
		line.Comment = trimmed
		return line
	}

	// Extract trailing comment
	content := trimmed
	if idx := strings.Index(trimmed, " #"); idx != -1 {
		content = strings.TrimSpace(trimmed[:idx])
		line.Comment = trimmed[idx+1:]
	}

	// Tokenize (simplified - doesn't handle all shell quoting)
	tokens := tokenize(content)
	if len(tokens) == 0 {
		line.IsEmpty = true
		return line
	}

	// First token is command[:config]
	cmdConfig := tokens[0]
	if idx := strings.Index(cmdConfig, ":"); idx != -1 {
		line.Command = cmdConfig[:idx]
		line.Config = cmdConfig[idx+1:]
	} else {
		line.Command = cmdConfig
	}

	// Remaining tokens are flags
	if len(tokens) > 1 {
		line.Flags = tokens[1:]
	}

	return line
}

// tokenize splits a line into tokens, respecting quotes.
func tokenize(s string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range s {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case r == ' ' || r == '\t':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// extractImport extracts an Import from a line if applicable.
func extractImport(line Line) *Import {
	cmd := strings.ToLower(line.Command)

	switch {
	case cmd == "import":
		if len(line.Flags) > 0 {
			return &Import{
				Line:     line.Number,
				Path:     line.Flags[0],
				Optional: false,
			}
		}
	case cmd == "try-import":
		if len(line.Flags) > 0 {
			return &Import{
				Line:     line.Number,
				Path:     line.Flags[0],
				Optional: true,
			}
		}
	case cmd == "try-import-if-bazel-version":
		if len(line.Flags) >= 2 {
			cond := parseVersionCondition(line.Flags[0])
			return &Import{
				Line:     line.Number,
				Path:     line.Flags[1],
				Optional: true,
				Version:  cond,
			}
		}
	}

	return nil
}

// parseVersionCondition parses a version condition like ">=7.0.0" or "~8.0".
func parseVersionCondition(s string) *VersionCondition {
	for _, op := range []string{">=", "<=", "==", "!=", ">", "<", "~"} {
		if strings.HasPrefix(s, op) {
			return &VersionCondition{
				Operator: op,
				Version:  strings.TrimPrefix(s, op),
			}
		}
	}
	return nil
}

// FlagsFor returns all flags that apply to the given command and config.
// If config is empty, returns flags for the default (no config) lines.
// Results include inherited flags from parent commands.
func (f *File) FlagsFor(command, config string) []string {
	var flags []string

	// Inheritance hierarchy
	parents := commandParents(command)

	for _, line := range f.Lines {
		if line.IsEmpty || len(line.Flags) == 0 {
			continue
		}

		// Check if this line applies
		applies := false
		lineCmd := strings.ToLower(line.Command)

		// common applies to all
		if lineCmd == "common" || lineCmd == "always" {
			if config == "" || line.Config == "" || line.Config == config {
				applies = true
			}
		}

		// Direct command match
		if lineCmd == command {
			if config == "" && line.Config == "" {
				applies = true
			} else if config != "" && line.Config == config {
				applies = true
			}
		}

		// Parent command match (inheritance)
		for _, parent := range parents {
			if lineCmd == parent {
				if config == "" && line.Config == "" {
					applies = true
				} else if config != "" && line.Config == config {
					applies = true
				}
			}
		}

		if applies {
			flags = append(flags, line.Flags...)
		}
	}

	return flags
}

// commandParents returns the parent commands that a command inherits from.
func commandParents(cmd string) []string {
	// Inheritance hierarchy per Bazel docs
	hierarchy := map[string][]string{
		"test":           {"build"},
		"run":            {"build"},
		"clean":          {"build"},
		"mobile-install": {"build"},
		"info":           {"build"},
		"print_action":   {"build"},
		"config":         {"build"},
		"cquery":         {"build"},
		"aquery":         {"build"},
		"coverage":       {"test", "build"},
		"fetch":          {"test", "build"},
		"vendor":         {"test", "build"},
	}

	if parents, ok := hierarchy[cmd]; ok {
		return parents
	}
	return nil
}

// AllConfigs returns all unique config names defined in the file.
func (f *File) AllConfigs() []string {
	seen := make(map[string]bool)
	var configs []string

	for _, line := range f.Lines {
		if line.Config != "" && !seen[line.Config] {
			seen[line.Config] = true
			configs = append(configs, line.Config)
		}
	}

	return configs
}

// AllFlags returns all unique flag names used in the file.
func (f *File) AllFlags() []string {
	seen := make(map[string]bool)
	var flags []string

	for _, line := range f.Lines {
		for _, flag := range line.Flags {
			name := extractFlagName(flag)
			if name != "" && !seen[name] {
				seen[name] = true
				flags = append(flags, name)
			}
		}
	}

	return flags
}

// extractFlagName extracts the flag name from a flag string.
// e.g., "--foo=bar" -> "foo", "--nofoo" -> "foo", "-k" -> "k"
func extractFlagName(flag string) string {
	// Remove leading dashes
	s := strings.TrimLeft(flag, "-")
	if s == "" {
		return ""
	}

	// Handle --no prefix
	if strings.HasPrefix(s, "no") && len(s) > 2 {
		// Check if it's a negated boolean flag
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

// FindFlag finds all occurrences of a flag in the file.
func (f *File) FindFlag(name string) []FlagOccurrence {
	var occurrences []FlagOccurrence

	for _, line := range f.Lines {
		for _, flag := range line.Flags {
			flagName := extractFlagName(flag)
			if flagName == name {
				occurrences = append(occurrences, FlagOccurrence{
					Line:    line.Number,
					Command: line.Command,
					Config:  line.Config,
					Flag:    flag,
				})
			}
		}
	}

	return occurrences
}

// FlagOccurrence represents where a flag appears in a bazelrc file.
type FlagOccurrence struct {
	Line    int
	Command string
	Config  string
	Flag    string // Full flag string including value
}
