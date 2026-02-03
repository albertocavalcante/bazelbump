# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# Helper functions for recipes

# Constant exported for use by other recipes
DEFAULT_VERSION = "1.0.0"

# Another constant
CC_PATTERNS = [r"cc_binary\s*\(", r"cc_library\s*\("]

def make_scan_step(step_id, file_patterns, search_patterns):
    """Create a scan step with common defaults.

    Args:
        step_id: The unique ID for this step
        file_patterns: List of file patterns to match
        search_patterns: List of regex patterns to search

    Returns:
        A scan step object
    """
    return scan(
        id = step_id,
        name = "Scan for patterns",
        files = file_patterns,
        patterns = search_patterns,
    )

def make_edit_step(step_id, dep_name, dep_version):
    """Create an edit step that adds a bazel_dep.

    Args:
        step_id: The unique ID for this step
        dep_name: Name of the dependency
        dep_version: Version of the dependency

    Returns:
        An edit step object
    """
    return edit(
        id = step_id,
        operations = [
            ensure_bazel_dep(dep_name, dep_version),
        ],
    )
