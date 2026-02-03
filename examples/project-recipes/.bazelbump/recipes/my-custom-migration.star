# Example custom project recipe
#
# This recipe is discovered automatically from .bazelbump/recipes/
# when running bazelbump in this project.
#
# Usage:
#   cd examples/project-recipes
#   bazelbump recipe list          # Shows this recipe as "project" source
#   bazelbump recipe show my-custom-migration
#   bazelbump recipe run my-custom-migration --dry-run

recipe(
    name = "my-custom-migration",
    version = "1.0.0",
    description = "Example project-specific migration recipe",
    steps = [
        scan(
            id = "detect-custom",
            name = "Detect custom patterns",
            files = ["**/BUILD", "**/BUILD.bazel"],
            patterns = [r"custom_rule\s*\("],
        ),
        edit(
            id = "add-loads",
            name = "Add load statements",
            depends_on = ["detect-custom"],
            operations = [
                # Add load for custom rule
                ensure_load(
                    rule = "custom_rule",
                    load_from = "//tools:custom_rule.bzl",
                ),
            ],
        ),
    ],
)
