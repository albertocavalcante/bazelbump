// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.starlark.net/starlark"
)

// Pre-compiled regex patterns for BUILD file parsing.
// These are compiled once at package initialization for performance.
var (
	// loadPattern matches load statements: load("path", "sym1", "sym2", ...)
	loadPattern = regexp.MustCompile(`(?m)^load\s*\(\s*"([^"]+)"\s*,([^)]+)\)`)

	// symbolPattern matches quoted strings in load symbol lists
	symbolPattern = regexp.MustCompile(`"([^"]+)"`)

	// rulePattern matches rule invocations: rule_name(
	rulePattern = regexp.MustCompile(`(?m)^([a-z_][a-z0-9_]*)\s*\(`)

	// namePattern matches name = "value" attributes
	namePattern = regexp.MustCompile(`name\s*=\s*"([^"]+)"`)

	// attrPattern matches attr = "value" patterns
	attrPattern = regexp.MustCompile(`([a-z_][a-z0-9_]*)\s*=\s*"([^"]*)"`)
)

// BuildFile represents a parsed BUILD file.
type BuildFile struct {
	// Path is the path to the BUILD file.
	Path string

	// Content is the raw content of the BUILD file.
	Content string

	// ParsedRules are the rules found in the BUILD file.
	ParsedRules []*ParsedRule

	// ParsedLoads are the load statements found in the BUILD file.
	ParsedLoads []*ParsedLoad
}

// ParsedRule represents a rule invocation in a BUILD file.
type ParsedRule struct {
	// Kind is the rule type (e.g., "cc_binary", "py_library").
	Kind string

	// Name is the target name from the name attribute.
	Name string

	// Attributes are the parsed attributes of the rule.
	Attributes map[string]string

	// RawText is the raw text of the rule invocation.
	RawText string

	// Line is the 1-based line number where the rule starts.
	Line int
}

// ParsedLoad represents a load statement in a BUILD file.
type ParsedLoad struct {
	// Path is the load path (e.g., "@rules_cc//cc:defs.bzl").
	Path string

	// Symbols are the symbols loaded.
	Symbols []string

	// Line is the 1-based line number where the load statement is.
	Line int
}

// ParseBuildFile parses a BUILD file and returns a BuildFile object.
func ParseBuildFile(path string) (*BuildFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read BUILD file: %w", err)
	}

	return ParseBuildFileContent(path, string(content))
}

// ParseBuildFileContent parses BUILD file content and returns a BuildFile object.
func ParseBuildFileContent(path, content string) (*BuildFile, error) {
	bf := &BuildFile{
		Path:    path,
		Content: content,
	}

	// Parse load statements
	bf.ParsedLoads = parseLoadStatements(content)

	// Parse rules
	bf.ParsedRules = parseRules(content)

	return bf, nil
}

// parseLoadStatements extracts load statements from BUILD file content.
// Format: load("path", "sym1", "sym2", alias = "sym3")
func parseLoadStatements(content string) []*ParsedLoad {
	var loads []*ParsedLoad

	matches := loadPattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}

		path := content[match[2]:match[3]]
		symbolsStr := content[match[4]:match[5]]

		// Calculate line number
		line := strings.Count(content[:match[0]], "\n") + 1

		// Parse symbols
		symbols := parseLoadSymbols(symbolsStr)

		loads = append(loads, &ParsedLoad{
			Path:    path,
			Symbols: symbols,
			Line:    line,
		})
	}

	return loads
}

// parseLoadSymbols extracts symbol names from the symbols part of a load statement.
func parseLoadSymbols(symbolsStr string) []string {
	var symbols []string

	matches := symbolPattern.FindAllStringSubmatch(symbolsStr, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			symbols = append(symbols, match[1])
		}
	}

	return symbols
}

// parseRules extracts rule invocations from BUILD file content.
func parseRules(content string) []*ParsedRule {
	var rules []*ParsedRule

	matches := rulePattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		kind := content[match[2]:match[3]]

		// Skip load statements and some common non-rule functions
		if kind == "load" || kind == "package" || kind == "exports_files" ||
			kind == "glob" || kind == "select" {
			continue
		}

		// Find the matching closing parenthesis
		startParen := match[3]
		endParen := findMatchingParen(content, startParen)
		if endParen == -1 {
			continue
		}

		ruleBody := content[startParen:endParen]

		// Calculate line number
		line := strings.Count(content[:match[0]], "\n") + 1

		// Extract name attribute
		name := extractNameAttribute(ruleBody)

		// Extract all attributes
		attrs := extractAttributes(ruleBody)

		rules = append(rules, &ParsedRule{
			Kind:       kind,
			Name:       name,
			Attributes: attrs,
			RawText:    content[match[0] : endParen+1],
			Line:       line,
		})
	}

	return rules
}

// findMatchingParen finds the index of the closing parenthesis.
func findMatchingParen(content string, start int) int {
	depth := 1
	inString := false
	stringChar := byte(0)

	for i := start + 1; i < len(content); i++ {
		c := content[i]

		// Handle string literals
		if !inString && (c == '"' || c == '\'') {
			inString = true
			stringChar = c
			continue
		}
		if inString {
			if c == stringChar && (i == 0 || content[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Handle parentheses
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// extractNameAttribute extracts the name attribute value from a rule body.
func extractNameAttribute(body string) string {
	match := namePattern.FindStringSubmatch(body)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

// extractAttributes extracts all string attributes from a rule body.
func extractAttributes(body string) map[string]string {
	attrs := make(map[string]string)

	matches := attrPattern.FindAllStringSubmatch(body, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			attrs[match[1]] = match[2]
		}
	}

	return attrs
}

// ============================================================================
// Starlark Value Types for BUILD file parsing
// ============================================================================

// buildFileValue wraps a BuildFile for use as a Starlark value.
type buildFileValue struct {
	bf *BuildFile
}

func (b *buildFileValue) String() string        { return fmt.Sprintf("<BuildFile %s>", b.bf.Path) }
func (b *buildFileValue) Type() string          { return "BuildFile" }
func (b *buildFileValue) Freeze()               {}
func (b *buildFileValue) Truth() starlark.Bool  { return true }
func (b *buildFileValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: BuildFile") }

// Attr implements starlark.HasAttrs.
func (b *buildFileValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "rules":
		return starlark.NewBuiltin("rules", b.rulesMethod), nil
	case "loads":
		return starlark.NewBuiltin("loads", b.loadsMethod), nil
	case "path":
		return starlark.String(b.bf.Path), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("BuildFile has no attribute %q", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (b *buildFileValue) AttrNames() []string {
	return []string{"rules", "loads", "path"}
}

// rulesMethod implements BuildFile.rules(kind=None).
func (b *buildFileValue) rulesMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var kindFilter string

	if err := starlark.UnpackArgs("rules", args, kwargs,
		"kind?", &kindFilter,
	); err != nil {
		return nil, err
	}

	var result []starlark.Value
	for _, rule := range b.bf.ParsedRules {
		if kindFilter != "" && !matchKind(rule.Kind, kindFilter) {
			continue
		}
		result = append(result, &ruleValue{rule: rule})
	}

	return starlark.NewList(result), nil
}

// matchKind checks if a rule kind matches a pattern.
// Supports exact match and wildcard patterns like "cc_*".
func matchKind(kind, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(kind, prefix)
	}
	return kind == pattern
}

// loadsMethod implements BuildFile.loads().
func (b *buildFileValue) loadsMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("loads", args, kwargs); err != nil {
		return nil, err
	}

	var result []starlark.Value
	for _, load := range b.bf.ParsedLoads {
		result = append(result, &loadValue{load: load})
	}

	return starlark.NewList(result), nil
}

// ruleValue wraps a ParsedRule for use as a Starlark value.
type ruleValue struct {
	rule *ParsedRule
}

func (r *ruleValue) String() string        { return fmt.Sprintf("<Rule %s(%s)>", r.rule.Kind, r.rule.Name) }
func (r *ruleValue) Type() string          { return "Rule" }
func (r *ruleValue) Freeze()               {}
func (r *ruleValue) Truth() starlark.Bool  { return true }
func (r *ruleValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: Rule") }

// Attr implements starlark.HasAttrs.
func (r *ruleValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(r.rule.Name), nil
	case "kind":
		return starlark.String(r.rule.Kind), nil
	case "attr":
		return starlark.NewBuiltin("attr", r.attrMethod), nil
	case "line":
		return starlark.MakeInt(r.rule.Line), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("Rule has no attribute %q", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (r *ruleValue) AttrNames() []string {
	return []string{"name", "kind", "attr", "line"}
}

// attrMethod implements Rule.attr(name).
func (r *ruleValue) attrMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var attrName string

	if err := starlark.UnpackArgs("attr", args, kwargs,
		"name", &attrName,
	); err != nil {
		return nil, err
	}

	if value, ok := r.rule.Attributes[attrName]; ok {
		return starlark.String(value), nil
	}

	return starlark.None, nil
}

// loadValue wraps a ParsedLoad for use as a Starlark value.
type loadValue struct {
	load *ParsedLoad
}

func (l *loadValue) String() string        { return fmt.Sprintf("<Load %s>", l.load.Path) }
func (l *loadValue) Type() string          { return "Load" }
func (l *loadValue) Freeze()               {}
func (l *loadValue) Truth() starlark.Bool  { return true }
func (l *loadValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: Load") }

// Attr implements starlark.HasAttrs.
func (l *loadValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "path":
		return starlark.String(l.load.Path), nil
	case "symbols":
		var symbols []starlark.Value
		for _, sym := range l.load.Symbols {
			symbols = append(symbols, starlark.String(sym))
		}
		return starlark.NewList(symbols), nil
	case "line":
		return starlark.MakeInt(l.load.Line), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("Load has no attribute %q", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (l *loadValue) AttrNames() []string {
	return []string{"path", "symbols", "line"}
}

// Ensure types implement the HasAttrs interface.
var (
	_ starlark.HasAttrs = (*buildFileValue)(nil)
	_ starlark.HasAttrs = (*ruleValue)(nil)
	_ starlark.HasAttrs = (*loadValue)(nil)
)

// builtinParseBuildFile implements the parse_build_file() builtin.
// It parses a BUILD file and returns a BuildFile object that provides
// methods to query rules and load statements.
func builtinParseBuildFile(projectPath string) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var path string

		if err := starlark.UnpackArgs("parse_build_file", args, kwargs,
			"path", &path,
		); err != nil {
			return nil, err
		}

		// Resolve the path relative to project path if set
		resolvedPath := path
		if projectPath != "" && !filepath.IsAbs(path) {
			resolvedPath = filepath.Join(projectPath, path)
		}

		// Parse the BUILD file
		bf, err := ParseBuildFile(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("parse_build_file: %w", err)
		}

		return &buildFileValue{bf: bf}, nil
	}
}
