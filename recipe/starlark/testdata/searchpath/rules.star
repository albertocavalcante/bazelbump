# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# A library file that will be loaded via search path (// prefix)

RULE_MAPPINGS = {
    "cc_binary": "@rules_cc//cc:defs.bzl",
    "cc_library": "@rules_cc//cc:defs.bzl",
    "py_binary": "@rules_python//python:defs.bzl",
    "py_library": "@rules_python//python:defs.bzl",
}

def get_load_path(rule_name):
    """Get the load path for a rule name.

    Args:
        rule_name: The name of the rule

    Returns:
        The load path or None if not found
    """
    return RULE_MAPPINGS.get(rule_name, None)
