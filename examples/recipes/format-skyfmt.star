# Format Starlark files with skyfmt
#
# This recipe formats BUILD, WORKSPACE, MODULE.bazel, and .bzl files
# using skyfmt from the sky toolchain.
#
# Prerequisites:
#   skyfmt must be installed from: https://github.com/albertocavalcante/sky
#   Or built locally: cd sky && bazel build //cmd/skyfmt
#
# Usage:
#   bazelbump recipe run format-skyfmt.star --dry-run  # Preview changes
#   bazelbump recipe run format-skyfmt.star            # Apply formatting
#
# A/B Testing:
#   Compare with format-buildifier.star to find differences between formatters.
#   Any differences may indicate bugs or behavior differences worth investigating.

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
            command = "skyfmt -w .",
            description = "Format all Starlark files with skyfmt",
            timeout = "5m",
            echo = True,  # Show what's being executed
        ),
    ]

recipe(
    name = "format-skyfmt",
    version = "1.0.0",
    description = "Format Starlark files using skyfmt",
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
            name = "Format with skyfmt",
            description = "Apply skyfmt formatting to all Starlark files",
            depends_on = ["find-files"],
            operations = format_ops,
        ),
    ],
)
