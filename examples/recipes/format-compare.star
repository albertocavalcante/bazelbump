# Compare buildifier vs skyfmt formatting
#
# This recipe runs both formatters on Starlark files and reports
# any differences between them. Useful for:
#   - Finding bugs in either formatter
#   - Understanding formatting style differences
#   - Validating skyfmt produces equivalent output to buildifier
#
# Prerequisites:
#   Both tools must be installed:
#   - buildifier: go install github.com/bazelbuild/buildtools/buildifier@latest
#   - skyfmt: Built from https://github.com/albertocavalcante/sky
#
# Usage:
#   bazelbump recipe run format-compare.star
#
# Output:
#   Shows files where buildifier and skyfmt produce different results.

def find_starlark_files(ctx):
    """Find all Starlark files for comparison."""
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
        for f in glob(pattern, exclude=["bazel-*/**", ".git/**", "*.backup"]):
            content = read_file(f, default="")
            if content:
                results.append({"file": f})

    return results

# Comparison script as a multi-line string
# Using string concatenation to avoid Starlark escape issues
COMPARE_SCRIPT = (
    "echo '=== Formatter Comparison: buildifier vs skyfmt ===' && " +
    "TMPDIR=$(mktemp -d) && " +
    "trap 'rm -rf $TMPDIR' EXIT && " +
    "DIFF_COUNT=0 && SAME_COUNT=0 && " +
    "for f in $(find . -name BUILD -o -name BUILD.bazel -o -name '*.bzl' " +
    "-o -name WORKSPACE -o -name MODULE.bazel 2>/dev/null | grep -v bazel- | grep -v .git); do " +
    "[ -f \"$f\" ] || continue; " +
    "buildifier \"$f\" > \"$TMPDIR/b\" 2>/dev/null || cat \"$f\" > \"$TMPDIR/b\"; " +
    "skyfmt \"$f\" > \"$TMPDIR/s\" 2>/dev/null || cat \"$f\" > \"$TMPDIR/s\"; " +
    "if diff -q \"$TMPDIR/b\" \"$TMPDIR/s\" >/dev/null 2>&1; then " +
    "SAME_COUNT=$((SAME_COUNT + 1)); " +
    "else " +
    "DIFF_COUNT=$((DIFF_COUNT + 1)); " +
    "echo \"DIFF: $f\"; " +
    "diff \"$TMPDIR/b\" \"$TMPDIR/s\" | head -20 || true; " +
    "fi; " +
    "done && " +
    "echo '=== Summary ===' && " +
    "echo \"Same: $SAME_COUNT, Different: $DIFF_COUNT\""
)

def compare_ops(ctx):
    """Generate comparison operations."""
    return [
        run_command(
            command = COMPARE_SCRIPT,
            description = "Compare buildifier vs skyfmt output",
            timeout = "10m",
            allow_failure = True,
            echo = True,     # Show what's being executed
            capture = True,  # Store output for potential later use
        ),
    ]

recipe(
    name = "format-compare",
    version = "1.0.0",
    description = "Compare buildifier vs skyfmt formatting output",
    steps = [
        # Step 1: Find all Starlark files
        scan(
            id = "find-files",
            name = "Find Starlark files",
            description = "Locate all Starlark files to compare",
            impl = find_starlark_files,
        ),

        # Step 2: Run comparison
        edit(
            id = "compare",
            name = "Compare formatters",
            description = "Run both formatters and report differences",
            depends_on = ["find-files"],
            operations = compare_ops,
        ),
    ],
)
