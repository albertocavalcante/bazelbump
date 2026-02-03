# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

"""CC rule migration helpers for bazelbump recipes.

This module provides helper functions and step generators for migrating
C/C++ native rules to explicit load statements.

Usage:
    load("@stdlib//migrations/cc.star", "cc_autoload_ops", "cc_migration_steps")
"""

load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")

# CC-specific rules
CC_RULES = [
    "cc_binary",
    "cc_library",
    "cc_test",
    "cc_import",
    "cc_shared_library",
]

def cc_autoload_ops(version = None):
    """Generate operations for CC rule autoload migration.

    Creates a list of operations that will:
    1. Add rules_cc dependency to MODULE.bazel
    2. Add load statements for all CC rules to BUILD files

    Args:
        version: string, optional - rules_cc version to use.
                 Defaults to RULE_DEP_VERSIONS["rules_cc"].

    Returns:
        list[Operation]: Operations to add CC rule support.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_cc"]

    ops = [
        ensure_bazel_dep("rules_cc", v),
    ]

    # Add load statements for each CC rule
    for rule in CC_RULES:
        load_path = RULE_LOAD_PATHS.get(rule)
        if load_path:
            ops.append(ensure_load(rule, load_path))

    return ops

def cc_migration_steps(version = None, verify_build = True, step_prefix = ""):
    """Generate complete CC migration recipe steps.

    Creates a full set of steps for migrating CC native rules:
    1. Scan for CC rule usage
    2. Add dependencies and load statements
    3. Optionally verify the build

    Args:
        version: string, optional - rules_cc version to use.
        verify_build: bool, optional - Whether to add a verify step. Default True.
        step_prefix: string, optional - Prefix for step IDs.

    Returns:
        list[Step]: Complete migration steps for CC rules.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_cc"]
    prefix = step_prefix if step_prefix else ""

    # Step 1: Detect CC rules
    detected = scan(
        id = prefix + "detect-cc",
        name = "Detect CC rule usage",
        description = "Scan BUILD files for cc_binary, cc_library, cc_test, etc.",
        files = ["BUILD", "BUILD.bazel"],
        patterns = NATIVE_RULE_PATTERNS["cc"],
    )

    # Step 2: Add dependencies and loads
    migrate = edit(
        id = prefix + "migrate-cc",
        name = "Add CC rule dependencies and loads",
        description = "Add rules_cc dependency and load statements",
        depends_on = [prefix + "detect-cc"],
        operations = [
            ensure_bazel_dep("rules_cc", v),
            ensure_load_for_detected(detected),
        ],
    )

    steps = [detected, migrate]

    # Step 3: Optional verification
    if verify_build:
        check = verify(
            id = prefix + "verify-cc",
            name = "Verify CC build",
            description = "Verify that CC targets still build correctly",
            depends_on = [prefix + "migrate-cc"],
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        )
        steps.append(check)

    return steps

def cc_load_statements():
    """Generate load statements for all CC rules.

    Returns a dictionary mapping load paths to the rules they provide.
    This is useful for generating or checking load statements.

    Returns:
        dict[string, list[string]]: Load path to rules mapping.
    """
    load_path = RULE_LOAD_PATHS["cc_binary"]
    return {
        load_path: CC_RULES,
    }
