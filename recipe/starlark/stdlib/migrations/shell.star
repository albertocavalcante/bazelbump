# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

"""Shell rule migration helpers for bazelbump recipes.

This module provides helper functions and step generators for migrating
shell native rules to explicit load statements.

Usage:
    load("@stdlib//migrations/shell.star", "shell_autoload_ops", "shell_migration_steps")
"""

load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")

# Shell-specific rules
SHELL_RULES = [
    "sh_binary",
    "sh_test",
    "sh_library",
]

def shell_autoload_ops(version = None):
    """Generate operations for shell rule autoload migration.

    Creates a list of operations that will:
    1. Add rules_shell dependency to MODULE.bazel
    2. Add load statements for all shell rules to BUILD files

    Note: Shell rules have separate .bzl files for each rule type,
    unlike other rule families that use a single defs.bzl.

    Args:
        version: string, optional - rules_shell version to use.
                 Defaults to RULE_DEP_VERSIONS["rules_shell"].

    Returns:
        list[Operation]: Operations to add shell rule support.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_shell"]

    ops = [
        ensure_bazel_dep("rules_shell", v),
    ]

    # Add load statements for each shell rule
    # Note: Each shell rule has its own .bzl file
    for rule in SHELL_RULES:
        load_path = RULE_LOAD_PATHS.get(rule)
        if load_path:
            ops.append(ensure_load(rule, load_path))

    return ops

def shell_migration_steps(version = None, verify_build = True, step_prefix = ""):
    """Generate complete shell migration recipe steps.

    Creates a full set of steps for migrating shell native rules:
    1. Scan for shell rule usage
    2. Add dependencies and load statements
    3. Optionally verify the build

    Args:
        version: string, optional - rules_shell version to use.
        verify_build: bool, optional - Whether to add a verify step. Default True.
        step_prefix: string, optional - Prefix for step IDs.

    Returns:
        list[Step]: Complete migration steps for shell rules.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_shell"]
    prefix = step_prefix if step_prefix else ""

    # Step 1: Detect shell rules
    detected = scan(
        id = prefix + "detect-shell",
        name = "Detect shell rule usage",
        description = "Scan BUILD files for sh_binary, sh_test, sh_library",
        files = ["BUILD", "BUILD.bazel"],
        patterns = NATIVE_RULE_PATTERNS["shell"],
    )

    # Step 2: Add dependencies and loads
    migrate = edit(
        id = prefix + "migrate-shell",
        name = "Add shell rule dependencies and loads",
        description = "Add rules_shell dependency and load statements",
        depends_on = [prefix + "detect-shell"],
        operations = [
            ensure_bazel_dep("rules_shell", v),
            ensure_load_for_detected(detected),
        ],
    )

    steps = [detected, migrate]

    # Step 3: Optional verification
    if verify_build:
        check = verify(
            id = prefix + "verify-shell",
            name = "Verify shell build",
            description = "Verify that shell targets still build correctly",
            depends_on = [prefix + "migrate-shell"],
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        )
        steps.append(check)

    return steps

def shell_load_statements():
    """Generate load statements for all shell rules.

    Returns a dictionary mapping load paths to the rules they provide.
    Note that shell rules use separate .bzl files for each rule type.

    Returns:
        dict[string, list[string]]: Load path to rules mapping.
    """
    return {
        RULE_LOAD_PATHS["sh_binary"]: ["sh_binary"],
        RULE_LOAD_PATHS["sh_test"]: ["sh_test"],
        RULE_LOAD_PATHS["sh_library"]: ["sh_library"],
    }
