# Autoload Migration Recipe
#
# This recipe migrates native Bazel rules to explicit load statements,
# which is required for Bazel 9.0+ where autoload is disabled by default.
#
# Usage:
#   bazelbump recipe run autoload-migration.star --dry-run
#   bazelbump recipe run autoload-migration.star

recipe(
    name = "autoload-migration",
    version = "1.0.0",
    description = "Migrate native rules to explicit load statements for Bazel 9.0+",
    min_bazel_version = "8.0.0",
    steps = [
        # Step 1: Detect native rule usage
        scan(
            id = "detect-native-rules",
            name = "Detect native rule usage",
            files = ["BUILD", "BUILD.bazel"],
            patterns = [
                r"cc_(binary|library|test|import|shared_library)\s*\(",
                r"java_(binary|library|test|import|plugin)\s*\(",
                r"py_(binary|library|test)\s*\(",
                r"sh_(binary|test|library)\s*\(",
                r"proto_library\s*\(",
            ],
        ),

        # Step 2: Add required module dependencies
        edit(
            id = "add-module-deps",
            name = "Add bazel_dep entries to MODULE.bazel",
            depends_on = ["detect-native-rules"],
            operations = [
                ensure_bazel_dep("rules_cc", "0.1.1"),
                ensure_bazel_dep("rules_java", "8.6.3"),
                ensure_bazel_dep("rules_python", "1.4.1"),
                ensure_bazel_dep("rules_shell", "0.4.1"),
            ],
        ),

        # Step 3: Add load statements to BUILD files
        edit(
            id = "add-load-statements",
            name = "Add load statements for native rules",
            depends_on = ["add-module-deps"],
            operations = [
                # CC rules
                ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
                ensure_load("cc_library", "@rules_cc//cc:defs.bzl"),
                ensure_load("cc_test", "@rules_cc//cc:defs.bzl"),

                # Java rules
                ensure_load("java_binary", "@rules_java//java:defs.bzl"),
                ensure_load("java_library", "@rules_java//java:defs.bzl"),
                ensure_load("java_test", "@rules_java//java:defs.bzl"),

                # Python rules
                ensure_load("py_binary", "@rules_python//python:defs.bzl"),
                ensure_load("py_library", "@rules_python//python:defs.bzl"),
                ensure_load("py_test", "@rules_python//python:defs.bzl"),

                # Shell rules
                ensure_load("sh_binary", "@rules_shell//shell:sh_binary.bzl"),
                ensure_load("sh_test", "@rules_shell//shell:sh_test.bzl"),
                ensure_load("sh_library", "@rules_shell//shell:sh_library.bzl"),
            ],
        ),

        # Step 4: Verify the build still works
        verify(
            id = "verify-build",
            name = "Verify build works",
            depends_on = ["add-load-statements"],
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        ),
    ],
    on_failure = "rollback",
)
