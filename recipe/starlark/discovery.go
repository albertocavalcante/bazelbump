// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Pre-compiled regex patterns for recipe metadata extraction.
var (
	descriptionPattern = regexp.MustCompile(`description\s*=\s*"([^"]*)"`)
	versionMetaPattern = regexp.MustCompile(`version\s*=\s*"([^"]*)"`)
)

// DiscoveredRecipe represents a recipe found during discovery.
type DiscoveredRecipe struct {
	// Name is the recipe name (filename without .star extension)
	Name string

	// Path is the absolute path to the .star file
	Path string

	// Source indicates where the recipe was found:
	// "explicit" - provided directly as a path
	// "project" - found in project recipes directory
	// "user" - found in user config directory
	// "builtin" - embedded in the binary
	Source string

	// Description is extracted from recipe metadata if available
	Description string

	// Version is extracted from recipe metadata if available
	Version string
}

// DiscoverySource represents a recipe source category.
type DiscoverySource string

const (
	// SourceExplicit is a recipe specified by explicit path.
	SourceExplicit DiscoverySource = "explicit"

	// SourceProject is a recipe from the project's .bazelbump/recipes or recipes directory.
	SourceProject DiscoverySource = "project"

	// SourceUser is a recipe from the user's config directory.
	SourceUser DiscoverySource = "user"

	// SourceBuiltin is a recipe embedded in the binary.
	SourceBuiltin DiscoverySource = "builtin"
)

// DiscoveryOptions configures recipe discovery.
type DiscoveryOptions struct {
	// ProjectPath is the project root directory.
	// If empty, uses current working directory.
	ProjectPath string

	// IncludeBuiltin includes built-in recipes in results.
	// Default is true.
	IncludeBuiltin bool

	// UserConfigDir overrides the user config directory.
	// If empty, uses ~/.config/bazelbump.
	UserConfigDir string
}

// DefaultDiscoveryOptions returns options with sensible defaults.
func DefaultDiscoveryOptions() DiscoveryOptions {
	return DiscoveryOptions{
		ProjectPath:    ".",
		IncludeBuiltin: true,
	}
}

// DiscoverRecipes finds all .star recipes from standard paths.
// Recipes are discovered in priority order:
//  1. Project recipes - ./.bazelbump/recipes/*.star
//  2. Project recipes (alt) - ./recipes/*.star
//  3. User recipes - ~/.config/bazelbump/recipes/*.star
//
// Later sources do not override earlier ones with the same name.
func DiscoverRecipes(projectPath string) ([]DiscoveredRecipe, error) {
	return DiscoverRecipesWithOptions(DiscoveryOptions{
		ProjectPath:    projectPath,
		IncludeBuiltin: true,
	})
}

// DiscoverRecipesWithOptions finds recipes with custom options.
func DiscoverRecipesWithOptions(opts DiscoveryOptions) ([]DiscoveredRecipe, error) {
	if opts.ProjectPath == "" {
		opts.ProjectPath = "."
	}

	// Resolve project path to absolute
	absProjectPath, err := filepath.Abs(opts.ProjectPath)
	if err != nil {
		return nil, err
	}

	// Track recipes by name to handle priority (first wins)
	seen := make(map[string]bool)
	var recipes []DiscoveredRecipe

	// Discovery paths in priority order
	discoveryPaths := []struct {
		path   string
		source DiscoverySource
	}{
		// Priority 2: Project recipes (.bazelbump/recipes)
		{filepath.Join(absProjectPath, ".bazelbump", "recipes"), SourceProject},
		// Priority 3: Project recipes (alt - recipes/)
		{filepath.Join(absProjectPath, "recipes"), SourceProject},
		// Priority 4: User recipes
		{getUserRecipesPath(opts.UserConfigDir), SourceUser},
	}

	for _, dp := range discoveryPaths {
		if dp.path == "" {
			continue
		}

		found, err := discoverFromDirectory(dp.path, dp.source)
		if err != nil {
			// Ignore errors for non-existent directories
			continue
		}

		for _, r := range found {
			if !seen[r.Name] {
				seen[r.Name] = true
				recipes = append(recipes, r)
			}
		}
	}

	// Sort by name for consistent ordering
	sort.Slice(recipes, func(i, j int) bool {
		// Sort by source priority first, then by name
		sourcePriority := map[DiscoverySource]int{
			SourceExplicit: 0,
			SourceProject:  1,
			SourceUser:     2,
			SourceBuiltin:  3,
		}
		pi := sourcePriority[DiscoverySource(recipes[i].Source)]
		pj := sourcePriority[DiscoverySource(recipes[j].Source)]
		if pi != pj {
			return pi < pj
		}
		return recipes[i].Name < recipes[j].Name
	})

	return recipes, nil
}

// getUserRecipesPath returns the path to user recipes directory.
func getUserRecipesPath(override string) string {
	if override != "" {
		return filepath.Join(override, "recipes")
	}

	// Use XDG_CONFIG_HOME if set, otherwise ~/.config
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configHome, "bazelbump", "recipes")
}

// GetUserConfigDir returns the user config directory for bazelbump.
func GetUserConfigDir() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configHome, "bazelbump")
}

// discoverFromDirectory finds all .star files in a directory.
func discoverFromDirectory(dir string, source DiscoverySource) ([]DiscoveredRecipe, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var recipes []DiscoveredRecipe

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".star") {
			continue
		}

		// Skip test files
		if strings.HasSuffix(name, "_test.star") {
			continue
		}

		recipeName := strings.TrimSuffix(name, ".star")
		absPath := filepath.Join(dir, name)

		// Try to extract metadata from the file
		desc, version := extractRecipeMetadata(absPath)

		recipes = append(recipes, DiscoveredRecipe{
			Name:        recipeName,
			Path:        absPath,
			Source:      string(source),
			Description: desc,
			Version:     version,
		})
	}

	return recipes, nil
}

// extractRecipeMetadata extracts name, version, and description from a .star file.
// This does a simple regex parse without fully interpreting the Starlark.
func extractRecipeMetadata(path string) (description, version string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	// Read up to 4KB to find the recipe() call
	scanner := bufio.NewScanner(f)
	var content strings.Builder
	lineCount := 0
	for scanner.Scan() && lineCount < 100 {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
		lineCount++
	}

	text := content.String()

	// Extract description
	if match := descriptionPattern.FindStringSubmatch(text); len(match) > 1 {
		description = match[1]
	}

	// Extract version
	if match := versionMetaPattern.FindStringSubmatch(text); len(match) > 1 {
		version = match[1]
	}

	return description, version
}

// ResolveRecipe finds a recipe by name or path.
// It searches in the following order:
//  1. If the name is a path (contains / or ends in .star), use it directly
//  2. Search discovered recipes by name
//  3. Return error if not found
func ResolveRecipe(name string, projectPath string) (*DiscoveredRecipe, error) {
	return ResolveRecipeWithOptions(name, DiscoveryOptions{
		ProjectPath:    projectPath,
		IncludeBuiltin: true,
	})
}

// ResolveRecipeWithOptions resolves a recipe with custom options.
func ResolveRecipeWithOptions(name string, opts DiscoveryOptions) (*DiscoveredRecipe, error) {
	// Check if this is an explicit path
	if isExplicitPath(name) {
		return resolveExplicitPath(name, opts.ProjectPath)
	}

	// Discover recipes and search by name
	recipes, err := DiscoverRecipesWithOptions(opts)
	if err != nil {
		return nil, err
	}

	for _, r := range recipes {
		if r.Name == name {
			return &r, nil
		}
	}

	return nil, &RecipeNotFoundError{Name: name}
}

// isExplicitPath returns true if the name looks like a file path.
func isExplicitPath(name string) bool {
	return strings.HasSuffix(name, ".star") ||
		strings.Contains(name, "/") ||
		strings.Contains(name, string(os.PathSeparator))
}

// resolveExplicitPath resolves an explicit .star file path.
func resolveExplicitPath(path string, projectPath string) (*DiscoveredRecipe, error) {
	// Make path absolute
	var absPath string
	if filepath.IsAbs(path) {
		absPath = path
	} else {
		base := projectPath
		if base == "" {
			base = "."
		}
		absBase, err := filepath.Abs(base)
		if err != nil {
			return nil, err
		}
		absPath = filepath.Join(absBase, path)
	}

	// Check file exists
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return nil, &RecipeNotFoundError{Name: path, Path: absPath}
		}
		return nil, err
	}

	// Extract recipe name from filename
	name := strings.TrimSuffix(filepath.Base(absPath), ".star")

	// Extract metadata
	desc, version := extractRecipeMetadata(absPath)

	return &DiscoveredRecipe{
		Name:        name,
		Path:        absPath,
		Source:      string(SourceExplicit),
		Description: desc,
		Version:     version,
	}, nil
}

// RecipeNotFoundError is returned when a recipe cannot be found.
type RecipeNotFoundError struct {
	Name string
	Path string
}

func (e *RecipeNotFoundError) Error() string {
	if e.Path != "" {
		return "recipe file not found: " + e.Path
	}
	return "unknown recipe: " + e.Name
}

// GetDiscoveryPaths returns the paths that will be searched for recipes.
// Useful for documentation and debugging.
func GetDiscoveryPaths(projectPath string) []string {
	if projectPath == "" {
		projectPath = "."
	}

	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		absProjectPath = projectPath
	}

	paths := []string{
		filepath.Join(absProjectPath, ".bazelbump", "recipes"),
		filepath.Join(absProjectPath, "recipes"),
		getUserRecipesPath(""),
	}

	// Filter out empty paths
	var result []string
	for _, p := range paths {
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}

// DiscoveredTest represents a test file found during discovery.
type DiscoveredTest struct {
	// Name is the test name (filename without .star extension)
	Name string

	// Path is the absolute path to the test file
	Path string

	// RecipeName is the name of the recipe this test is for (if determinable)
	// Empty if the test is standalone or the recipe cannot be determined.
	RecipeName string

	// Source indicates where the test was found (same as DiscoverySource)
	Source string
}

// TestDiscoveryOptions configures test file discovery.
type TestDiscoveryOptions struct {
	// ProjectPath is the project root directory.
	// If empty, uses current working directory.
	ProjectPath string

	// UserConfigDir overrides the user config directory.
	// If empty, uses ~/.config/bazelbump.
	UserConfigDir string

	// RecursiveSearch enables searching subdirectories.
	// Default is false (only searches standard recipe directories).
	RecursiveSearch bool
}

// DefaultTestDiscoveryOptions returns options with sensible defaults.
func DefaultTestDiscoveryOptions() TestDiscoveryOptions {
	return TestDiscoveryOptions{
		ProjectPath:     ".",
		RecursiveSearch: false,
	}
}

// DiscoverTestFiles finds all test files (*_test.star, test_*.star) from standard paths.
// Test files are discovered from the same locations as recipes.
func DiscoverTestFiles(opts TestDiscoveryOptions) ([]DiscoveredTest, error) {
	if opts.ProjectPath == "" {
		opts.ProjectPath = "."
	}

	// Resolve project path to absolute
	absProjectPath, err := filepath.Abs(opts.ProjectPath)
	if err != nil {
		return nil, err
	}

	var tests []DiscoveredTest

	// Discovery paths (same as recipes)
	discoveryPaths := []struct {
		path   string
		source DiscoverySource
	}{
		{filepath.Join(absProjectPath, ".bazelbump", "recipes"), SourceProject},
		{filepath.Join(absProjectPath, "recipes"), SourceProject},
		{getUserRecipesPath(opts.UserConfigDir), SourceUser},
	}

	for _, dp := range discoveryPaths {
		if dp.path == "" {
			continue
		}

		found, err := discoverTestsFromDirectory(dp.path, dp.source)
		if err != nil {
			// Ignore errors for non-existent directories
			continue
		}

		tests = append(tests, found...)
	}

	// Sort by name for consistent ordering
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Name < tests[j].Name
	})

	return tests, nil
}

// DiscoverTestsForRecipe finds test files associated with a specific recipe.
// It looks for files named <recipe>_test.star or test_<recipe>.star in the same
// directory as the recipe.
func DiscoverTestsForRecipe(recipePath string) ([]DiscoveredTest, error) {
	// Get the directory containing the recipe
	dir := filepath.Dir(recipePath)
	baseName := filepath.Base(recipePath)

	// Remove .star extension to get recipe name
	recipeName := strings.TrimSuffix(baseName, ".star")

	var tests []DiscoveredTest

	// Look for <recipe>_test.star
	testPath := filepath.Join(dir, recipeName+"_test.star")
	if _, err := os.Stat(testPath); err == nil {
		tests = append(tests, DiscoveredTest{
			Name:       recipeName + "_test",
			Path:       testPath,
			RecipeName: recipeName,
			Source:     string(SourceExplicit),
		})
	}

	// Look for test_<recipe>.star
	testPath = filepath.Join(dir, "test_"+recipeName+".star")
	if _, err := os.Stat(testPath); err == nil {
		tests = append(tests, DiscoveredTest{
			Name:       "test_" + recipeName,
			Path:       testPath,
			RecipeName: recipeName,
			Source:     string(SourceExplicit),
		})
	}

	return tests, nil
}

// discoverTestsFromDirectory finds all test files in a directory.
func discoverTestsFromDirectory(dir string, source DiscoverySource) ([]DiscoveredTest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var tests []DiscoveredTest

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".star") {
			continue
		}

		// Check if this is a test file
		if !isTestFile(name) {
			continue
		}

		testName := strings.TrimSuffix(name, ".star")
		absPath := filepath.Join(dir, name)

		// Try to determine the associated recipe name
		recipeName := ""
		if strings.HasSuffix(testName, "_test") {
			recipeName = strings.TrimSuffix(testName, "_test")
		} else if strings.HasPrefix(testName, "test_") {
			recipeName = strings.TrimPrefix(testName, "test_")
		}

		tests = append(tests, DiscoveredTest{
			Name:       testName,
			Path:       absPath,
			RecipeName: recipeName,
			Source:     string(source),
		})
	}

	return tests, nil
}

// isTestFile returns true if the filename indicates a test file.
func isTestFile(name string) bool {
	// Must end with .star
	if !strings.HasSuffix(name, ".star") {
		return false
	}

	baseName := strings.TrimSuffix(name, ".star")

	// Check for *_test.star pattern
	if strings.HasSuffix(baseName, "_test") {
		return true
	}

	// Check for test_*.star pattern
	if strings.HasPrefix(baseName, "test_") {
		return true
	}

	return false
}
