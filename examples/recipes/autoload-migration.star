# Autoload Migration Recipe
# =========================
#
# CRITICAL RECIPE for Bazel 8 -> 9 Migration
#
# In Bazel 9.0+, "autoloading" of native rules is disabled by default. Previously,
# you could use rules like `cc_library`, `java_binary`, `py_test`, etc. directly
# in BUILD files without any load() statements. Now you must explicitly load them.
#
# This recipe automatically:
#   1. Scans all BUILD/BUILD.bazel files for native rule usage
#   2. Detects which rules need load statements
#   3. Adds required bazel_dep() entries to MODULE.bazel
#   4. Adds load() statements only to files that need them
#   5. Skips files that already have the required loads
#
# BEFORE:
#   cc_library(name = "foo", ...)
#
# AFTER:
#   load("@rules_cc//cc:defs.bzl", "cc_library")
#   cc_library(name = "foo", ...)
#
# Supported Rule Families:
#   - C/C++:   cc_binary, cc_library, cc_test, cc_import, cc_shared_library
#   - Java:    java_binary, java_library, java_test, java_import, java_plugin
#   - Python:  py_binary, py_library, py_test
#   - Shell:   sh_binary, sh_test, sh_library
#   - Proto:   proto_library, cc_proto_library, java_proto_library, py_proto_library
#   - Android: android_binary, android_library, android_test, aar_import
#
# Prerequisites:
#   - Project must use bzlmod (MODULE.bazel must exist)
#   - Bazel 8.0+ recommended
#
# Usage:
#   # Preview changes (recommended first)
#   bazelbump recipe run examples/recipes/autoload-migration.star --dry-run
#
#   # Apply the migration
#   bazelbump recipe run examples/recipes/autoload-migration.star
#
#   # Run with verbose output
#   bazelbump recipe run examples/recipes/autoload-migration.star --verbose
#
# Rollback:
#   If verification fails, changes are automatically rolled back.
#   You can also use: bazelbump undo
#
# Notes:
#   - The recipe is idempotent - running it twice won't duplicate loads
#   - Existing loads are preserved and not duplicated
#   - Files with no native rules are skipped entirely
#   - The verify step is optional (allow_failure=True) - migration proceeds
#     even if the build has other issues
#
# See Also:
#   - docs/recipes/autoload-migration.md for detailed documentation
#   - examples/recipes/flag-cleanup.star to remove related deprecated flags
#   - examples/recipes/bzlmod-migration.star for WORKSPACE -> bzlmod migration
#
# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# Load the standard library for rule mappings and patterns.
# This provides consistent, up-to-date mappings across all recipes.
load("@stdlib//rules.star",
    "NATIVE_RULE_PATTERNS",
    "RULE_LOAD_PATHS",
    "RULE_DEP_VERSIONS",
    "RULE_FAMILY_TO_DEP",
    "all_patterns",
)

# =============================================================================
# Configuration
# =============================================================================

# Rule families to migrate. Comment out any families you don't need.
# Each family will get its own bazel_dep and load statements.
FAMILIES_TO_MIGRATE = [
    "cc",       # C/C++: cc_binary, cc_library, cc_test, etc.
    "java",     # Java: java_binary, java_library, java_test, etc.
    "python",   # Python: py_binary, py_library, py_test
    "shell",    # Shell: sh_binary, sh_test, sh_library
    "proto",    # Protobuf: proto_library, cc_proto_library, etc.
    # "android",  # Android: android_binary, android_library, etc. (uncomment if needed)
]

# Override dependency versions if needed (leave empty to use defaults)
# Example: VERSION_OVERRIDES = {"rules_cc": "0.2.0"}
VERSION_OVERRIDES = {}

# =============================================================================
# Helper Functions
# =============================================================================

def get_patterns_for_families():
    """Get all regex patterns for the families we want to migrate."""
    patterns = []
    for family in FAMILIES_TO_MIGRATE:
        if family in NATIVE_RULE_PATTERNS:
            patterns.extend(NATIVE_RULE_PATTERNS[family])
    return patterns

def get_dep_version(dep_name):
    """Get the dependency version, respecting any overrides."""
    if dep_name in VERSION_OVERRIDES:
        return VERSION_OVERRIDES[dep_name]
    return RULE_DEP_VERSIONS.get(dep_name, "0.0.0")

def build_dependency_operations():
    """Build operations to add bazel_dep entries for each rule family."""
    ops = []
    seen_deps = {}

    for family in FAMILIES_TO_MIGRATE:
        dep_name = RULE_FAMILY_TO_DEP.get(family)
        if dep_name and dep_name not in seen_deps:
            seen_deps[dep_name] = True
            version = get_dep_version(dep_name)
            ops.append(ensure_bazel_dep(dep_name, version))

    return ops

# =============================================================================
# Recipe Definition
# =============================================================================

# Step 1: Detect native rule usage
# --------------------------------
# Scans all BUILD and BUILD.bazel files for native rule invocations.
# Excludes bazel-* directories and any rules that already have load statements.
detect_native_rules = scan(
    id = "detect-native-rules",
    name = "Detect native rule usage",
    description = "Find BUILD files using native rules that need load statements",
    files = ["BUILD", "BUILD.bazel"],
    patterns = get_patterns_for_families(),
)

# Step 2: Add module dependencies
# -------------------------------
# Ensures the required bazel_dep() entries exist in MODULE.bazel.
# This step depends on detection to ensure we only proceed if rules were found.
add_module_deps = edit(
    id = "add-module-deps",
    name = "Add bazel_dep entries to MODULE.bazel",
    description = "Add required rules_* dependencies for detected rule families",
    depends_on = ["detect-native-rules"],
    operations = build_dependency_operations(),
)

# Step 3: Add load statements
# ---------------------------
# Uses ensure_load_for_detected to automatically add load() statements
# to each BUILD file based on which rules were detected in that file.
# This is intelligent - it only adds loads for rules actually used in each file.
add_load_statements = edit(
    id = "add-load-statements",
    name = "Add load statements for native rules",
    description = "Add load() statements to BUILD files that use native rules",
    depends_on = ["add-module-deps"],
    operations = [
        ensure_load_for_detected(detect_native_rules),
    ],
)

# Step 4: Verify the build (optional)
# -----------------------------------
# Runs a build to verify the changes work. This is optional and won't
# block the migration if it fails (allow_failure=True).
verify_build = verify(
    id = "verify-build",
    name = "Verify build works",
    description = "Run 'bazel build //...' to verify the migration succeeded",
    depends_on = ["add-load-statements"],
    command = "bazel build //...",
    timeout = "10m",
    allow_failure = True,  # Don't fail the recipe if build has other issues
)

# =============================================================================
# Recipe Registration
# =============================================================================

recipe(
    name = "autoload-migration",
    version = "2.0.0",
    description = "Migrate native rules to explicit load statements for Bazel 9.0+",

    # Version constraints
    min_bazel_version = "8.0.0",  # Recipe is relevant starting from Bazel 8

    # Steps are executed in dependency order
    steps = [
        detect_native_rules,
        add_module_deps,
        add_load_statements,
        verify_build,
    ],

    # Rollback all changes if any step fails
    on_failure = "rollback",
)
