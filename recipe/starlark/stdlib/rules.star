# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

"""Standard library for bazelbump recipes - Rule mappings and patterns.

This module provides common constants and mappings for Bazel rule migrations.
These can be used in recipes to detect native rules and add appropriate
load statements and dependencies.

Usage:
    load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")
"""

# Pattern constants for detecting native rules in BUILD files.
# Each pattern is a regex that matches a rule invocation.
# The patterns are grouped by rule family for easy filtering.
NATIVE_RULE_PATTERNS = {
    "cc": [
        r"cc_binary\s*\(",
        r"cc_library\s*\(",
        r"cc_test\s*\(",
        r"cc_import\s*\(",
        r"cc_shared_library\s*\(",
    ],
    "python": [
        r"py_binary\s*\(",
        r"py_library\s*\(",
        r"py_test\s*\(",
    ],
    "java": [
        r"java_binary\s*\(",
        r"java_library\s*\(",
        r"java_test\s*\(",
        r"java_import\s*\(",
        r"java_plugin\s*\(",
    ],
    "shell": [
        r"sh_binary\s*\(",
        r"sh_test\s*\(",
        r"sh_library\s*\(",
    ],
    "proto": [
        r"proto_library\s*\(",
        r"cc_proto_library\s*\(",
        r"java_proto_library\s*\(",
        r"py_proto_library\s*\(",
    ],
    "android": [
        r"android_binary\s*\(",
        r"android_library\s*\(",
        r"android_test\s*\(",
        r"aar_import\s*\(",
    ],
    "go": [
        r"go_binary\s*\(",
        r"go_library\s*\(",
        r"go_test\s*\(",
    ],
}

# Load path mappings for native rules.
# Maps rule name to the load statement path.
RULE_LOAD_PATHS = {
    # CC rules
    "cc_binary": "@rules_cc//cc:defs.bzl",
    "cc_library": "@rules_cc//cc:defs.bzl",
    "cc_test": "@rules_cc//cc:defs.bzl",
    "cc_import": "@rules_cc//cc:defs.bzl",
    "cc_shared_library": "@rules_cc//cc:defs.bzl",

    # Python rules
    "py_binary": "@rules_python//python:defs.bzl",
    "py_library": "@rules_python//python:defs.bzl",
    "py_test": "@rules_python//python:defs.bzl",

    # Java rules
    "java_binary": "@rules_java//java:defs.bzl",
    "java_library": "@rules_java//java:defs.bzl",
    "java_test": "@rules_java//java:defs.bzl",
    "java_import": "@rules_java//java:defs.bzl",
    "java_plugin": "@rules_java//java:defs.bzl",

    # Shell rules
    "sh_binary": "@rules_shell//shell:sh_binary.bzl",
    "sh_test": "@rules_shell//shell:sh_test.bzl",
    "sh_library": "@rules_shell//shell:sh_library.bzl",

    # Proto rules
    "proto_library": "@protobuf//bazel:proto_library.bzl",
    "cc_proto_library": "@protobuf//bazel:cc_proto_library.bzl",
    "java_proto_library": "@protobuf//bazel:java_proto_library.bzl",
    "py_proto_library": "@protobuf//bazel:py_proto_library.bzl",

    # Android rules
    "android_binary": "@rules_android//android:rules.bzl",
    "android_library": "@rules_android//android:rules.bzl",
    "android_test": "@rules_android//android:rules.bzl",
    "aar_import": "@rules_android//android:rules.bzl",

    # Go rules
    "go_binary": "@rules_go//go:def.bzl",
    "go_library": "@rules_go//go:def.bzl",
    "go_test": "@rules_go//go:def.bzl",
}

# Recommended dependency versions for rule sets.
# These are the versions that should be added to MODULE.bazel.
# Updated as of late 2025 - update as needed.
RULE_DEP_VERSIONS = {
    "rules_cc": "0.1.1",
    "rules_python": "1.4.1",
    "rules_java": "8.6.3",
    "rules_shell": "0.4.1",
    "protobuf": "29.3",
    "rules_android": "0.6.0",
    "rules_go": "0.53.0",
    "gazelle": "0.42.0",
}

# Mapping from rule family to module dependency name
RULE_FAMILY_TO_DEP = {
    "cc": "rules_cc",
    "python": "rules_python",
    "java": "rules_java",
    "shell": "rules_shell",
    "proto": "protobuf",
    "android": "rules_android",
    "go": "rules_go",
}

# Helper function to get all patterns as a flat list
def all_patterns():
    """Return all native rule patterns as a flat list.

    Returns:
        list[string]: All regex patterns for detecting native rules.
    """
    result = []
    for patterns in NATIVE_RULE_PATTERNS.values():
        result.extend(patterns)
    return result

# Helper function to get patterns for specific families
def patterns_for_families(families):
    """Return patterns for the specified rule families.

    Args:
        families: list[string] - Rule families to get patterns for (e.g., ["cc", "python"])

    Returns:
        list[string]: Regex patterns for the specified families.
    """
    result = []
    for family in families:
        if family in NATIVE_RULE_PATTERNS:
            result.extend(NATIVE_RULE_PATTERNS[family])
    return result

# Helper function to get load path for a rule
def get_load_path(rule):
    """Return the load path for a rule.

    Args:
        rule: string - The rule name (e.g., "cc_binary")

    Returns:
        string: The load path, or None if unknown.
    """
    return RULE_LOAD_PATHS.get(rule)

# Helper function to get dependency version
def get_dep_version(dep_name):
    """Return the recommended version for a dependency.

    Args:
        dep_name: string - The dependency name (e.g., "rules_cc")

    Returns:
        string: The recommended version, or None if unknown.
    """
    return RULE_DEP_VERSIONS.get(dep_name)
