# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

"""Java rule migration helpers for bazelbump recipes.

This module provides helper functions and step generators for migrating
Java native rules to explicit load statements.

Usage:
    load("@stdlib//migrations/java.star", "java_autoload_ops", "java_migration_steps")
"""

load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")

# Java-specific rules
JAVA_RULES = [
    "java_binary",
    "java_library",
    "java_test",
    "java_import",
    "java_plugin",
]

def java_autoload_ops(version = None):
    """Generate operations for Java rule autoload migration.

    Creates a list of operations that will:
    1. Add rules_java dependency to MODULE.bazel
    2. Add load statements for all Java rules to BUILD files

    Args:
        version: string, optional - rules_java version to use.
                 Defaults to RULE_DEP_VERSIONS["rules_java"].

    Returns:
        list[Operation]: Operations to add Java rule support.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_java"]

    ops = [
        ensure_bazel_dep("rules_java", v),
    ]

    # Add load statements for each Java rule
    for rule in JAVA_RULES:
        load_path = RULE_LOAD_PATHS.get(rule)
        if load_path:
            ops.append(ensure_load(rule, load_path))

    return ops

def java_migration_steps(version = None, verify_build = True, step_prefix = ""):
    """Generate complete Java migration recipe steps.

    Creates a full set of steps for migrating Java native rules:
    1. Scan for Java rule usage
    2. Add dependencies and load statements
    3. Optionally verify the build

    Args:
        version: string, optional - rules_java version to use.
        verify_build: bool, optional - Whether to add a verify step. Default True.
        step_prefix: string, optional - Prefix for step IDs.

    Returns:
        list[Step]: Complete migration steps for Java rules.
    """
    v = version if version else RULE_DEP_VERSIONS["rules_java"]
    prefix = step_prefix if step_prefix else ""

    # Step 1: Detect Java rules
    detected = scan(
        id = prefix + "detect-java",
        name = "Detect Java rule usage",
        description = "Scan BUILD files for java_binary, java_library, java_test, etc.",
        files = ["BUILD", "BUILD.bazel"],
        patterns = NATIVE_RULE_PATTERNS["java"],
    )

    # Step 2: Add dependencies and loads
    migrate = edit(
        id = prefix + "migrate-java",
        name = "Add Java rule dependencies and loads",
        description = "Add rules_java dependency and load statements",
        depends_on = [prefix + "detect-java"],
        operations = [
            ensure_bazel_dep("rules_java", v),
            ensure_load_for_detected(detected),
        ],
    )

    steps = [detected, migrate]

    # Step 3: Optional verification
    if verify_build:
        check = verify(
            id = prefix + "verify-java",
            name = "Verify Java build",
            description = "Verify that Java targets still build correctly",
            depends_on = [prefix + "migrate-java"],
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        )
        steps.append(check)

    return steps

def java_load_statements():
    """Generate load statements for all Java rules.

    Returns a dictionary mapping load paths to the rules they provide.
    This is useful for generating or checking load statements.

    Returns:
        dict[string, list[string]]: Load path to rules mapping.
    """
    load_path = RULE_LOAD_PATHS["java_binary"]
    return {
        load_path: JAVA_RULES,
    }

def java_toolchain_ops(java_version = "21"):
    """Generate operations to configure Java toolchain.

    Creates operations to set up Java toolchain registration
    in MODULE.bazel for the specified Java version.

    Args:
        java_version: string - Java version to configure (e.g., "21", "17", "11")

    Returns:
        list[Operation]: Operations to configure Java toolchain.
    """
    return [
        ensure_bazel_dep("rules_java", RULE_DEP_VERSIONS["rules_java"]),
    ]
