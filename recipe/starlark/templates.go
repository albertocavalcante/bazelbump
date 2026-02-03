// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// Embed the recipe templates.
// These templates can be used to scaffold new recipes.
//
//go:embed templates/*.star.tmpl
var templatesFS embed.FS

// TemplateInfo describes an available recipe template.
type TemplateInfo struct {
	// Name is the template identifier (e.g., "flag-cleanup").
	Name string

	// Description provides a short description of the template.
	Description string

	// FileName is the template filename (e.g., "flag-cleanup.star.tmpl").
	FileName string
}

// TemplateData contains the data passed to templates during rendering.
type TemplateData struct {
	// Name is the recipe name provided by the user.
	Name string

	// Description is the recipe description provided by the user.
	Description string

	// FileName is the output filename (e.g., "my-recipe.star").
	FileName string
}

// templateDescriptions maps template names to their descriptions.
var templateDescriptions = map[string]string{
	"flag-cleanup":   "Remove deprecated/removed flags from .bazelrc",
	"load-migration": "Add load() statements for native rules",
	"custom-scanner": "Custom scan + transform logic template",
	"blank":          "Empty recipe template",
}

// ListTemplates returns all available recipe templates.
func ListTemplates() ([]TemplateInfo, error) {
	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates: %w", err)
	}

	var templates []TemplateInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".star.tmpl") {
			continue
		}

		templateName := strings.TrimSuffix(name, ".star.tmpl")
		desc := templateDescriptions[templateName]
		if desc == "" {
			desc = "Recipe template"
		}

		templates = append(templates, TemplateInfo{
			Name:        templateName,
			Description: desc,
			FileName:    name,
		})
	}

	// Sort by name for consistent ordering
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})

	return templates, nil
}

// GetTemplate returns information about a specific template.
func GetTemplate(name string) (*TemplateInfo, error) {
	templates, err := ListTemplates()
	if err != nil {
		return nil, err
	}

	for _, t := range templates {
		if t.Name == name {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("unknown template: %s", name)
}

// RenderTemplate renders a template with the given data.
func RenderTemplate(templateName string, data TemplateData) (string, error) {
	// Read the template content
	fileName := templateName + ".star.tmpl"
	content, err := templatesFS.ReadFile("templates/" + fileName)
	if err != nil {
		return "", fmt.Errorf("template %q not found: %w", templateName, err)
	}

	// Parse and execute the template
	tmpl, err := template.New(templateName).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return buf.String(), nil
}

// InitRecipeOptions configures recipe initialization.
type InitRecipeOptions struct {
	// Name is the recipe name (required).
	Name string

	// Description is the recipe description (optional).
	Description string

	// Template is the template to use (optional, defaults to "blank").
	Template string

	// OutputDir is the directory to write the recipe file to (optional).
	// Defaults to current directory.
	OutputDir string

	// Force overwrites existing files without prompting.
	Force bool
}

// InitRecipeResult contains the result of recipe initialization.
type InitRecipeResult struct {
	// Path is the absolute path to the created file.
	Path string

	// FileName is the base filename.
	FileName string

	// Template is the template that was used.
	Template string
}

// InitRecipe creates a new recipe file from a template.
func InitRecipe(opts InitRecipeOptions) (*InitRecipeResult, error) {
	// Validate name
	if opts.Name == "" {
		return nil, fmt.Errorf("recipe name is required")
	}

	// Sanitize name for use as filename
	fileName := sanitizeRecipeName(opts.Name) + ".star"

	// Default template
	if opts.Template == "" {
		opts.Template = "blank"
	}

	// Verify template exists
	if _, err := GetTemplate(opts.Template); err != nil {
		return nil, err
	}

	// Default description
	if opts.Description == "" {
		opts.Description = "Custom migration recipe"
	}

	// Default output directory
	outputDir := opts.OutputDir
	if outputDir == "" {
		var err error
		outputDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Build output path
	outputPath := filepath.Join(outputDir, fileName)

	// Check if file exists
	if _, err := os.Stat(outputPath); err == nil && !opts.Force {
		return nil, fmt.Errorf("file already exists: %s (use --force to overwrite)", outputPath)
	}

	// Render the template
	data := TemplateData{
		Name:        opts.Name,
		Description: opts.Description,
		FileName:    fileName,
	}

	content, err := RenderTemplate(opts.Template, data)
	if err != nil {
		return nil, err
	}

	// Write the file
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return &InitRecipeResult{
		Path:     outputPath,
		FileName: fileName,
		Template: opts.Template,
	}, nil
}

// sanitizeRecipeName converts a recipe name to a valid filename.
func sanitizeRecipeName(name string) string {
	// Trim leading/trailing whitespace first
	name = strings.TrimSpace(name)

	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Remove any character that isn't alphanumeric, hyphen, or underscore
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}

	// Convert to lowercase and trim any leading/trailing hyphens
	sanitized := strings.ToLower(result.String())
	sanitized = strings.Trim(sanitized, "-")
	return sanitized
}

// TemplateNames returns the names of all available templates.
func TemplateNames() []string {
	templates, err := ListTemplates()
	if err != nil {
		return nil
	}

	names := make([]string, len(templates))
	for i, t := range templates {
		names[i] = t.Name
	}
	return names
}
