# Example: Project-Level Recipes

This directory demonstrates how to create project-specific recipes that are automatically discovered by bazelbump.

## Directory Structure

```
.bazelbump/
  recipes/
    my-custom-migration.star    # Discovered as "project" source
```

## How It Works

When you run `bazelbump recipe list` in this directory, the CLI automatically discovers recipes from:

1. **Project recipes** - `./.bazelbump/recipes/*.star` (highest priority)
2. **Project recipes (alt)** - `./recipes/*.star`
3. **User recipes** - `~/.config/bazelbump/recipes/*.star`
4. **Built-in recipes** - Embedded in the bazelbump binary

## Usage

```bash
# List available recipes (from this directory)
cd examples/project-recipes
bazelbump recipe list

# Output shows recipes with their source:
#   NAME                  SOURCE   VERSION   DESCRIPTION
#   my-custom-migration   project  1.0.0     Example project-specific migration recipe
#   autoload-migration    builtin  1.0.0     Migrate native rules to explicit load statements

# Show recipe details
bazelbump recipe show my-custom-migration

# Run the recipe
bazelbump recipe run my-custom-migration --dry-run
```

## Priority

When multiple recipes have the same name, the first one found wins:
- Project `.bazelbump/recipes/` beats `recipes/`
- Project beats user config
- User config beats built-in

This allows projects to override built-in recipes with custom implementations.
