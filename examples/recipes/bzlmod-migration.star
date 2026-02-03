# Bzlmod Migration Recipe
#
# This recipe helps migrate from WORKSPACE to bzlmod (MODULE.bazel).
# It performs the following steps:
# 1. Creates a MODULE.bazel file if it doesn't exist
# 2. Adds common dependencies based on detected rules
# 3. Updates .bazelrc to enable bzlmod
# 4. Optionally marks WORKSPACE as deprecated
#
# Usage:
#   bazelbump recipe run bzlmod-migration.star --dry-run
#   bazelbump recipe run bzlmod-migration.star
#
# Note: This is a starting point - you may need to manually adjust
# dependencies and their versions for your specific project.

# Default module name derived from directory
DEFAULT_MODULE_NAME = "my_project"

# Common rule patterns to detect
RULE_PATTERNS = [
    r"cc_(binary|library|test)\s*\(",
    r"java_(binary|library|test)\s*\(",
    r"py_(binary|library|test)\s*\(",
    r"sh_(binary|test|library)\s*\(",
    r"proto_library\s*\(",
    r"go_(binary|library|test)\s*\(",
]

# Dependency mappings
RULE_DEPS = {
    "cc": ("rules_cc", "0.1.1"),
    "java": ("rules_java", "8.6.3"),
    "python": ("rules_python", "1.4.1"),
    "shell": ("rules_shell", "0.4.1"),
    "proto": ("protobuf", "29.3"),
    "go": ("rules_go", "0.53.0"),
}

recipe(
    name = "bzlmod-migration",
    version = "1.0.0",
    description = "Migrate from WORKSPACE to bzlmod (MODULE.bazel)",
    min_bazel_version = "7.0.0",
    steps = [
        # Step 1: Scan for rule usage to determine needed dependencies
        scan(
            id = "detect-rules",
            name = "Detect rule usage",
            description = "Scan BUILD files to determine which rule sets are used",
            files = ["BUILD", "BUILD.bazel"],
            patterns = RULE_PATTERNS,
        ),

        # Step 2: Check if WORKSPACE exists
        scan(
            id = "detect-workspace",
            name = "Detect WORKSPACE file",
            description = "Check if a WORKSPACE file exists",
            files = ["WORKSPACE", "WORKSPACE.bazel"],
            patterns = [r"workspace\s*\(", r"http_archive\s*\(", r"git_repository\s*\("],
        ),

        # Step 3: Add common bazel_deps to MODULE.bazel
        # Note: This adds all common deps - you may want to customize based on detect-rules
        edit(
            id = "add-module-deps",
            name = "Add module dependencies",
            description = "Add bazel_dep entries to MODULE.bazel for detected rule sets",
            depends_on = ["detect-rules"],
            operations = [
                # Common dependencies that most projects need
                ensure_bazel_dep("rules_cc", "0.1.1"),
                ensure_bazel_dep("rules_java", "8.6.3"),
                ensure_bazel_dep("rules_python", "1.4.1"),
                ensure_bazel_dep("rules_shell", "0.4.1"),
            ],
        ),

        # Step 4: Update .bazelrc to enable bzlmod
        edit(
            id = "enable-bzlmod",
            name = "Enable bzlmod in .bazelrc",
            description = "Add flags to enable bzlmod and configure migration settings",
            depends_on = ["add-module-deps"],
            operations = [
                # Enable bzlmod (default in Bazel 7+, but explicit is good)
                ensure_flag("enable_bzlmod", value = "true", config = "common"),

                # Allow WORKSPACE to still work during migration
                ensure_flag("enable_workspace", value = "true", config = "common"),
            ],
        ),

        # Step 5: Add deprecation comment to WORKSPACE
        edit(
            id = "deprecate-workspace",
            name = "Mark WORKSPACE as deprecated",
            description = "Add comment to WORKSPACE indicating it's being migrated",
            depends_on = ["enable-bzlmod"],
            operations = [
                # This is a placeholder - in practice you might want to
                # add a comment at the top of WORKSPACE
                # For now, we'll leave WORKSPACE alone
            ],
        ),

        # Step 6: Verify the build works with bzlmod enabled
        verify(
            id = "verify-bzlmod",
            name = "Verify build with bzlmod",
            description = "Verify the build works with bzlmod enabled",
            depends_on = ["deprecate-workspace"],
            command = "bazel build //...",
            timeout = "15m",
            allow_failure = True,
        ),
    ],
    on_failure = "rollback",
)
