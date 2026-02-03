// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	recipestar "github.com/albertocavalcante/bazelbump/recipe/starlark"
)

func runRecipeInit(args []string) error {
	fs := flag.NewFlagSet("recipe init", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create a new recipe from a template.

Scaffolds a new recipe file using one of the built-in templates.
If no --template is specified, prompts interactively for template selection.

%s
  bazelbump recipe init [flags] <name>

%s
  --template   Template to use (flag-cleanup, load-migration, custom-scanner, blank)
  --desc       Recipe description
  --output     Output directory (default: current directory)
  --force      Overwrite existing file
  --list       List available templates
  --help       Show this help

%s
  # Interactive mode
  bazelbump recipe init my-recipe

  # With specific template
  bazelbump recipe init my-recipe --template flag-cleanup
  bazelbump recipe init my-recipe --template custom-scanner

  # With description
  bazelbump recipe init my-recipe --template load-migration --desc "Migrate to rules_cc"

%s
  flag-cleanup    - Remove deprecated/removed flags from .bazelrc
  load-migration  - Add load() statements for native rules
  custom-scanner  - Custom scan + transform logic template
  blank           - Empty recipe template
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"),
			color(colorBold, "Templates:"))
	}

	templateName := fs.String("template", "", "Template to use")
	description := fs.String("desc", "", "Recipe description")
	outputDir := fs.String("output", "", "Output directory")
	force := fs.Bool("force", false, "Overwrite existing file")
	listTemplates := fs.Bool("list", false, "List available templates")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Handle --list flag
	if *listTemplates {
		return listRecipeTemplates()
	}

	// Require recipe name
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("recipe name required")
	}

	recipeName := fs.Arg(0)

	// If no template specified, run in interactive mode
	if *templateName == "" {
		return runRecipeInitInteractive(recipeName, *description, *outputDir, *force)
	}

	// Non-interactive mode with specified template
	opts := recipestar.InitRecipeOptions{
		Name:        recipeName,
		Description: *description,
		Template:    *templateName,
		OutputDir:   *outputDir,
		Force:       *force,
	}

	result, err := recipestar.InitRecipe(opts)
	if err != nil {
		return err
	}

	outputRecipeInitSuccess(result)
	return nil
}

// listRecipeTemplates lists all available recipe templates.
func listRecipeTemplates() error {
	templates, err := recipestar.ListTemplates()
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	fmt.Printf("%s Available templates:\n\n", color(colorBlue, "==>"))

	for _, t := range templates {
		fmt.Printf("  %s\n", color(colorCyan, t.Name))
		fmt.Printf("      %s\n\n", t.Description)
	}

	fmt.Printf("Use: bazelbump recipe init <name> --template <template-name>\n")
	return nil
}

// runRecipeInitInteractive prompts the user for template selection.
func runRecipeInitInteractive(name, description, outputDir string, force bool) error {
	templates, err := recipestar.ListTemplates()
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	// Check if stdin is a terminal for interactive mode
	stdinInfo, err := os.Stdin.Stat()
	if err != nil || (stdinInfo.Mode()&os.ModeCharDevice) == 0 {
		// Not a terminal, use default template
		fmt.Println("Non-interactive mode detected. Using 'blank' template.")
		opts := recipestar.InitRecipeOptions{
			Name:        name,
			Description: description,
			Template:    "blank",
			OutputDir:   outputDir,
			Force:       force,
		}

		result, err := recipestar.InitRecipe(opts)
		if err != nil {
			return err
		}

		outputRecipeInitSuccess(result)
		return nil
	}

	// Prompt for description if not provided
	if description == "" {
		fmt.Print("? Recipe description: ")
		var desc string
		_, err := fmt.Scanln(&desc)
		if err == nil && desc != "" {
			description = desc
		} else {
			description = "Custom migration recipe"
		}
	}

	// Prompt for template selection
	fmt.Println("? Select a template:")
	for i, t := range templates {
		marker := "  "
		if i == 0 {
			marker = "> "
		}
		fmt.Printf("  %s%s - %s\n", marker, color(colorCyan, t.Name), t.Description)
	}

	fmt.Print("\nEnter template name (or press Enter for 'blank'): ")
	var selectedTemplate string
	_, err = fmt.Scanln(&selectedTemplate)
	if err != nil || selectedTemplate == "" {
		selectedTemplate = "blank"
	}

	// Validate template
	validTemplate := false
	for _, t := range templates {
		if t.Name == selectedTemplate {
			validTemplate = true
			break
		}
	}
	if !validTemplate {
		return fmt.Errorf("unknown template: %s\n\nAvailable templates: %s",
			selectedTemplate, strings.Join(recipestar.TemplateNames(), ", "))
	}

	opts := recipestar.InitRecipeOptions{
		Name:        name,
		Description: description,
		Template:    selectedTemplate,
		OutputDir:   outputDir,
		Force:       force,
	}

	result, err := recipestar.InitRecipe(opts)
	if err != nil {
		return err
	}

	outputRecipeInitSuccess(result)
	return nil
}

// outputRecipeInitSuccess prints the success message after creating a recipe.
func outputRecipeInitSuccess(result *recipestar.InitRecipeResult) {
	fmt.Printf("\n%s Created: %s\n", color(colorGreen, "[OK]"), color(colorBold, result.FileName))
	fmt.Printf("    Template: %s\n", result.Template)
	fmt.Printf("    Path: %s\n", result.Path)

	fmt.Printf("\n%s\n", color(colorBold, "Next steps:"))
	fmt.Printf("  1. Edit %s to customize\n", result.FileName)
	fmt.Printf("  2. Test: bazelbump recipe run %s --dry-run\n", result.FileName)
	fmt.Printf("  3. Apply: bazelbump recipe run %s\n", result.FileName)
}
