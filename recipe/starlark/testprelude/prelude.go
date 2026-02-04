// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package testprelude provides a Starlark prelude for testing bazelbump recipes.
// It generates mock implementations of bazelbump builtins that can be used with
// the skytest testing framework.
package testprelude

import (
	"os"
	"path/filepath"
)

// PreludeContent contains the Starlark prelude for recipe testing.
// Uses struct builtin (available in skytest) for clean attribute access.
const PreludeContent = `# Auto-generated prelude for bazelbump recipe tests
# This provides mock implementations of bazelbump builtins for testing.
#
# Uses struct for attribute access: result.field instead of result["field"]
# Requires skytest with struct builtin (PR #37+)

# =============================================================================
# Step builtins
# =============================================================================

def recipe(name, version, steps, description=None, min_bazel_version=None, max_bazel_version=None, requires=None, on_failure="rollback"):
    """Define a migration recipe."""
    return struct(
        name=name,
        version=version,
        steps=steps,
        description=description,
        min_bazel_version=min_bazel_version,
        max_bazel_version=max_bazel_version,
        requires=requires or [],
        on_failure=on_failure,
    )

def scan(id, name=None, description=None, files=None, patterns=None, impl=None):
    """Scan files for patterns and produce matches."""
    return struct(
        type="scan",
        id=id,
        name=name or id,
        description=description,
        files=files or [],
        patterns=patterns or [],
        impl=impl,
        # Mock scan result fields
        matches=[],
        count=0,
        rules=[],
        families=[],
    )

def edit(id, operations, name=None, description=None, depends_on=None):
    """Apply transformations to files."""
    return struct(
        type="edit",
        id=id,
        name=name or id,
        description=description,
        operations=operations if type(operations) == "list" else [],
        depends_on=depends_on or [],
    )

def verify(id, command, name=None, description=None, depends_on=None, timeout="5m", allow_failure=False):
    """Run a verification command."""
    return struct(
        type="verify",
        id=id,
        name=name or id,
        description=description,
        command=command,
        depends_on=depends_on or [],
        timeout=timeout,
        allow_failure=allow_failure,
    )

# =============================================================================
# Operation builtins - BUILD files
# =============================================================================

def ensure_load(rule, load_from, file=None):
    """Add load() statement for a Bazel rule."""
    return struct(type="ensure_load", rule=rule, load_from=load_from, file=file)

def ensure_load_for_detected(detected, mapping=None):
    """Add load() statements for all rules found by a scan."""
    return struct(type="ensure_load_for_detected", detected=detected, mapping=mapping)

def remove_load(rule, file=None):
    """Remove a symbol from load() statements."""
    return struct(type="remove_load", rule=rule, file=file)

# =============================================================================
# Operation builtins - MODULE.bazel
# =============================================================================

def ensure_bazel_dep(name, version, dev_dependency=False):
    """Add bazel_dep() entry to MODULE.bazel."""
    return struct(type="ensure_bazel_dep", name=name, version=version, dev_dependency=dev_dependency)

def update_bazel_dep(name, version):
    """Update version of an existing bazel_dep()."""
    return struct(type="update_bazel_dep", name=name, version=version)

def remove_bazel_dep(name):
    """Remove bazel_dep() from MODULE.bazel."""
    return struct(type="remove_bazel_dep", name=name)

# =============================================================================
# Operation builtins - .bazelrc
# =============================================================================

def remove_flag(flag, file=".bazelrc", config=None):
    """Remove flag from .bazelrc."""
    return struct(type="remove_flag", flag=flag, file=file, config=config)

def replace_flag(old, new, file=".bazelrc"):
    """Replace one flag with another in .bazelrc."""
    return struct(type="replace_flag", old=old, new=new, file=file)

def ensure_flag(flag, value=None, config="build", file=".bazelrc"):
    """Ensure a flag exists in .bazelrc."""
    return struct(type="ensure_flag", flag=flag, value=value, config=config, file=file)

# =============================================================================
# Operation builtins - Generic
# =============================================================================

def replace_text(old, new, files, regex=False):
    """Replace text in files."""
    return struct(type="replace_text", old=old, new=new, files=files, regex=regex)

def copy_file(src, dst):
    """Copy a file."""
    return struct(type="copy_file", src=src, dst=dst)

def delete_file(path):
    """Delete a file."""
    return struct(type="delete_file", path=path)

def run_command(command, description=None, timeout="5m", allow_failure=False, echo=False, capture=False, env=None, working_dir=None, shell="sh"):
    """Execute a shell command."""
    return struct(
        type="run_command",
        command=command,
        description=description,
        timeout=timeout,
        allow_failure=allow_failure,
        echo=echo,
        capture=capture,
        env=env,
        working_dir=working_dir,
        shell=shell,
    )

# =============================================================================
# Utility builtins
# =============================================================================

def fail(msg):
    """Stop execution with an error (in tests, just returns the operation)."""
    return struct(type="fail", msg=msg)

def warn(msg):
    """Print a warning but continue."""
    return struct(type="warn", msg=msg)

def skip(msg):
    """Skip the current step."""
    return struct(type="skip", msg=msg)

# =============================================================================
# File I/O builtins (mock implementations)
# =============================================================================

def read_file(path, default=None):
    """Read file contents as string (mock)."""
    if default != None:
        return default
    return ""

def file_exists(path):
    """Check if file or directory exists (mock)."""
    return False

def glob_files(pattern, exclude=[]):
    """Find files matching a glob pattern (mock)."""
    return []

def list_dir(path, recursive=False):
    """List directory contents (mock)."""
    return []

def parse_build_file(path):
    """Parse a BUILD file and return structure (mock)."""
    return struct(
        rules=[],
        loads=[],
    )

# =============================================================================
# Context builtins (mock implementations)
# =============================================================================

def get_project_context():
    """Get project context information (mock)."""
    return struct(
        root=".",
        bazel_version="8.0.0",
        use_bzlmod=True,
        workspace_name="test_workspace",
    )

def get_bazel_deps():
    """Get list of bazel_dep() entries from MODULE.bazel (mock)."""
    return []

def get_bazel_compatibility():
    """Get bazel_compatibility constraints from MODULE.bazel (mock)."""
    return []

# =============================================================================
# Version builtins
# =============================================================================

def parse_version(s):
    """Parse version string into components."""
    parts = s.split(".")
    major = int(parts[0]) if len(parts) > 0 else 0
    minor = int(parts[1]) if len(parts) > 1 else 0
    patch_str = parts[2] if len(parts) > 2 else "0"
    # Handle prerelease
    prerelease = ""
    if "-" in patch_str:
        idx = patch_str.index("-")
        prerelease = patch_str[idx+1:]
        patch_str = patch_str[:idx]
    patch = int(patch_str) if patch_str.isdigit() else 0
    return struct(
        major=major,
        minor=minor,
        patch=patch,
        prerelease=prerelease,
        string=s,
    )

def version_compare(v1, v2):
    """Compare two version strings. Returns -1, 0, or 1."""
    p1 = parse_version(v1)
    p2 = parse_version(v2)

    if p1.major != p2.major:
        return -1 if p1.major < p2.major else 1
    if p1.minor != p2.minor:
        return -1 if p1.minor < p2.minor else 1
    if p1.patch != p2.patch:
        return -1 if p1.patch < p2.patch else 1

    # Prerelease versions are less than release versions
    if p1.prerelease and not p2.prerelease:
        return -1
    if not p1.prerelease and p2.prerelease:
        return 1
    if p1.prerelease < p2.prerelease:
        return -1
    if p1.prerelease > p2.prerelease:
        return 1

    return 0

def version_matches(version, constraints):
    """Check if version satisfies BCR-style constraints (mock)."""
    return True

def version_at_least(version, minimum):
    """Check if version >= minimum."""
    return version_compare(version, minimum) >= 0

def version_in_range(version, min_ver, max_ver):
    """Check if min <= version <= max."""
    return version_compare(version, min_ver) >= 0 and version_compare(version, max_ver) <= 0

# =============================================================================
# Module namespaces (as structs with callable attributes)
# =============================================================================

# project module
project = struct(
    context=get_project_context,
    deps=get_bazel_deps,
    compatibility=get_bazel_compatibility,
)

# version module
version = struct(
    parse=parse_version,
    compare=version_compare,
    matches=version_matches,
    at_least=version_at_least,
    in_range=version_in_range,
)

# fs module
fs = struct(
    read=read_file,
    exists=file_exists,
    glob=glob_files,
    list_dir=list_dir,
    parse_build=parse_build_file,
)

# =============================================================================
# Test helpers - for tracking operations in tests
# =============================================================================

def make_operation_tracker():
    """Create a fresh operation tracker for use in tests.

    Returns a struct with:
    - operations: list to store operations
    - record: function to record an operation
    - clear: function to clear operations
    - get_by_type: function to filter by type

    Usage in tests:
        tracker = make_operation_tracker()
        tracker.record("ensure_load", rule="cc_library")
        assert.len(tracker.operations, 1)
    """
    ops = []

    def record(op_type, **kwargs):
        op = struct(type=op_type, **kwargs)
        ops.append(op)
        return op

    def clear():
        for _ in range(len(ops)):
            ops.pop()

    def get_by_type(op_type):
        return [op for op in ops if op.type == op_type]

    return struct(
        operations=ops,
        record=record,
        clear=clear,
        get_by_type=get_by_type,
    )
`

// Generate returns the Starlark prelude content for recipe testing.
func Generate() string {
	return PreludeContent
}

// GenerateToFile writes the prelude to a temporary file and returns:
// - the path to the file
// - a cleanup function to remove the file
// - any error that occurred
func GenerateToFile() (string, func(), error) {
	// Create a temporary directory for the prelude
	tmpDir, err := os.MkdirTemp("", "bazelbump-test-prelude-*")
	if err != nil {
		return "", nil, err
	}

	// Write the prelude file
	preludePath := filepath.Join(tmpDir, "bazelbump_test_prelude.star")
	if err := os.WriteFile(preludePath, []byte(PreludeContent), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, err
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return preludePath, cleanup, nil
}
