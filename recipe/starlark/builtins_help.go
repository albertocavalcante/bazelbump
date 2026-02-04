// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

// BuiltinDoc contains documentation for a single builtin function.
type BuiltinDoc struct {
	// Name is the builtin function name.
	Name string

	// Category is the category of the builtin ("step", "operation", "fileio", "util", "regex").
	Category string

	// Signature is the function signature (e.g., "ensure_load(rule, load_from, file=None)").
	Signature string

	// Description is a short description of what the builtin does.
	Description string

	// Args describes each argument.
	Args []ArgDoc

	// Behavior describes the detailed behavior of the builtin.
	Behavior []string

	// Example shows usage examples.
	Example string

	// SeeAlso lists related builtins.
	SeeAlso []string
}

// ArgDoc describes a single argument to a builtin function.
type ArgDoc struct {
	// Name is the argument name.
	Name string

	// Type is the argument type (e.g., "str", "list", "bool").
	Type string

	// Required indicates if the argument is required.
	Required bool

	// Description describes the argument.
	Description string
}

// AllBuiltinDocs returns documentation for all available builtins.
func AllBuiltinDocs() []BuiltinDoc {
	return []BuiltinDoc{
		// Steps
		docRecipe(),
		docScan(),
		docEdit(),
		docVerify(),

		// Operations - BUILD file
		docEnsureLoad(),
		docEnsureLoadForDetected(),
		docRemoveLoad(),

		// Operations - MODULE.bazel
		docEnsureBazelDep(),
		docUpdateBazelDep(),
		docRemoveBazelDep(),

		// Operations - .bazelrc
		docRemoveFlag(),
		docReplaceFlag(),
		docEnsureFlag(),

		// Operations - Generic
		docReplaceText(),
		docCopyFile(),
		docDeleteFile(),
		docRunCommand(),

		// Modules - project
		docProjectContext(),
		docProjectDeps(),
		docProjectCompatibility(),

		// Modules - version
		docVersionParse(),
		docVersionCompareModule(),
		docVersionMatchesModule(),
		docVersionAtLeastModule(),
		docVersionInRangeModule(),

		// Modules - fs
		docFsRead(),
		docFsExists(),
		docFsGlob(),
		docFsListDir(),
		docFsParseBuild(),

		// Modules - flags
		docFlagsExists(),
		docFlagsGet(),
		docFlagsDiff(),
		docFlagsIncompatible(),
		docFlagsDeprecated(),

		// File I/O (DEPRECATED - use fs module)
		docReadFile(),
		docFileExists(),
		docGlob(),
		docListDir(),
		docParseBuildFile(),

		// Project Context (DEPRECATED - use project module)
		docGetProjectContext(),
		docGetBazelDeps(),
		docGetBazelCompatibility(),

		// Version Utilities (DEPRECATED - use version module)
		docVersionMatches(),
		docVersionCompare(),
		docParseVersion(),
		docVersionAtLeast(),
		docVersionInRange(),

		// Utilities
		docFail(),
		docWarn(),
		docSkip(),

		// Regex
		docReMatch(),
		docReFindall(),
		docReSub(),
		docReSplit(),
	}
}

// GetBuiltinDoc returns documentation for a specific builtin by name.
// Returns nil if not found.
func GetBuiltinDoc(name string) *BuiltinDoc {
	for _, doc := range AllBuiltinDocs() {
		if doc.Name == name {
			return &doc
		}
	}
	return nil
}

// GetBuiltinsByCategory returns all builtins in a specific category.
func GetBuiltinsByCategory(category string) []BuiltinDoc {
	var result []BuiltinDoc
	for _, doc := range AllBuiltinDocs() {
		if doc.Category == category {
			result = append(result, doc)
		}
	}
	return result
}

// BuiltinCategories returns all available categories in display order.
func BuiltinCategories() []string {
	return []string{
		"step", "operation",
		"module-project", "module-version", "module-fs", "module-flags",
		"fileio", "context", "version",
		"util", "regex",
	}
}

// CategoryDisplayName returns the display name for a category.
func CategoryDisplayName(category string) string {
	switch category {
	case "step":
		return "STEPS"
	case "operation":
		return "OPERATIONS"
	case "module-project":
		return "PROJECT MODULE (project.*)"
	case "module-version":
		return "VERSION MODULE (version.*)"
	case "module-fs":
		return "FILESYSTEM MODULE (fs.*)"
	case "module-flags":
		return "FLAGS MODULE (flags.*)"
	case "fileio":
		return "FILE I/O (DEPRECATED)"
	case "context":
		return "PROJECT CONTEXT (DEPRECATED)"
	case "version":
		return "VERSION UTILITIES (DEPRECATED)"
	case "util":
		return "UTILITIES"
	case "regex":
		return "REGEX (re module)"
	default:
		return category
	}
}

// =============================================================================
// Step Builtins
// =============================================================================

func docRecipe() BuiltinDoc {
	return BuiltinDoc{
		Name:        "recipe",
		Category:    "step",
		Signature:   "recipe(name, version, steps, description=None, min_bazel_version=None, max_bazel_version=None, on_failure=\"rollback\")",
		Description: "Define a migration recipe",
		Args: []ArgDoc{
			{Name: "name", Type: "str", Required: true, Description: "Unique identifier for the recipe"},
			{Name: "version", Type: "str", Required: true, Description: "Semantic version (e.g., \"1.0.0\")"},
			{Name: "steps", Type: "list", Required: true, Description: "List of scan/edit/verify steps"},
			{Name: "description", Type: "str", Required: false, Description: "Human-readable description"},
			{Name: "min_bazel_version", Type: "str", Required: false, Description: "Minimum Bazel version"},
			{Name: "max_bazel_version", Type: "str", Required: false, Description: "Maximum Bazel version"},
			{Name: "on_failure", Type: "str", Required: false, Description: "Failure mode: \"rollback\", \"stop\", or \"continue\""},
		},
		Behavior: []string{
			"Must be called exactly once per recipe file",
			"Steps are executed in order, respecting depends_on",
			"on_failure controls behavior when a step fails",
		},
		Example: `recipe(
    name = "my-migration",
    version = "1.0.0",
    description = "Migrate native rules to rules_cc",
    steps = [detect, apply_loads, verify_build],
)`,
		SeeAlso: []string{"scan", "edit", "verify"},
	}
}

func docScan() BuiltinDoc {
	return BuiltinDoc{
		Name:        "scan",
		Category:    "step",
		Signature:   "scan(id, name=None, description=None, files=None, patterns=None, impl=None)",
		Description: "Scan files for patterns and produce matches",
		Args: []ArgDoc{
			{Name: "id", Type: "str", Required: true, Description: "Unique step identifier"},
			{Name: "name", Type: "str", Required: false, Description: "Display name for the step"},
			{Name: "description", Type: "str", Required: false, Description: "Step description"},
			{Name: "files", Type: "list", Required: false, Description: "Glob patterns for files to scan"},
			{Name: "patterns", Type: "list", Required: false, Description: "Regex patterns to match"},
			{Name: "impl", Type: "callable", Required: false, Description: "Custom scan function(ctx)"},
		},
		Behavior: []string{
			"Either provide (files + patterns) or impl, not both",
			"Returns a scan result that can be used by later steps",
			"Results include: matches, files, count, rules, families",
		},
		Example: `detect = scan(
    id = "detect-native",
    name = "Detect native rules",
    files = ["**/BUILD", "**/BUILD.bazel"],
    patterns = ["\\b(cc_binary|cc_library|cc_test)\\s*\\("],
)`,
		SeeAlso: []string{"edit", "ensure_load_for_detected"},
	}
}

func docEdit() BuiltinDoc {
	return BuiltinDoc{
		Name:        "edit",
		Category:    "step",
		Signature:   "edit(id, operations, name=None, description=None, depends_on=None)",
		Description: "Apply transformations to files",
		Args: []ArgDoc{
			{Name: "id", Type: "str", Required: true, Description: "Unique step identifier"},
			{Name: "operations", Type: "list|callable", Required: true, Description: "Operations to perform or function(ctx)"},
			{Name: "name", Type: "str", Required: false, Description: "Display name for the step"},
			{Name: "description", Type: "str", Required: false, Description: "Step description"},
			{Name: "depends_on", Type: "list", Required: false, Description: "Step IDs this step depends on"},
		},
		Behavior: []string{
			"Operations can be a list or a callable returning a list",
			"Operations are applied in order",
			"Changes are tracked for potential rollback",
		},
		Example: `edit(
    id = "add-loads",
    name = "Add load statements",
    depends_on = ["detect-native"],
    operations = [
        ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
        ensure_load("cc_library", "@rules_cc//cc:defs.bzl"),
    ],
)`,
		SeeAlso: []string{"scan", "verify", "ensure_load"},
	}
}

func docVerify() BuiltinDoc {
	return BuiltinDoc{
		Name:        "verify",
		Category:    "step",
		Signature:   "verify(id, command, name=None, description=None, depends_on=None, timeout=\"5m\", allow_failure=False)",
		Description: "Run a verification command",
		Args: []ArgDoc{
			{Name: "id", Type: "str", Required: true, Description: "Unique step identifier"},
			{Name: "command", Type: "str", Required: true, Description: "Shell command to execute"},
			{Name: "name", Type: "str", Required: false, Description: "Display name for the step"},
			{Name: "description", Type: "str", Required: false, Description: "Step description"},
			{Name: "depends_on", Type: "list", Required: false, Description: "Step IDs this step depends on"},
			{Name: "timeout", Type: "str", Required: false, Description: "Command timeout (default: \"5m\")"},
			{Name: "allow_failure", Type: "bool", Required: false, Description: "Continue on failure (default: False)"},
		},
		Behavior: []string{
			"Executes the command in a shell",
			"Fails the step if command returns non-zero (unless allow_failure)",
			"Timeout is parsed as a Go duration (e.g., \"5m\", \"30s\")",
		},
		Example: `verify(
    id = "verify-build",
    name = "Verify build works",
    depends_on = ["add-loads"],
    command = "bazel build //...",
    timeout = "10m",
)`,
		SeeAlso: []string{"scan", "edit"},
	}
}

// =============================================================================
// Operation Builtins - BUILD files
// =============================================================================

func docEnsureLoad() BuiltinDoc {
	return BuiltinDoc{
		Name:        "ensure_load",
		Category:    "operation",
		Signature:   "ensure_load(rule, load_from, file=None)",
		Description: "Add load() statement for a Bazel rule",
		Args: []ArgDoc{
			{Name: "rule", Type: "str", Required: true, Description: "The rule/symbol name to load"},
			{Name: "load_from", Type: "str", Required: true, Description: "The .bzl file path to load from"},
			{Name: "file", Type: "str", Required: false, Description: "Specific BUILD file, or all if None"},
		},
		Behavior: []string{
			"If the load already exists, this is a no-op (idempotent)",
			"If there's an existing load from the same path, adds to it",
			"Creates new load statement if needed",
		},
		Example: `ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")
ensure_load("py_test", "@rules_python//python:defs.bzl", "tests/BUILD")`,
		SeeAlso: []string{"ensure_bazel_dep", "ensure_load_for_detected", "parse_build_file"},
	}
}

func docEnsureLoadForDetected() BuiltinDoc {
	return BuiltinDoc{
		Name:        "ensure_load_for_detected",
		Category:    "operation",
		Signature:   "ensure_load_for_detected(detected, mapping=None)",
		Description: "Add load() statements for all rules found by a scan",
		Args: []ArgDoc{
			{Name: "detected", Type: "scan_result", Required: true, Description: "Result from a scan() step"},
			{Name: "mapping", Type: "dict", Required: false, Description: "Override rule -> load path mapping"},
		},
		Behavior: []string{
			"Reads the rules found by the scan step",
			"Uses built-in mapping for native rules (cc_*, py_*, java_*)",
			"Custom mapping overrides the built-in mapping",
		},
		Example: `ensure_load_for_detected(
    detected = detect_step,
    mapping = {"my_rule": "@my_rules//:defs.bzl"},
)`,
		SeeAlso: []string{"scan", "ensure_load"},
	}
}

func docRemoveLoad() BuiltinDoc {
	return BuiltinDoc{
		Name:        "remove_load",
		Category:    "operation",
		Signature:   "remove_load(rule, file=None)",
		Description: "Remove a symbol from load() statements",
		Args: []ArgDoc{
			{Name: "rule", Type: "str", Required: true, Description: "The rule/symbol to remove"},
			{Name: "file", Type: "str", Required: false, Description: "Specific BUILD file, or all if None"},
		},
		Behavior: []string{
			"Removes the symbol from any load statements that include it",
			"If the load statement becomes empty, removes it entirely",
			"No-op if the symbol is not loaded",
		},
		Example: `remove_load("native_binary")
remove_load("old_rule", "legacy/BUILD")`,
		SeeAlso: []string{"ensure_load"},
	}
}

// =============================================================================
// Operation Builtins - MODULE.bazel
// =============================================================================

func docEnsureBazelDep() BuiltinDoc {
	return BuiltinDoc{
		Name:        "ensure_bazel_dep",
		Category:    "operation",
		Signature:   "ensure_bazel_dep(name, version, dev_dependency=False)",
		Description: "Add bazel_dep() entry to MODULE.bazel",
		Args: []ArgDoc{
			{Name: "name", Type: "str", Required: true, Description: "The dependency name (e.g., \"rules_cc\")"},
			{Name: "version", Type: "str", Required: true, Description: "The version to require"},
			{Name: "dev_dependency", Type: "bool", Required: false, Description: "Mark as dev-only dependency"},
		},
		Behavior: []string{
			"If the dependency exists with the same or higher version, no-op",
			"If the dependency exists with lower version, logs a warning",
			"Creates MODULE.bazel if it doesn't exist",
		},
		Example: `ensure_bazel_dep("rules_cc", "0.1.1")
ensure_bazel_dep("rules_testing", "0.0.4", dev_dependency=True)`,
		SeeAlso: []string{"update_bazel_dep", "remove_bazel_dep"},
	}
}

func docUpdateBazelDep() BuiltinDoc {
	return BuiltinDoc{
		Name:        "update_bazel_dep",
		Category:    "operation",
		Signature:   "update_bazel_dep(name, version)",
		Description: "Update version of an existing bazel_dep()",
		Args: []ArgDoc{
			{Name: "name", Type: "str", Required: true, Description: "The dependency name to update"},
			{Name: "version", Type: "str", Required: true, Description: "The new version"},
		},
		Behavior: []string{
			"Updates the version of an existing dependency",
			"Fails if the dependency doesn't exist (use ensure_bazel_dep instead)",
		},
		Example: `update_bazel_dep("rules_cc", "0.2.0")`,
		SeeAlso: []string{"ensure_bazel_dep", "remove_bazel_dep"},
	}
}

func docRemoveBazelDep() BuiltinDoc {
	return BuiltinDoc{
		Name:        "remove_bazel_dep",
		Category:    "operation",
		Signature:   "remove_bazel_dep(name)",
		Description: "Remove bazel_dep() from MODULE.bazel",
		Args: []ArgDoc{
			{Name: "name", Type: "str", Required: true, Description: "The dependency name to remove"},
		},
		Behavior: []string{
			"Removes the bazel_dep entry for the given name",
			"No-op if the dependency doesn't exist",
		},
		Example: `remove_bazel_dep("deprecated_rules")`,
		SeeAlso: []string{"ensure_bazel_dep", "update_bazel_dep"},
	}
}

// =============================================================================
// Operation Builtins - .bazelrc
// =============================================================================

func docRemoveFlag() BuiltinDoc {
	return BuiltinDoc{
		Name:        "remove_flag",
		Category:    "operation",
		Signature:   "remove_flag(flag, file=\".bazelrc\", config=None)",
		Description: "Remove flag from .bazelrc",
		Args: []ArgDoc{
			{Name: "flag", Type: "str", Required: true, Description: "The flag name to remove (e.g., \"incompatible_foo\")"},
			{Name: "file", Type: "str", Required: false, Description: "The bazelrc file (default: \".bazelrc\")"},
			{Name: "config", Type: "str", Required: false, Description: "Limit to specific config section"},
		},
		Behavior: []string{
			"Removes all occurrences of --flag and --noflag",
			"If config is specified, only removes from that config section",
			"Handles both boolean and value flags",
		},
		Example: `remove_flag("incompatible_enable_cc_toolchain_resolution")
remove_flag("experimental_flag", config="ci")`,
		SeeAlso: []string{"replace_flag", "ensure_flag"},
	}
}

func docReplaceFlag() BuiltinDoc {
	return BuiltinDoc{
		Name:        "replace_flag",
		Category:    "operation",
		Signature:   "replace_flag(old, new, file=\".bazelrc\")",
		Description: "Replace one flag with another in .bazelrc",
		Args: []ArgDoc{
			{Name: "old", Type: "str", Required: true, Description: "The flag to replace"},
			{Name: "new", Type: "str", Required: true, Description: "The replacement flag (empty to remove)"},
			{Name: "file", Type: "str", Required: false, Description: "The bazelrc file (default: \".bazelrc\")"},
		},
		Behavior: []string{
			"Finds and replaces the flag in all occurrences",
			"If new is empty, effectively removes the flag",
			"Preserves the config section context",
		},
		Example: `replace_flag("--incompatible_old_flag", "--incompatible_new_flag")
replace_flag("--deprecated_flag=value", "")`,
		SeeAlso: []string{"remove_flag", "ensure_flag"},
	}
}

func docEnsureFlag() BuiltinDoc {
	return BuiltinDoc{
		Name:        "ensure_flag",
		Category:    "operation",
		Signature:   "ensure_flag(flag, value=None, config=\"build\", file=\".bazelrc\")",
		Description: "Ensure a flag exists in .bazelrc",
		Args: []ArgDoc{
			{Name: "flag", Type: "str", Required: true, Description: "The flag name to add"},
			{Name: "value", Type: "str", Required: false, Description: "The flag value (for non-boolean flags)"},
			{Name: "config", Type: "str", Required: false, Description: "Config section (default: \"build\")"},
			{Name: "file", Type: "str", Required: false, Description: "The bazelrc file (default: \".bazelrc\")"},
		},
		Behavior: []string{
			"Adds the flag if not present in the specified config",
			"No-op if the flag already exists",
			"Creates the config section if needed",
		},
		Example: `ensure_flag("incompatible_enable_cc_toolchain_resolution")
ensure_flag("jobs", value="auto", config="build:ci")`,
		SeeAlso: []string{"remove_flag", "replace_flag"},
	}
}

// =============================================================================
// Operation Builtins - Generic
// =============================================================================

func docReplaceText() BuiltinDoc {
	return BuiltinDoc{
		Name:        "replace_text",
		Category:    "operation",
		Signature:   "replace_text(old, new, files, regex=False)",
		Description: "Replace text in files",
		Args: []ArgDoc{
			{Name: "old", Type: "str", Required: true, Description: "Text or pattern to find"},
			{Name: "new", Type: "str", Required: true, Description: "Replacement text"},
			{Name: "files", Type: "list", Required: true, Description: "Glob patterns for files to modify"},
			{Name: "regex", Type: "bool", Required: false, Description: "Treat old as regex (default: False)"},
		},
		Behavior: []string{
			"Replaces all occurrences in matching files",
			"When regex=True, supports capture groups ($1, $2, etc.)",
			"Files not matching any pattern are skipped",
		},
		Example: `replace_text("old_name", "new_name", ["**/BUILD"])
replace_text(r"version = \"\\d+\"", "version = \"2.0\"", ["*.bzl"], regex=True)`,
		SeeAlso: []string{"copy_file", "delete_file"},
	}
}

func docCopyFile() BuiltinDoc {
	return BuiltinDoc{
		Name:        "copy_file",
		Category:    "operation",
		Signature:   "copy_file(src, dst)",
		Description: "Copy a file",
		Args: []ArgDoc{
			{Name: "src", Type: "str", Required: true, Description: "Source file path"},
			{Name: "dst", Type: "str", Required: true, Description: "Destination file path"},
		},
		Behavior: []string{
			"Creates parent directories if needed",
			"Overwrites destination if it exists",
			"Preserves file permissions",
		},
		Example: `copy_file("templates/BUILD.tpl", "new_package/BUILD")`,
		SeeAlso: []string{"delete_file", "replace_text"},
	}
}

func docDeleteFile() BuiltinDoc {
	return BuiltinDoc{
		Name:        "delete_file",
		Category:    "operation",
		Signature:   "delete_file(path)",
		Description: "Delete a file",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "File path or glob pattern"},
		},
		Behavior: []string{
			"Deletes the file at the given path",
			"No-op if the file doesn't exist",
			"Supports glob patterns for multiple files",
		},
		Example: `delete_file("deprecated/BUILD")
delete_file("**/*.bak")`,
		SeeAlso: []string{"copy_file", "replace_text"},
	}
}

func docRunCommand() BuiltinDoc {
	return BuiltinDoc{
		Name:        "run_command",
		Category:    "operation",
		Signature:   "run_command(command, description=None, timeout=\"5m\", allow_failure=False, echo=False, capture=False, env=None, working_dir=None, shell=\"sh\")",
		Description: "Execute a shell command with optional echo, capture, and environment",
		Args: []ArgDoc{
			{Name: "command", Type: "str", Required: true, Description: "Shell command to execute"},
			{Name: "description", Type: "str", Required: false, Description: "Human-readable description"},
			{Name: "timeout", Type: "str", Required: false, Description: "Command timeout (default: \"5m\")"},
			{Name: "allow_failure", Type: "bool", Required: false, Description: "Continue on failure (default: False)"},
			{Name: "echo", Type: "bool", Required: false, Description: "Print command before executing (like set -x)"},
			{Name: "capture", Type: "bool", Required: false, Description: "Store stdout/stderr in ctx.outputs for later steps"},
			{Name: "env", Type: "dict", Required: false, Description: "Environment variables to set"},
			{Name: "working_dir", Type: "str", Required: false, Description: "Working directory (relative to project)"},
			{Name: "shell", Type: "str", Required: false, Description: "Shell to use: \"sh\" (default) or \"bash\""},
		},
		Behavior: []string{
			"Executes command via the specified shell",
			"echo=True prints command before execution (like set -x)",
			"capture=True stores {stdout, stderr, exit_code} in ctx.outputs",
			"In dry-run mode, shows what would be executed",
			"Environment variables merge with existing environment",
		},
		Example: `# Simple command with echo (set -x style)
run_command(
    command = "buildifier -mode=fix .",
    echo = True,
)

# Capture output for later use
run_command(
    command = "git rev-parse HEAD",
    capture = True,
    echo = True,
)
# Later: ctx.outputs["step-id"]["stdout"]

# With environment and working directory
run_command(
    command = "npm install",
    working_dir = "frontend/",
    env = {"NODE_ENV": "production"},
    echo = True,
)

# Use bash for advanced features
run_command(
    command = "for f in *.txt; do echo $f; done",
    shell = "bash",
)`,
		SeeAlso: []string{"verify", "replace_text"},
	}
}

// =============================================================================
// File I/O Builtins
// =============================================================================

func docReadFile() BuiltinDoc {
	return BuiltinDoc{
		Name:        "read_file",
		Category:    "fileio",
		Signature:   "read_file(path, default=None)",
		Description: "Read file contents as string",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Path to the file (relative to project)"},
			{Name: "default", Type: "str", Required: false, Description: "Default value if file doesn't exist"},
		},
		Behavior: []string{
			"Returns file contents as a string",
			"If file doesn't exist and default is provided, returns default",
			"If file doesn't exist and no default, raises an error",
			"Path is sandboxed to project directory",
		},
		Example: `content = read_file("MODULE.bazel")
version = read_file(".bazel-version", default="7.0.0")`,
		SeeAlso: []string{"file_exists", "glob"},
	}
}

func docFileExists() BuiltinDoc {
	return BuiltinDoc{
		Name:        "file_exists",
		Category:    "fileio",
		Signature:   "file_exists(path)",
		Description: "Check if file or directory exists",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Path to check (relative to project)"},
		},
		Behavior: []string{
			"Returns True if the file or directory exists",
			"Returns False otherwise",
			"Path is sandboxed to project directory",
		},
		Example: `if file_exists("MODULE.bazel"):
    # Running with bzlmod
    pass`,
		SeeAlso: []string{"read_file", "glob"},
	}
}

func docGlob() BuiltinDoc {
	return BuiltinDoc{
		Name:        "glob",
		Category:    "fileio",
		Signature:   "glob(pattern, exclude=[])",
		Description: "Find files matching a glob pattern",
		Args: []ArgDoc{
			{Name: "pattern", Type: "str", Required: true, Description: "Glob pattern (supports **)"},
			{Name: "exclude", Type: "list", Required: false, Description: "Patterns to exclude"},
		},
		Behavior: []string{
			"Returns list of matching file paths (relative to project)",
			"** matches any number of directories",
			"Hidden directories (starting with .) are skipped",
			"Path is sandboxed to project directory",
		},
		Example: `build_files = glob("**/BUILD")
star_files = glob("**/*.star", exclude=["**/test_*.star"])`,
		SeeAlso: []string{"list_dir", "file_exists"},
	}
}

func docListDir() BuiltinDoc {
	return BuiltinDoc{
		Name:        "list_dir",
		Category:    "fileio",
		Signature:   "list_dir(path, recursive=False)",
		Description: "List directory contents",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Directory path (relative to project)"},
			{Name: "recursive", Type: "bool", Required: false, Description: "Include subdirectories (default: False)"},
		},
		Behavior: []string{
			"Returns list of file/directory names",
			"If recursive=True, returns relative paths for all files",
			"Path is sandboxed to project directory",
		},
		Example: `entries = list_dir("src")
all_files = list_dir(".", recursive=True)`,
		SeeAlso: []string{"glob", "file_exists"},
	}
}

func docParseBuildFile() BuiltinDoc {
	return BuiltinDoc{
		Name:        "parse_build_file",
		Category:    "fileio",
		Signature:   "parse_build_file(path)",
		Description: "Parse a BUILD file and return structure",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Path to BUILD file"},
		},
		Behavior: []string{
			"Returns a BuildFile object with rules() and loads() methods",
			"rules(kind=None) returns list of rules, optionally filtered by kind",
			"loads() returns list of load statements",
			"Each rule has: name, kind, line, attr(name) method",
		},
		Example: `bf = parse_build_file("src/BUILD")
for rule in bf.rules("cc_*"):
    print(rule.name, rule.kind)
for load in bf.loads():
    print(load.path, load.symbols)`,
		SeeAlso: []string{"ensure_load", "glob"},
	}
}

// =============================================================================
// Utility Builtins
// =============================================================================

func docFail() BuiltinDoc {
	return BuiltinDoc{
		Name:        "fail",
		Category:    "util",
		Signature:   "fail(msg)",
		Description: "Stop execution with an error",
		Args: []ArgDoc{
			{Name: "msg", Type: "str", Required: true, Description: "Error message"},
		},
		Behavior: []string{
			"Immediately stops recipe execution",
			"Triggers rollback if on_failure=\"rollback\"",
			"Use for unrecoverable errors",
		},
		Example: `if not file_exists("MODULE.bazel"):
    fail("This recipe requires bzlmod")`,
		SeeAlso: []string{"warn", "skip"},
	}
}

func docWarn() BuiltinDoc {
	return BuiltinDoc{
		Name:        "warn",
		Category:    "util",
		Signature:   "warn(msg)",
		Description: "Print a warning but continue",
		Args: []ArgDoc{
			{Name: "msg", Type: "str", Required: true, Description: "Warning message"},
		},
		Behavior: []string{
			"Prints the warning message to stderr",
			"Does not stop execution",
			"Use for non-critical issues",
		},
		Example: `if len(bf.rules("cc_*")) == 0:
    warn("No cc rules found, nothing to migrate")`,
		SeeAlso: []string{"fail", "skip"},
	}
}

func docSkip() BuiltinDoc {
	return BuiltinDoc{
		Name:        "skip",
		Category:    "util",
		Signature:   "skip(msg)",
		Description: "Skip the current step",
		Args: []ArgDoc{
			{Name: "msg", Type: "str", Required: true, Description: "Skip reason message"},
		},
		Behavior: []string{
			"Marks the current step as skipped",
			"Continues with the next step",
			"Use when preconditions are not met",
		},
		Example: `def custom_impl(ctx):
    if not ctx.get("detected"):
        return skip("No rules detected")
    # ...`,
		SeeAlso: []string{"fail", "warn"},
	}
}

// =============================================================================
// Regex Builtins (re module)
// =============================================================================

func docReMatch() BuiltinDoc {
	return BuiltinDoc{
		Name:        "re.match",
		Category:    "regex",
		Signature:   "re.match(pattern, text)",
		Description: "Match pattern at start of string",
		Args: []ArgDoc{
			{Name: "pattern", Type: "str", Required: true, Description: "Regex pattern"},
			{Name: "text", Type: "str", Required: true, Description: "Text to match against"},
		},
		Behavior: []string{
			"Returns match object if pattern matches at start",
			"Returns None if no match",
			"Use re.search() for matching anywhere in string",
		},
		Example: `m = re.match(r"version = \"(\\d+\\.\\d+)\"", line)
if m:
    version = m.group(1)`,
		SeeAlso: []string{"re.findall", "re.sub"},
	}
}

func docReFindall() BuiltinDoc {
	return BuiltinDoc{
		Name:        "re.findall",
		Category:    "regex",
		Signature:   "re.findall(pattern, text)",
		Description: "Find all matches of pattern",
		Args: []ArgDoc{
			{Name: "pattern", Type: "str", Required: true, Description: "Regex pattern"},
			{Name: "text", Type: "str", Required: true, Description: "Text to search"},
		},
		Behavior: []string{
			"Returns list of all non-overlapping matches",
			"If pattern has groups, returns list of groups",
			"Returns empty list if no matches",
		},
		Example: `rules = re.findall(r"(cc_\\w+)\\s*\\(", content)
# rules = ["cc_binary", "cc_library", ...]`,
		SeeAlso: []string{"re.match", "re.sub"},
	}
}

func docReSub() BuiltinDoc {
	return BuiltinDoc{
		Name:        "re.sub",
		Category:    "regex",
		Signature:   "re.sub(pattern, repl, text)",
		Description: "Replace pattern matches with replacement",
		Args: []ArgDoc{
			{Name: "pattern", Type: "str", Required: true, Description: "Regex pattern"},
			{Name: "repl", Type: "str", Required: true, Description: "Replacement string"},
			{Name: "text", Type: "str", Required: true, Description: "Text to modify"},
		},
		Behavior: []string{
			"Replaces all occurrences of pattern with repl",
			"Supports backreferences ($1, $2, etc.)",
			"Returns the modified string",
		},
		Example: `# Update version numbers
new_content = re.sub(
    r'version = "\\d+\\.\\d+\\.\\d+"',
    'version = "2.0.0"',
    content
)`,
		SeeAlso: []string{"re.match", "re.findall"},
	}
}

func docReSplit() BuiltinDoc {
	return BuiltinDoc{
		Name:        "re.split",
		Category:    "regex",
		Signature:   "re.split(pattern, text)",
		Description: "Split string by pattern",
		Args: []ArgDoc{
			{Name: "pattern", Type: "str", Required: true, Description: "Regex pattern to split on"},
			{Name: "text", Type: "str", Required: true, Description: "Text to split"},
		},
		Behavior: []string{
			"Splits the string at each match of pattern",
			"Returns list of substrings",
			"Empty strings may be included at boundaries",
		},
		Example: `parts = re.split(r"\\s*,\\s*", "a, b,  c")
# parts = ["a", "b", "c"]`,
		SeeAlso: []string{"re.findall", "re.sub"},
	}
}

// =============================================================================
// Project Context Builtins
// =============================================================================

func docGetProjectContext() BuiltinDoc {
	return BuiltinDoc{
		Name:        "get_project_context",
		Category:    "context",
		Signature:   "get_project_context()",
		Description: "Get project context information (memoized per run)",
		Args:        []ArgDoc{},
		Behavior: []string{
			"Detects project root, Bazel version, and module configuration",
			"Results are cached for the duration of the recipe execution",
			"Requires project path to be set (automatic when running recipes)",
		},
		Example: `ctx = get_project_context()
if ctx.use_bzlmod:
    print("Using bzlmod at", ctx.root)
if ctx.bazel_version:
    print("Bazel version:", ctx.bazel_version)`,
		SeeAlso: []string{"get_bazel_deps", "get_bazel_compatibility", "version_at_least"},
	}
}

func docGetBazelDeps() BuiltinDoc {
	return BuiltinDoc{
		Name:        "get_bazel_deps",
		Category:    "context",
		Signature:   "get_bazel_deps()",
		Description: "Get list of bazel_dep() entries from MODULE.bazel",
		Args:        []ArgDoc{},
		Behavior: []string{
			"Parses MODULE.bazel and extracts all bazel_dep entries",
			"Returns empty list if MODULE.bazel doesn't exist",
			"Each entry is a dict with name, version, repo_name, dev_dependency",
		},
		Example: `deps = get_bazel_deps()
for dep in deps:
    print(dep["name"], "at version", dep["version"])
    if dep["dev_dependency"]:
        print("  (dev dependency)")`,
		SeeAlso: []string{"get_project_context", "ensure_bazel_dep"},
	}
}

func docGetBazelCompatibility() BuiltinDoc {
	return BuiltinDoc{
		Name:        "get_bazel_compatibility",
		Category:    "context",
		Signature:   "get_bazel_compatibility()",
		Description: "Get bazel_compatibility constraints from MODULE.bazel",
		Args:        []ArgDoc{},
		Behavior: []string{
			"Extracts the bazel_compatibility attribute from module()",
			"Returns list of constraint strings (BCR format)",
			"Returns empty list if no compatibility constraints defined",
		},
		Example: `compat = get_bazel_compatibility()
# compat might be [">=6.4.0", "<=9.0.0", "-7.1.0"]
ctx = get_project_context()
if ctx.bazel_version and not version_matches(ctx.bazel_version, compat):
    fail("Project Bazel version doesn't match compatibility constraints")`,
		SeeAlso: []string{"get_project_context", "version_matches"},
	}
}

// =============================================================================
// Version Utility Builtins
// =============================================================================

func docVersionMatches() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version_matches",
		Category:    "version",
		Signature:   "version_matches(version, constraints)",
		Description: "Check if version satisfies BCR-style constraints",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Version string to check (e.g., \"7.7.1\")"},
			{Name: "constraints", Type: "list[str]", Required: true, Description: "List of constraint strings"},
		},
		Behavior: []string{
			"Supports BCR constraint operators: >=, <=, >, <, ==, !=",
			"Supports exclusion with - prefix (e.g., \"-7.1.0\")",
			"All constraints must match for the result to be True",
			"Empty constraints list returns True (matches everything)",
		},
		Example: `# Check against compatibility constraints
version_matches("7.7.1", [">=6.4.0", "<=9.0.0"])  # True
version_matches("7.1.0", [">=6.4.0", "-7.1.0"])   # False (excluded)
version_matches("5.0.0", [">=6.4.0"])             # False`,
		SeeAlso: []string{"version_compare", "version_at_least", "get_bazel_compatibility"},
	}
}

func docVersionCompare() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version_compare",
		Category:    "version",
		Signature:   "version_compare(v1, v2)",
		Description: "Compare two version strings",
		Args: []ArgDoc{
			{Name: "v1", Type: "str", Required: true, Description: "First version string"},
			{Name: "v2", Type: "str", Required: true, Description: "Second version string"},
		},
		Behavior: []string{
			"Returns -1 if v1 < v2",
			"Returns 0 if v1 == v2",
			"Returns 1 if v1 > v2",
			"Handles prerelease versions (e.g., 7.0.0-rc1 < 7.0.0)",
		},
		Example: `version_compare("7.0.0", "8.0.0")  # -1
version_compare("8.0.0", "7.0.0")  # 1
version_compare("7.0.0", "7.0.0")  # 0

# Sort versions
versions = ["8.0.0", "7.2.0", "9.0.0"]
sorted_versions = sorted(versions, cmp=lambda a, b: version_compare(a, b))`,
		SeeAlso: []string{"version_matches", "version_at_least", "parse_version"},
	}
}

func docParseVersion() BuiltinDoc {
	return BuiltinDoc{
		Name:        "parse_version",
		Category:    "version",
		Signature:   "parse_version(s)",
		Description: "Parse version string into components",
		Args: []ArgDoc{
			{Name: "s", Type: "str", Required: true, Description: "Version string to parse"},
		},
		Behavior: []string{
			"Returns struct with major, minor, patch, prerelease, and string fields",
			"Supports semver format with optional prerelease suffix",
			"Raises error for invalid version strings",
		},
		Example: `v = parse_version("7.0.0-rc1")
v.major       # 7
v.minor       # 0
v.patch       # 0
v.prerelease  # "rc1"
v.string      # "7.0.0-rc1"

# Check major version
if parse_version(ctx.bazel_version).major >= 8:
    print("Running on Bazel 8+")`,
		SeeAlso: []string{"version_compare", "version_matches"},
	}
}

func docVersionAtLeast() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version_at_least",
		Category:    "version",
		Signature:   "version_at_least(version, minimum)",
		Description: "Check if version >= minimum",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Version string to check"},
			{Name: "minimum", Type: "str", Required: true, Description: "Minimum required version"},
		},
		Behavior: []string{
			"Returns True if version >= minimum",
			"Equivalent to version_compare(version, minimum) >= 0",
			"Useful for prerequisite checks",
		},
		Example: `ctx = get_project_context()
if not version_at_least(ctx.bazel_version, "8.0.0"):
    fail("This recipe requires Bazel 8.0.0 or later")

# Use in recipe definition
recipe(
    name = "my-migration",
    min_bazel_version = "8.0.0",  # Also validates automatically
    ...
)`,
		SeeAlso: []string{"version_in_range", "version_compare", "version_matches"},
	}
}

func docVersionInRange() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version_in_range",
		Category:    "version",
		Signature:   "version_in_range(version, min, max)",
		Description: "Check if min <= version <= max",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Version string to check"},
			{Name: "min", Type: "str", Required: true, Description: "Minimum version (inclusive)"},
			{Name: "max", Type: "str", Required: true, Description: "Maximum version (inclusive)"},
		},
		Behavior: []string{
			"Returns True if min <= version <= max",
			"Both min and max are inclusive bounds",
			"Useful for checking compatibility ranges",
		},
		Example: `ctx = get_project_context()

# Check if running on Bazel 7.x
if version_in_range(ctx.bazel_version, "7.0.0", "7.99.99"):
    print("Running on Bazel 7.x")

# Check supported version range
if not version_in_range(ctx.bazel_version, "7.0.0", "9.0.0"):
    fail("This recipe only supports Bazel 7.x to 9.x")`,
		SeeAlso: []string{"version_at_least", "version_compare", "version_matches"},
	}
}

// =============================================================================
// Project Module (project.*)
// =============================================================================

func docProjectContext() BuiltinDoc {
	return BuiltinDoc{
		Name:        "project.context",
		Category:    "module-project",
		Signature:   "project.context()",
		Description: "Get project context information (memoized per run)",
		Args:        []ArgDoc{},
		Behavior: []string{
			"Detects project root, Bazel version, and module configuration",
			"Results are cached for the duration of the recipe execution",
			"Requires project path to be set (automatic when running recipes)",
		},
		Example: `ctx = project.context()
if ctx.use_bzlmod:
    print("Using bzlmod at", ctx.root)
if ctx.bazel_version:
    print("Bazel version:", ctx.bazel_version)`,
		SeeAlso: []string{"project.deps", "project.compatibility", "version.at_least"},
	}
}

func docProjectDeps() BuiltinDoc {
	return BuiltinDoc{
		Name:        "project.deps",
		Category:    "module-project",
		Signature:   "project.deps()",
		Description: "Get list of bazel_dep() entries from MODULE.bazel",
		Args:        []ArgDoc{},
		Behavior: []string{
			"Parses MODULE.bazel and extracts all bazel_dep entries",
			"Returns empty list if MODULE.bazel doesn't exist",
			"Each entry is a dict with name, version, repo_name, dev_dependency",
		},
		Example: `deps = project.deps()
for dep in deps:
    print(dep["name"], "at version", dep["version"])
    if dep["dev_dependency"]:
        print("  (dev dependency)")`,
		SeeAlso: []string{"project.context", "ensure_bazel_dep"},
	}
}

func docProjectCompatibility() BuiltinDoc {
	return BuiltinDoc{
		Name:        "project.compatibility",
		Category:    "module-project",
		Signature:   "project.compatibility()",
		Description: "Get bazel_compatibility constraints from MODULE.bazel",
		Args:        []ArgDoc{},
		Behavior: []string{
			"Extracts the bazel_compatibility attribute from module()",
			"Returns list of constraint strings (BCR format)",
			"Returns empty list if no compatibility constraints defined",
		},
		Example: `compat = project.compatibility()
# compat might be [">=6.4.0", "<=9.0.0", "-7.1.0"]
ctx = project.context()
if ctx.bazel_version and not version.matches(ctx.bazel_version, compat):
    fail("Project Bazel version doesn't match compatibility constraints")`,
		SeeAlso: []string{"project.context", "version.matches"},
	}
}

// =============================================================================
// Version Module (version.*)
// =============================================================================

func docVersionParse() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version.parse",
		Category:    "module-version",
		Signature:   "version.parse(s)",
		Description: "Parse version string into components",
		Args: []ArgDoc{
			{Name: "s", Type: "str", Required: true, Description: "Version string to parse"},
		},
		Behavior: []string{
			"Returns struct with major, minor, patch, prerelease, and string fields",
			"Supports semver format with optional prerelease suffix",
			"Raises error for invalid version strings",
		},
		Example: `v = version.parse("7.0.0-rc1")
v.major       # 7
v.minor       # 0
v.patch       # 0
v.prerelease  # "rc1"
v.string      # "7.0.0-rc1"

# Check major version
if version.parse(ctx.bazel_version).major >= 8:
    print("Running on Bazel 8+")`,
		SeeAlso: []string{"version.compare", "version.matches"},
	}
}

func docVersionCompareModule() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version.compare",
		Category:    "module-version",
		Signature:   "version.compare(v1, v2)",
		Description: "Compare two version strings",
		Args: []ArgDoc{
			{Name: "v1", Type: "str", Required: true, Description: "First version string"},
			{Name: "v2", Type: "str", Required: true, Description: "Second version string"},
		},
		Behavior: []string{
			"Returns -1 if v1 < v2",
			"Returns 0 if v1 == v2",
			"Returns 1 if v1 > v2",
			"Handles prerelease versions (e.g., 7.0.0-rc1 < 7.0.0)",
		},
		Example: `version.compare("7.0.0", "8.0.0")  # -1
version.compare("8.0.0", "7.0.0")  # 1
version.compare("7.0.0", "7.0.0")  # 0`,
		SeeAlso: []string{"version.matches", "version.at_least", "version.parse"},
	}
}

func docVersionMatchesModule() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version.matches",
		Category:    "module-version",
		Signature:   "version.matches(version, constraints)",
		Description: "Check if version satisfies BCR-style constraints",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Version string to check (e.g., \"7.7.1\")"},
			{Name: "constraints", Type: "list[str]", Required: true, Description: "List of constraint strings"},
		},
		Behavior: []string{
			"Supports BCR constraint operators: >=, <=, >, <, ==, !=",
			"Supports exclusion with - prefix (e.g., \"-7.1.0\")",
			"All constraints must match for the result to be True",
			"Empty constraints list returns True (matches everything)",
		},
		Example: `# Check against compatibility constraints
version.matches("7.7.1", [">=6.4.0", "<=9.0.0"])  # True
version.matches("7.1.0", [">=6.4.0", "-7.1.0"])   # False (excluded)
version.matches("5.0.0", [">=6.4.0"])             # False`,
		SeeAlso: []string{"version.compare", "version.at_least", "project.compatibility"},
	}
}

func docVersionAtLeastModule() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version.at_least",
		Category:    "module-version",
		Signature:   "version.at_least(version, minimum)",
		Description: "Check if version >= minimum",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Version string to check"},
			{Name: "minimum", Type: "str", Required: true, Description: "Minimum required version"},
		},
		Behavior: []string{
			"Returns True if version >= minimum",
			"Equivalent to version.compare(version, minimum) >= 0",
			"Useful for prerequisite checks",
		},
		Example: `ctx = project.context()
if not version.at_least(ctx.bazel_version, "8.0.0"):
    fail("This recipe requires Bazel 8.0.0 or later")`,
		SeeAlso: []string{"version.in_range", "version.compare", "version.matches"},
	}
}

func docVersionInRangeModule() BuiltinDoc {
	return BuiltinDoc{
		Name:        "version.in_range",
		Category:    "module-version",
		Signature:   "version.in_range(version, min, max)",
		Description: "Check if min <= version <= max",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Version string to check"},
			{Name: "min", Type: "str", Required: true, Description: "Minimum version (inclusive)"},
			{Name: "max", Type: "str", Required: true, Description: "Maximum version (inclusive)"},
		},
		Behavior: []string{
			"Returns True if min <= version <= max",
			"Both min and max are inclusive bounds",
			"Useful for checking compatibility ranges",
		},
		Example: `ctx = project.context()

# Check if running on Bazel 7.x
if version.in_range(ctx.bazel_version, "7.0.0", "7.99.99"):
    print("Running on Bazel 7.x")`,
		SeeAlso: []string{"version.at_least", "version.compare", "version.matches"},
	}
}

// =============================================================================
// Filesystem Module (fs.*)
// =============================================================================

func docFsRead() BuiltinDoc {
	return BuiltinDoc{
		Name:        "fs.read",
		Category:    "module-fs",
		Signature:   "fs.read(path, default=None)",
		Description: "Read file contents as string",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Path to the file (relative to project)"},
			{Name: "default", Type: "str", Required: false, Description: "Default value if file doesn't exist"},
		},
		Behavior: []string{
			"Returns file contents as a string",
			"If file doesn't exist and default is provided, returns default",
			"If file doesn't exist and no default, raises an error",
			"Path is sandboxed to project directory",
		},
		Example: `content = fs.read("MODULE.bazel")
version = fs.read(".bazel-version", default="7.0.0")`,
		SeeAlso: []string{"fs.exists", "fs.glob"},
	}
}

func docFsExists() BuiltinDoc {
	return BuiltinDoc{
		Name:        "fs.exists",
		Category:    "module-fs",
		Signature:   "fs.exists(path)",
		Description: "Check if file or directory exists",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Path to check (relative to project)"},
		},
		Behavior: []string{
			"Returns True if the file or directory exists",
			"Returns False otherwise",
			"Path is sandboxed to project directory",
		},
		Example: `if fs.exists("MODULE.bazel"):
    # Running with bzlmod
    pass`,
		SeeAlso: []string{"fs.read", "fs.glob"},
	}
}

func docFsGlob() BuiltinDoc {
	return BuiltinDoc{
		Name:        "fs.glob",
		Category:    "module-fs",
		Signature:   "fs.glob(pattern, exclude=[])",
		Description: "Find files matching a glob pattern",
		Args: []ArgDoc{
			{Name: "pattern", Type: "str", Required: true, Description: "Glob pattern (supports **)"},
			{Name: "exclude", Type: "list", Required: false, Description: "Patterns to exclude"},
		},
		Behavior: []string{
			"Returns list of matching file paths (relative to project)",
			"** matches any number of directories",
			"Hidden directories (starting with .) are skipped",
			"Path is sandboxed to project directory",
		},
		Example: `build_files = fs.glob("**/BUILD")
star_files = fs.glob("**/*.star", exclude=["**/test_*.star"])`,
		SeeAlso: []string{"fs.list_dir", "fs.exists"},
	}
}

func docFsListDir() BuiltinDoc {
	return BuiltinDoc{
		Name:        "fs.list_dir",
		Category:    "module-fs",
		Signature:   "fs.list_dir(path, recursive=False)",
		Description: "List directory contents",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Directory path (relative to project)"},
			{Name: "recursive", Type: "bool", Required: false, Description: "Include subdirectories (default: False)"},
		},
		Behavior: []string{
			"Returns list of file/directory names",
			"If recursive=True, returns relative paths for all files",
			"Path is sandboxed to project directory",
		},
		Example: `entries = fs.list_dir("src")
all_files = fs.list_dir(".", recursive=True)`,
		SeeAlso: []string{"fs.glob", "fs.exists"},
	}
}

func docFsParseBuild() BuiltinDoc {
	return BuiltinDoc{
		Name:        "fs.parse_build",
		Category:    "module-fs",
		Signature:   "fs.parse_build(path)",
		Description: "Parse a BUILD file and return structure",
		Args: []ArgDoc{
			{Name: "path", Type: "str", Required: true, Description: "Path to BUILD file"},
		},
		Behavior: []string{
			"Returns a BuildFile object with rules() and loads() methods",
			"rules(kind=None) returns list of rules, optionally filtered by kind",
			"loads() returns list of load statements",
			"Each rule has: name, kind, line, attr(name) method",
		},
		Example: `bf = fs.parse_build("src/BUILD")
for rule in bf.rules("cc_*"):
    print(rule.name, rule.kind)
for load in bf.loads():
    print(load.path, load.symbols)`,
		SeeAlso: []string{"ensure_load", "fs.glob"},
	}
}

// =============================================================================
// Flags Module (flags.*)
// =============================================================================

func docFlagsExists() BuiltinDoc {
	return BuiltinDoc{
		Name:        "flags.exists",
		Category:    "module-flags",
		Signature:   "flags.exists(name, version)",
		Description: "Check if a flag exists in the given Bazel version",
		Args: []ArgDoc{
			{Name: "name", Type: "str", Required: true, Description: "Flag name (without leading --)"},
			{Name: "version", Type: "str", Required: true, Description: "Bazel version to check"},
		},
		Behavior: []string{
			"Returns True if the flag exists in the given version",
			"Returns False if the flag doesn't exist",
			"Requires flag database to be loaded",
		},
		Example: `if flags.exists("incompatible_enable_cc_toolchain_resolution", "8.0.0"):
    print("Flag exists in Bazel 8.0.0")`,
		SeeAlso: []string{"flags.get", "flags.diff"},
	}
}

func docFlagsGet() BuiltinDoc {
	return BuiltinDoc{
		Name:        "flags.get",
		Category:    "module-flags",
		Signature:   "flags.get(name, version)",
		Description: "Get detailed information about a flag",
		Args: []ArgDoc{
			{Name: "name", Type: "str", Required: true, Description: "Flag name (without leading --)"},
			{Name: "version", Type: "str", Required: true, Description: "Bazel version to check"},
		},
		Behavior: []string{
			"Returns struct with flag details or None if not found",
			"Struct fields: name, short, type, default, description, tags, sources",
			"Also includes: is_deprecated, is_experimental, is_incompatible",
		},
		Example: `flag = flags.get("incompatible_enable_cc_toolchain_resolution", "8.0.0")
if flag:
    print("Default:", flag.default)
    print("Deprecated:", flag.is_deprecated)`,
		SeeAlso: []string{"flags.exists", "flags.diff"},
	}
}

func docFlagsDiff() BuiltinDoc {
	return BuiltinDoc{
		Name:        "flags.diff",
		Category:    "module-flags",
		Signature:   "flags.diff(v1, v2)",
		Description: "Compute differences in flags between two Bazel versions",
		Args: []ArgDoc{
			{Name: "v1", Type: "str", Required: true, Description: "Source version"},
			{Name: "v2", Type: "str", Required: true, Description: "Target version"},
		},
		Behavior: []string{
			"Returns struct with added, removed, and changed flags",
			"Each change includes change_type: default_changed, deprecated, type_changed",
			"Useful for migration planning between Bazel versions",
		},
		Example: `diff = flags.diff("7.0.0", "8.0.0")
print("Added flags:", len(diff.added))
print("Removed flags:", len(diff.removed))
for change in diff.changed:
    if change.change_type == "deprecated":
        print("Newly deprecated:", change.name)`,
		SeeAlso: []string{"flags.get", "flags.incompatible"},
	}
}

func docFlagsIncompatible() BuiltinDoc {
	return BuiltinDoc{
		Name:        "flags.incompatible",
		Category:    "module-flags",
		Signature:   "flags.incompatible(version)",
		Description: "Get all --incompatible_* flags for a Bazel version",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Bazel version"},
		},
		Behavior: []string{
			"Returns list of all flags starting with 'incompatible_'",
			"Each flag is a struct with full flag details",
			"Useful for discovering migration-related flags",
		},
		Example: `incompatible = flags.incompatible("8.0.0")
for flag in incompatible:
    print(flag.name, "default:", flag.default)`,
		SeeAlso: []string{"flags.deprecated", "flags.diff"},
	}
}

func docFlagsDeprecated() BuiltinDoc {
	return BuiltinDoc{
		Name:        "flags.deprecated",
		Category:    "module-flags",
		Signature:   "flags.deprecated(version)",
		Description: "Get all deprecated flags for a Bazel version",
		Args: []ArgDoc{
			{Name: "version", Type: "str", Required: true, Description: "Bazel version"},
		},
		Behavior: []string{
			"Returns list of all flags with 'deprecated' tag",
			"Each flag is a struct with full flag details",
			"Useful for finding flags that should be removed from .bazelrc",
		},
		Example: `deprecated = flags.deprecated("8.0.0")
for flag in deprecated:
    print("Consider removing:", flag.name)`,
		SeeAlso: []string{"flags.incompatible", "flags.diff"},
	}
}
