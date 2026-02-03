# Format Starlark files with buildifier
#
# This recipe formats BUILD, WORKSPACE, MODULE.bazel, and .bzl files
# using Google's buildifier tool.
#
# Prerequisites:
#   buildifier must be installed: https://github.com/bazelbuild/buildtools
#   go install github.com/bazelbuild/buildtools/buildifier@latest
#
# Usage:
#   bazelbump recipe run format-buildifier.star --dry-run  # Preview changes (check mode)
#   bazelbump recipe run format-buildifier.star            # Apply formatting
#
# A/B Testing:
#   Compare with format-skyfmt.star to find differences between formatters.

def find_starlark_files(ctx):
    """Find all Starlark files that can be formatted."""
    results = []

    patterns = [
        "**/BUILD",
        "**/BUILD.bazel",
        "**/*.bzl",
        "**/WORKSPACE",
        "**/WORKSPACE.bazel",
        "**/MODULE.bazel",
    ]

    for pattern in patterns:
        for f in glob(pattern, exclude=["bazel-*/**", ".git/**"]):
            results.append({"file": f})

    return results

def format_ops(ctx):
    """Generate formatting operations."""
    return [
        run_command(
            command = "buildifier -mode=fix .",
            description = "Format all Starlark files with buildifier",
            timeout = "5m",
            echo = True,  # Show what's being executed
        ),
    ]

recipe(
    name = "format-buildifier",
    version = "1.0.0",
    description = "Format Starlark files using buildifier",
    steps = [
        # Step 1: Find all Starlark files
        scan(
            id = "find-files",
            name = "Find Starlark files",
            description = "Locate BUILD, WORKSPACE, MODULE.bazel, and .bzl files",
            impl = find_starlark_files,
        ),

        # Step 2: Format files
        edit(
            id = "format",
            name = "Format with buildifier",
            description = "Apply buildifier formatting to all Starlark files",
            depends_on = ["find-files"],
            operations = format_ops,
        ),
    ],
)
