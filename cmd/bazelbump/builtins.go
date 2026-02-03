// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	recipestar "github.com/albertocavalcante/bazelbump/recipe/starlark"
)

func runRecipeBuiltins(args []string) error {
	fs := flag.NewFlagSet("recipe builtins", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `List and document available Starlark builtins.

Shows all built-in functions available in Starlark recipes with their
signatures and descriptions. Use with a builtin name for detailed help.

%s
  bazelbump recipe builtins [name]

%s
  --json   Output results as JSON
  --help   Show this help

%s
  bazelbump recipe builtins                 # List all builtins
  bazelbump recipe builtins ensure_load     # Show help for specific builtin
  bazelbump recipe builtins --json          # Output as JSON
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// If a specific builtin name is provided, show detailed help
	if fs.NArg() > 0 {
		builtinName := fs.Arg(0)
		return showBuiltinHelp(builtinName, *jsonOutput)
	}

	// Otherwise, list all builtins
	return listAllBuiltins(*jsonOutput)
}

func listAllBuiltins(jsonOutput bool) error {
	allDocs := recipestar.AllBuiltinDocs()

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(allDocs)
	}

	// Text output - organized by category
	fmt.Println("Recipe Builtins")
	fmt.Println("===============")
	fmt.Println()

	for _, category := range recipestar.BuiltinCategories() {
		docs := recipestar.GetBuiltinsByCategory(category)
		if len(docs) == 0 {
			continue
		}

		fmt.Printf("%s\n", color(colorBold, recipestar.CategoryDisplayName(category)))
		for _, doc := range docs {
			// Format: name(sig...)  description
			// Truncate signature to fit nicely
			sig := doc.Signature
			if len(sig) > 35 {
				// Find closing paren and truncate
				if idx := strings.Index(sig, "("); idx != -1 {
					builtinName := sig[:idx]
					// Abbreviate args
					sig = builtinName + "(...)"
				}
			}
			fmt.Printf("  %-35s %s\n", sig, doc.Description)
		}
		fmt.Println()
	}

	fmt.Println("Use 'bazelbump recipe builtins <name>' for detailed help.")
	return nil
}

func showBuiltinHelp(builtinName string, jsonOutput bool) error {
	doc := recipestar.GetBuiltinDoc(builtinName)
	if doc == nil {
		// Try with re. prefix for regex functions
		if !strings.HasPrefix(builtinName, "re.") {
			doc = recipestar.GetBuiltinDoc("re." + builtinName)
		}
	}

	if doc == nil {
		return fmt.Errorf("unknown builtin: %s\n\nUse 'bazelbump recipe builtins' to see available builtins", builtinName)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}

	// Text output - detailed help
	fmt.Printf("%s\n", doc.Signature)
	fmt.Println(strings.Repeat("=", len(doc.Signature)))
	fmt.Println()
	fmt.Println(doc.Description)
	fmt.Println()

	// Arguments
	if len(doc.Args) > 0 {
		fmt.Printf("%s\n", color(colorBold, "ARGUMENTS"))
		for _, arg := range doc.Args {
			reqStr := "optional"
			if arg.Required {
				reqStr = "required"
			}
			fmt.Printf("  %-12s (%s, %s)  %s\n", arg.Name, arg.Type, reqStr, arg.Description)
		}
		fmt.Println()
	}

	// Behavior
	if len(doc.Behavior) > 0 {
		fmt.Printf("%s\n", color(colorBold, "BEHAVIOR"))
		for _, b := range doc.Behavior {
			fmt.Printf("  - %s\n", b)
		}
		fmt.Println()
	}

	// Example
	if doc.Example != "" {
		fmt.Printf("%s\n", color(colorBold, "EXAMPLE"))
		// Indent example lines
		lines := strings.Split(doc.Example, "\n")
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}

	// See Also
	if len(doc.SeeAlso) > 0 {
		fmt.Printf("%s\n", color(colorBold, "SEE ALSO"))
		fmt.Printf("  %s\n", strings.Join(doc.SeeAlso, ", "))
	}

	return nil
}
