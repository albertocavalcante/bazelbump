# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

"""Python rule migration helpers for bazelbump recipes.

This module provides helper functions and step generators for migrating
Python native rules to explicit load statements.

Usage:
    load("@stdlib//migrations/python.star", "python_autoload_ops", "python_migration_steps")
"""

load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")

# Python-specific rules
PYTHON_RULES = [
    "py_binary",
    "py_library",
    "py_test",
]

def python_autoload_ops(version = None):
    """Generate operations for Python rule autoload migration.

    Creates a list of operations that will:
    1. Add rules_python dependency to MODULE.bazel
    2. Add load statements for all Python rules to BUILD files

    Args:
        version: string, optional - rules_python version to use.
                 Defaults to RULE_DEP_VERSIONS["rules_python"].

    Returns:
        list[Operation]: Operations to add Python rule support.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_python"]

    ops = [
        ensure_bazel_dep("rules_python", v),
    ]

    # Add load statements for each Python rule
    for rule in PYTHON_RULES:
        load_path = RULE_LOAD_PATHS.get(rule)
        if load_path:
            ops.append(ensure_load(rule, load_path))

    return ops

def python_migration_steps(version = None, verify_build = True, step_prefix = ""):
    """Generate complete Python migration recipe steps.

    Creates a full set of steps for migrating Python native rules:
    1. Scan for Python rule usage
    2. Add dependencies and load statements
    3. Optionally verify the build

    Args:
        version: string, optional - rules_python version to use.
        verify_build: bool, optional - Whether to add a verify step. Default True.
        step_prefix: string, optional - Prefix for step IDs.

    Returns:
        list[Step]: Complete migration steps for Python rules.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_python"]
    prefix = step_prefix if step_prefix else ""

    # Step 1: Detect Python rules
    detected = scan(
        id = prefix + "detect-python",
        name = "Detect Python rule usage",
        description = "Scan BUILD files for py_binary, py_library, py_test",
        files = ["BUILD", "BUILD.bazel"],
        patterns = NATIVE_RULE_PATTERNS["python"],
    )

    # Step 2: Add dependencies and loads
    migrate = edit(
        id = prefix + "migrate-python",
        name = "Add Python rule dependencies and loads",
        description = "Add rules_python dependency and load statements",
        depends_on = [prefix + "detect-python"],
        operations = [
            ensure_bazel_dep("rules_python", v),
            ensure_load_for_detected(detected),
        ],
    )

    steps = [detected, migrate]

    # Step 3: Optional verification
    if verify_build:
        check = verify(
            id = prefix + "verify-python",
            name = "Verify Python build",
            description = "Verify that Python targets still build correctly",
            depends_on = [prefix + "migrate-python"],
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        )
        steps.append(check)

    return steps

def python_load_statements():
    """Generate load statements for all Python rules.

    Returns a dictionary mapping load paths to the rules they provide.
    This is useful for generating or checking load statements.

    Returns:
        dict[string, list[string]]: Load path to rules mapping.
    """
    load_path = RULE_LOAD_PATHS["py_binary"]
    return {
        load_path: PYTHON_RULES,
    }

def python_toolchain_ops(python_version = "3.11"):
    """Generate operations to configure Python toolchain.

    Creates operations to set up Python toolchain registration
    in MODULE.bazel for the specified Python version.

    Args:
        python_version: string - Python version to configure (e.g., "3.11")

    Returns:
        list[Operation]: Operations to configure Python toolchain.
    """
    # These operations would add the python.toolchain call
    # For now, just ensure the dependency is added
    return [
        ensure_bazel_dep("rules_python", RULE_DEP_VERSIONS["rules_python"]),
    ]
