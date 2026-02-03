# Flag Cleanup Recipe
#
# This recipe removes deprecated incompatible flags from .bazelrc files.
# These flags were used to opt into behavior changes that are now the default
# in newer Bazel versions.
#
# Usage:
#   bazelbump recipe run flag-cleanup.star --dry-run
#   bazelbump recipe run flag-cleanup.star

# List of deprecated incompatible flags that should be removed
# These flags have been enabled by default and no longer have any effect
DEPRECATED_FLAGS = [
    # Bazel 7.0 removals
    "incompatible_disallow_empty_glob",
    "incompatible_no_implicit_file_export",
    "incompatible_use_cc_configure_from_rules_cc",
    "incompatible_require_linker_input_cc_api",

    # Bazel 8.0 removals
    "incompatible_enable_cc_toolchain_resolution",
    "incompatible_enable_android_toolchain_resolution",
    "incompatible_use_toolchain_resolution_for_java_rules",
    "incompatible_autoload_externally",
    "incompatible_auto_exec_groups",

    # Legacy flags that have been removed
    "incompatible_load_java_rules_from_bzl",
    "incompatible_load_proto_rules_from_bzl",
    "incompatible_load_cc_rules_from_bzl",
]

# Build operations to remove each deprecated flag
def build_remove_ops():
    """Build a list of remove_flag operations for all deprecated flags."""
    ops = []
    for flag in DEPRECATED_FLAGS:
        ops.append(remove_flag(flag))
    return ops

recipe(
    name = "flag-cleanup",
    version = "1.0.0",
    description = "Remove deprecated incompatible flags from .bazelrc",
    min_bazel_version = "7.0.0",
    steps = [
        # Step 1: Scan for deprecated flags
        scan(
            id = "detect-deprecated-flags",
            name = "Detect deprecated flags",
            description = "Scan .bazelrc files for deprecated incompatible flags",
            files = [".bazelrc", "*.bazelrc"],
            patterns = [
                r"--incompatible_disallow_empty_glob",
                r"--incompatible_no_implicit_file_export",
                r"--incompatible_use_cc_configure_from_rules_cc",
                r"--incompatible_require_linker_input_cc_api",
                r"--incompatible_enable_cc_toolchain_resolution",
                r"--incompatible_enable_android_toolchain_resolution",
                r"--incompatible_use_toolchain_resolution_for_java_rules",
                r"--incompatible_autoload_externally",
                r"--incompatible_auto_exec_groups",
                r"--incompatible_load_java_rules_from_bzl",
                r"--incompatible_load_proto_rules_from_bzl",
                r"--incompatible_load_cc_rules_from_bzl",
            ],
        ),

        # Step 2: Remove the deprecated flags
        edit(
            id = "remove-deprecated-flags",
            name = "Remove deprecated flags",
            description = "Remove deprecated incompatible flags from .bazelrc files",
            depends_on = ["detect-deprecated-flags"],
            operations = build_remove_ops(),
        ),

        # Step 3: Verify build still works
        verify(
            id = "verify-build",
            name = "Verify build works",
            description = "Verify the build still works after removing flags",
            depends_on = ["remove-deprecated-flags"],
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        ),
    ],
    on_failure = "rollback",
)
