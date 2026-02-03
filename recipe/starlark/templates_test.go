// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListTemplates(t *testing.T) {
	templates, err := ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates() error = %v", err)
	}

	if len(templates) == 0 {
		t.Fatal("ListTemplates() returned no templates")
	}

	// Check for expected templates
	expectedTemplates := map[string]bool{
		"flag-cleanup":   false,
		"load-migration": false,
		"custom-scanner": false,
		"blank":          false,
	}

	for _, tmpl := range templates {
		if _, ok := expectedTemplates[tmpl.Name]; ok {
			expectedTemplates[tmpl.Name] = true
		}
	}

	for name, found := range expectedTemplates {
		if !found {
			t.Errorf("Expected template %q not found", name)
		}
	}
}

func TestGetTemplate(t *testing.T) {
	tests := []struct {
		name        string
		templateName string
		wantErr     bool
	}{
		{"valid flag-cleanup", "flag-cleanup", false},
		{"valid load-migration", "load-migration", false},
		{"valid custom-scanner", "custom-scanner", false},
		{"valid blank", "blank", false},
		{"invalid template", "nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := GetTemplate(tt.templateName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tmpl.Name != tt.templateName {
				t.Errorf("GetTemplate() name = %q, want %q", tmpl.Name, tt.templateName)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
		data         TemplateData
		wantContains []string
	}{
		{
			name:         "flag-cleanup template",
			templateName: "flag-cleanup",
			data: TemplateData{
				Name:        "my-cleanup",
				Description: "Test description",
				FileName:    "my-cleanup.star",
			},
			wantContains: []string{
				"my-cleanup",
				"Test description",
				"remove_flag",
				"recipe(",
			},
		},
		{
			name:         "load-migration template",
			templateName: "load-migration",
			data: TemplateData{
				Name:        "test-migration",
				Description: "Test load migration",
				FileName:    "test-migration.star",
			},
			wantContains: []string{
				"test-migration",
				"ensure_load",
				"ensure_bazel_dep",
				"scan(",
			},
		},
		{
			name:         "custom-scanner template",
			templateName: "custom-scanner",
			data: TemplateData{
				Name:        "custom-test",
				Description: "Custom test recipe",
				FileName:    "custom-test.star",
			},
			wantContains: []string{
				"custom-test",
				"def find_patterns",
				"def apply_transforms",
				"impl = find_patterns",
			},
		},
		{
			name:         "blank template",
			templateName: "blank",
			data: TemplateData{
				Name:        "empty-recipe",
				Description: "Empty recipe",
				FileName:    "empty-recipe.star",
			},
			wantContains: []string{
				"empty-recipe",
				"Empty recipe",
				"recipe(",
				"steps = [",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := RenderTemplate(tt.templateName, tt.data)
			if err != nil {
				t.Fatalf("RenderTemplate() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(content, want) {
					t.Errorf("RenderTemplate() content does not contain %q\nContent:\n%s", want, content)
				}
			}
		})
	}
}

func TestRenderTemplateInvalid(t *testing.T) {
	_, err := RenderTemplate("nonexistent", TemplateData{})
	if err == nil {
		t.Error("RenderTemplate() with invalid template should return error")
	}
}

func TestInitRecipe(t *testing.T) {
	// Create a temporary directory for test output
	tmpDir, err := os.MkdirTemp("", "bazelbump-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		opts    InitRecipeOptions
		wantErr bool
	}{
		{
			name: "create with flag-cleanup template",
			opts: InitRecipeOptions{
				Name:        "test-cleanup",
				Description: "Test cleanup recipe",
				Template:    "flag-cleanup",
				OutputDir:   tmpDir,
			},
			wantErr: false,
		},
		{
			name: "create with default template",
			opts: InitRecipeOptions{
				Name:      "test-default",
				OutputDir: tmpDir,
			},
			wantErr: false,
		},
		{
			name: "invalid template",
			opts: InitRecipeOptions{
				Name:      "test-invalid",
				Template:  "nonexistent",
				OutputDir: tmpDir,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			opts: InitRecipeOptions{
				Template:  "blank",
				OutputDir: tmpDir,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := InitRecipe(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitRecipe() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify file was created
			if _, err := os.Stat(result.Path); err != nil {
				t.Errorf("InitRecipe() created file not found at %s", result.Path)
			}

			// Verify file content
			content, err := os.ReadFile(result.Path)
			if err != nil {
				t.Errorf("Failed to read created file: %v", err)
			}

			if !strings.Contains(string(content), tt.opts.Name) {
				t.Errorf("Created file does not contain recipe name %q", tt.opts.Name)
			}
		})
	}
}

func TestInitRecipeOverwrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bazelbump-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create initial file
	opts := InitRecipeOptions{
		Name:      "test-overwrite",
		Template:  "blank",
		OutputDir: tmpDir,
	}

	_, err = InitRecipe(opts)
	if err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	// Try to create again without force - should fail
	_, err = InitRecipe(opts)
	if err == nil {
		t.Error("InitRecipe() should fail when file exists and force=false")
	}

	// Try with force - should succeed
	opts.Force = true
	result, err := InitRecipe(opts)
	if err != nil {
		t.Errorf("InitRecipe() with force=true failed: %v", err)
	}

	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("File not found after overwrite: %v", err)
	}
}

func TestSanitizeRecipeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-recipe", "my-recipe"},
		{"my recipe", "my-recipe"},
		{"My Recipe", "my-recipe"},
		{"my_recipe", "my_recipe"},
		{"recipe123", "recipe123"},
		{"recipe!@#$%", "recipe"},
		{"  spaces  ", "spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeRecipeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeRecipeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTemplateNames(t *testing.T) {
	names := TemplateNames()
	if len(names) == 0 {
		t.Error("TemplateNames() returned empty list")
	}

	// Should contain expected templates
	expected := []string{"flag-cleanup", "load-migration", "custom-scanner", "blank"}
	for _, e := range expected {
		found := false
		for _, n := range names {
			if n == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("TemplateNames() does not contain expected template %q", e)
		}
	}
}

func TestInitRecipeFilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bazelbump-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := InitRecipeOptions{
		Name:        "test-path",
		Template:    "blank",
		OutputDir:   tmpDir,
	}

	result, err := InitRecipe(opts)
	if err != nil {
		t.Fatalf("InitRecipe() error = %v", err)
	}

	// Verify path components
	expectedFileName := "test-path.star"
	if result.FileName != expectedFileName {
		t.Errorf("FileName = %q, want %q", result.FileName, expectedFileName)
	}

	expectedPath := filepath.Join(tmpDir, expectedFileName)
	if result.Path != expectedPath {
		t.Errorf("Path = %q, want %q", result.Path, expectedPath)
	}
}
