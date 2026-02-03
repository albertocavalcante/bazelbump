// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRecipes(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create project recipes directory
	projectRecipesDir := filepath.Join(tmpDir, ".bazelbump", "recipes")
	if err := os.MkdirAll(projectRecipesDir, 0755); err != nil {
		t.Fatalf("failed to create project recipes dir: %v", err)
	}

	// Create a project recipe
	projectRecipe := filepath.Join(projectRecipesDir, "project-recipe.star")
	if err := os.WriteFile(projectRecipe, []byte(`
recipe(
    name = "project-recipe",
    version = "1.0.0",
    description = "A project-level recipe",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create project recipe: %v", err)
	}

	// Create alt recipes directory
	altRecipesDir := filepath.Join(tmpDir, "recipes")
	if err := os.MkdirAll(altRecipesDir, 0755); err != nil {
		t.Fatalf("failed to create alt recipes dir: %v", err)
	}

	// Create an alt recipe
	altRecipe := filepath.Join(altRecipesDir, "alt-recipe.star")
	if err := os.WriteFile(altRecipe, []byte(`
recipe(
    name = "alt-recipe",
    version = "2.0.0",
    description = "An alternate location recipe",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create alt recipe: %v", err)
	}

	// Discover recipes
	recipes, err := DiscoverRecipes(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverRecipes failed: %v", err)
	}

	// Verify we found both recipes
	if len(recipes) < 2 {
		t.Errorf("expected at least 2 recipes, got %d", len(recipes))
	}

	// Check for project recipe
	var foundProject, foundAlt bool
	for _, r := range recipes {
		switch r.Name {
		case "project-recipe":
			foundProject = true
			if r.Source != "project" {
				t.Errorf("project-recipe source = %q, want %q", r.Source, "project")
			}
			if r.Description != "A project-level recipe" {
				t.Errorf("project-recipe description = %q, want %q", r.Description, "A project-level recipe")
			}
			if r.Version != "1.0.0" {
				t.Errorf("project-recipe version = %q, want %q", r.Version, "1.0.0")
			}
		case "alt-recipe":
			foundAlt = true
			if r.Source != "project" {
				t.Errorf("alt-recipe source = %q, want %q", r.Source, "project")
			}
		}
	}

	if !foundProject {
		t.Error("project-recipe not found")
	}
	if !foundAlt {
		t.Error("alt-recipe not found")
	}
}

func TestDiscoverRecipes_Priority(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create the same recipe in both locations
	projectRecipesDir := filepath.Join(tmpDir, ".bazelbump", "recipes")
	if err := os.MkdirAll(projectRecipesDir, 0755); err != nil {
		t.Fatalf("failed to create project recipes dir: %v", err)
	}

	altRecipesDir := filepath.Join(tmpDir, "recipes")
	if err := os.MkdirAll(altRecipesDir, 0755); err != nil {
		t.Fatalf("failed to create alt recipes dir: %v", err)
	}

	// Create same-named recipe in project dir (higher priority)
	projectRecipe := filepath.Join(projectRecipesDir, "my-recipe.star")
	if err := os.WriteFile(projectRecipe, []byte(`
recipe(
    name = "my-recipe",
    version = "1.0.0",
    description = "From .bazelbump/recipes",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create project recipe: %v", err)
	}

	// Create same-named recipe in alt dir (lower priority)
	altRecipe := filepath.Join(altRecipesDir, "my-recipe.star")
	if err := os.WriteFile(altRecipe, []byte(`
recipe(
    name = "my-recipe",
    version = "2.0.0",
    description = "From recipes/",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create alt recipe: %v", err)
	}

	// Discover recipes
	recipes, err := DiscoverRecipes(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverRecipes failed: %v", err)
	}

	// Should only have one recipe with the higher priority version
	var found *DiscoveredRecipe
	for _, r := range recipes {
		if r.Name == "my-recipe" {
			found = &r
			break
		}
	}

	if found == nil {
		t.Fatal("my-recipe not found")
	}

	// Should be the one from .bazelbump/recipes (higher priority)
	if found.Description != "From .bazelbump/recipes" {
		t.Errorf("expected recipe from .bazelbump/recipes, got description = %q", found.Description)
	}
	if found.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", found.Version)
	}
}

func TestResolveRecipe_ExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a recipe file
	recipePath := filepath.Join(tmpDir, "my-recipe.star")
	if err := os.WriteFile(recipePath, []byte(`
recipe(
    name = "my-recipe",
    version = "1.0.0",
    description = "Test recipe",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create recipe: %v", err)
	}

	// Resolve by absolute path
	r, err := ResolveRecipe(recipePath, tmpDir)
	if err != nil {
		t.Fatalf("ResolveRecipe failed: %v", err)
	}

	if r.Name != "my-recipe" {
		t.Errorf("Name = %q, want %q", r.Name, "my-recipe")
	}
	if r.Source != "explicit" {
		t.Errorf("Source = %q, want %q", r.Source, "explicit")
	}
	if r.Path != recipePath {
		t.Errorf("Path = %q, want %q", r.Path, recipePath)
	}
}

func TestResolveRecipe_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a recipe file
	recipePath := filepath.Join(tmpDir, "my-recipe.star")
	if err := os.WriteFile(recipePath, []byte(`
recipe(
    name = "my-recipe",
    version = "1.0.0",
    description = "Test recipe",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create recipe: %v", err)
	}

	// Resolve by relative path
	r, err := ResolveRecipe("my-recipe.star", tmpDir)
	if err != nil {
		t.Fatalf("ResolveRecipe failed: %v", err)
	}

	if r.Name != "my-recipe" {
		t.Errorf("Name = %q, want %q", r.Name, "my-recipe")
	}
	if r.Source != "explicit" {
		t.Errorf("Source = %q, want %q", r.Source, "explicit")
	}
}

func TestResolveRecipe_ByName(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project recipes directory
	projectRecipesDir := filepath.Join(tmpDir, ".bazelbump", "recipes")
	if err := os.MkdirAll(projectRecipesDir, 0755); err != nil {
		t.Fatalf("failed to create project recipes dir: %v", err)
	}

	// Create a recipe
	recipePath := filepath.Join(projectRecipesDir, "test-recipe.star")
	if err := os.WriteFile(recipePath, []byte(`
recipe(
    name = "test-recipe",
    version = "1.0.0",
    description = "Test recipe",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create recipe: %v", err)
	}

	// Resolve by name
	r, err := ResolveRecipe("test-recipe", tmpDir)
	if err != nil {
		t.Fatalf("ResolveRecipe failed: %v", err)
	}

	if r.Name != "test-recipe" {
		t.Errorf("Name = %q, want %q", r.Name, "test-recipe")
	}
	if r.Source != "project" {
		t.Errorf("Source = %q, want %q", r.Source, "project")
	}
}

func TestResolveRecipe_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := ResolveRecipe("nonexistent-recipe", tmpDir)
	if err == nil {
		t.Fatal("expected error for nonexistent recipe")
	}

	var notFound *RecipeNotFoundError
	if _, ok := err.(*RecipeNotFoundError); !ok {
		t.Errorf("error type = %T, want *RecipeNotFoundError", err)
	} else {
		notFound = err.(*RecipeNotFoundError)
		if notFound.Name != "nonexistent-recipe" {
			t.Errorf("error name = %q, want %q", notFound.Name, "nonexistent-recipe")
		}
	}
}

func TestGetDiscoveryPaths(t *testing.T) {
	paths := GetDiscoveryPaths("/test/project")

	// Should have 3 paths
	if len(paths) < 3 {
		t.Errorf("expected at least 3 paths, got %d", len(paths))
	}

	// First should be .bazelbump/recipes
	if !filepath.IsAbs(paths[0]) || !containsPath(paths[0], ".bazelbump", "recipes") {
		t.Errorf("first path should be .bazelbump/recipes, got %q", paths[0])
	}

	// Second should be recipes/
	if !filepath.IsAbs(paths[1]) || filepath.Base(paths[1]) != "recipes" {
		t.Errorf("second path should be recipes/, got %q", paths[1])
	}
}

func TestExtractRecipeMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a recipe with metadata
	recipePath := filepath.Join(tmpDir, "test.star")
	if err := os.WriteFile(recipePath, []byte(`
# Test recipe file
recipe(
    name = "test-recipe",
    version = "1.2.3",
    description = "This is a test recipe for validation",
    steps = [],
)
`), 0644); err != nil {
		t.Fatalf("failed to create recipe: %v", err)
	}

	desc, version := extractRecipeMetadata(recipePath)

	if desc != "This is a test recipe for validation" {
		t.Errorf("description = %q, want %q", desc, "This is a test recipe for validation")
	}
	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
}

func TestDiscoverRecipes_SkipsTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project recipes directory
	recipesDir := filepath.Join(tmpDir, ".bazelbump", "recipes")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatalf("failed to create recipes dir: %v", err)
	}

	// Create a regular recipe
	if err := os.WriteFile(filepath.Join(recipesDir, "my-recipe.star"), []byte(`
recipe(name = "my-recipe", version = "1.0.0", steps = [])
`), 0644); err != nil {
		t.Fatalf("failed to create recipe: %v", err)
	}

	// Create a test recipe (should be skipped)
	if err := os.WriteFile(filepath.Join(recipesDir, "my-recipe_test.star"), []byte(`
# Test file for my-recipe
`), 0644); err != nil {
		t.Fatalf("failed to create test recipe: %v", err)
	}

	recipes, err := DiscoverRecipes(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverRecipes failed: %v", err)
	}

	// Should only find the non-test recipe
	for _, r := range recipes {
		if r.Name == "my-recipe_test" {
			t.Error("test file should not be discovered")
		}
	}
}

func containsPath(full string, parts ...string) bool {
	for _, part := range parts {
		if !contains(full, part) {
			return false
		}
	}
	return true
}

// TestDiscoverRecipes_RealExamples tests discovery using the actual examples directory.
// This test is skipped if the examples directory doesn't exist.
func TestDiscoverRecipes_RealExamples(t *testing.T) {
	// This test verifies that discovery works with the real examples/project-recipes directory.
	// It's useful for catching issues with relative path handling.

	// Get the path relative to the test file location
	// Tests run from the package directory, so we need to go up two levels
	projectPath := "../../examples/project-recipes"

	// Skip if the directory doesn't exist (e.g., running tests from a different location)
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		t.Skipf("examples directory not found at %s", projectPath)
	}

	// Test discovery
	recipes, err := DiscoverRecipes(projectPath)
	if err != nil {
		t.Fatalf("DiscoverRecipes failed: %v", err)
	}

	t.Logf("Discovered %d recipes from %s", len(recipes), projectPath)
	for _, r := range recipes {
		t.Logf("  - %s (source=%s, path=%s)", r.Name, r.Source, r.Path)
	}

	// Should find my-custom-migration
	var found bool
	for _, r := range recipes {
		if r.Name == "my-custom-migration" {
			found = true
			if r.Source != "project" {
				t.Errorf("source = %q, want %q", r.Source, "project")
			}
		}
	}

	if !found {
		t.Error("my-custom-migration recipe not found")
	}

	// Test resolution
	resolved, err := ResolveRecipe("my-custom-migration", projectPath)
	if err != nil {
		t.Fatalf("ResolveRecipe failed: %v", err)
	}

	t.Logf("Resolved: name=%s, source=%s, path=%s", resolved.Name, resolved.Source, resolved.Path)

	if resolved.Name != "my-custom-migration" {
		t.Errorf("resolved.Name = %q, want %q", resolved.Name, "my-custom-migration")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
