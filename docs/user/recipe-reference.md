# Recipe Reference

Complete API reference for bazelbump Starlark recipes.

> **Note:** As of v1.x, utility functions are available in namespaced modules (`project.*`, `version.*`, `fs.*`, `flags.*`). The old global function names are deprecated and will be removed in v2.0.

## Table of Contents

- [Core Functions](#core-functions)
  - [recipe()](#recipe)
  - [scan()](#scan)
  - [edit()](#edit)
  - [verify()](#verify)
- [Modules (New API)](#modules-new-api)
  - [project module](#project-module)
  - [version module](#version-module)
  - [fs module](#fs-module)
  - [flags module](#flags-module)
- [Operations](#operations)
  - [BUILD File Operations](#build-file-operations)
  - [MODULE.bazel Operations](#modulebazel-operations)
  - [.bazelrc Operations](#bazelrc-operations)
  - [Generic Operations](#generic-operations)
- [Error Handling](#error-handling-functions)
- [Deprecated Functions](#deprecated-functions-removed-in-v20)

---

## recipe()

Defines a recipe. Must be called exactly once per file.

```starlark
recipe(
    name,                    # string, required
    version,                 # string, required
    description = "",        # string, optional
    min_bazel_version = "",  # string, optional
    max_bazel_version = "",  # string, optional
    steps = [],              # list[Step], required
    on_failure = "rollback", # string, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Unique identifier for the recipe |
| `version` | string | Yes | Semantic version (e.g., "1.0.0") |
| `description` | string | No | Human-readable description |
| `min_bazel_version` | string | No | Minimum compatible Bazel version |
| `max_bazel_version` | string | No | Maximum compatible Bazel version |
| `steps` | list | Yes | Ordered list of steps to execute |
| `on_failure` | string | No | Failure behavior: "rollback", "stop", or "continue" |

**Failure Modes:**

| Mode | Behavior |
|------|----------|
| `rollback` | Revert all changes and stop (default) |
| `stop` | Stop execution, keep changes made so far |
| `continue` | Log error and continue with remaining steps |

**Example:**

```starlark
recipe(
    name = "autoload-migration",
    version = "1.0.0",
    description = "Migrate native rules to explicit loads",
    min_bazel_version = "8.0.0",
    steps = [
        scan(id = "detect", files = ["BUILD"], patterns = [r"cc_binary\s*\("]),
        edit(id = "fix", operations = [...]),
    ],
    on_failure = "rollback",
)
```

---

## Step Functions

### scan()

Create a scan step that detects patterns in files. Returns a **future reference** that can be used in later steps.

```starlark
scan(
    id,                      # string, required
    name = "",               # string, optional
    description = "",        # string, optional
    files = [],              # list[string], required
    patterns = [],           # list[string], required
) -> ScanResult
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Unique step identifier |
| `name` | string | No | Display name for output |
| `description` | string | No | Description of what this step does |
| `files` | list[string] | Yes | File name patterns (globs) to scan |
| `patterns` | list[string] | Yes | Regex patterns to match |

**Returns:** `ScanResult` future reference with these attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `.matches` | list[Match] | All pattern matches found |
| `.files` | list[string] | Files containing matches |
| `.count` | int | Total number of matches |
| `.rules` | list[string] | Unique rule names found |
| `.families` | list[string] | Rule families (cc, python, etc.) |

**Example:**

```starlark
detected = scan(
    id = "find-cc-rules",
    name = "Detect CC rule usage",
    files = ["BUILD", "BUILD.bazel"],
    patterns = [
        r"cc_binary\s*\(",
        r"cc_library\s*\(",
        r"cc_test\s*\(",
    ],
)

# Use in later step
edit(
    id = "add-loads",
    operations = [
        ensure_load_for_detected(detected),
    ],
)
```

---

### edit()

Create an edit step that modifies files.

```starlark
edit(
    id,                      # string, required
    name = "",               # string, optional
    description = "",        # string, optional
    depends_on = [],         # list[string], optional
    operations = [],         # list[Operation], required
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Unique step identifier |
| `name` | string | No | Display name for output |
| `description` | string | No | Description of what this step does |
| `depends_on` | list[string] | No | Step IDs that must complete first |
| `operations` | list[Operation] | Yes | Operations to perform |

**Example:**

```starlark
edit(
    id = "add-deps",
    name = "Add module dependencies",
    depends_on = ["detect"],
    operations = [
        ensure_bazel_dep("rules_cc", "0.1.1"),
        ensure_bazel_dep("rules_python", "1.4.1"),
    ],
)
```

---

### verify()

Create a verification step that runs a shell command.

```starlark
verify(
    id,                      # string, required
    name = "",               # string, optional
    description = "",        # string, optional
    depends_on = [],         # list[string], optional
    command = "",            # string, required
    timeout = "5m",          # string, optional
    allow_failure = False,   # bool, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Unique step identifier |
| `name` | string | No | Display name for output |
| `description` | string | No | Description of what this step does |
| `depends_on` | list[string] | No | Step IDs that must complete first |
| `command` | string | Yes | Shell command to execute |
| `timeout` | string | No | Maximum duration (default: "5m") |
| `allow_failure` | bool | No | If true, don't fail recipe on command failure |

**Timeout format:** Duration string like "5m", "30s", "1h30m"

**Example:**

```starlark
verify(
    id = "build-check",
    name = "Verify build works",
    depends_on = ["add-loads"],
    command = "bazel build //...",
    timeout = "10m",
    allow_failure = True,
)
```

---

## BUILD File Operations

### ensure_load()

Add a load statement to BUILD files if not already present. Only adds the load if the rule is actually used in the file.

```starlark
ensure_load(
    rule,                    # string, required
    load_from,               # string, required
    file = None,             # string, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `rule` | string | Yes | Rule name to load (e.g., "cc_binary") |
| `load_from` | string | Yes | Load path (e.g., "@rules_cc//cc:defs.bzl") |
| `file` | string | No | Specific file to modify (default: all BUILD files) |

**Example:**

```starlark
ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")
ensure_load("cc_library", "@rules_cc//cc:defs.bzl", file = "src/BUILD")
```

---

### ensure_load_for_detected()

Add load statements for all rules found by a scan step. Uses the built-in mapping of rules to load paths.

```starlark
ensure_load_for_detected(
    detected,                # ScanResult, required
    mapping = None,          # dict, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `detected` | ScanResult | Yes | Future reference from scan() |
| `mapping` | dict | No | Override default rule-to-load mapping |

**Built-in mappings:**

| Rule | Load Path |
|------|-----------|
| `cc_binary`, `cc_library`, `cc_test` | `@rules_cc//cc:defs.bzl` |
| `java_binary`, `java_library`, `java_test` | `@rules_java//java:defs.bzl` |
| `py_binary`, `py_library`, `py_test` | `@rules_python//python:defs.bzl` |
| `sh_binary` | `@rules_shell//shell:sh_binary.bzl` |
| `sh_test` | `@rules_shell//shell:sh_test.bzl` |
| `sh_library` | `@rules_shell//shell:sh_library.bzl` |

**Example:**

```starlark
detected = scan(
    id = "find",
    files = ["BUILD", "BUILD.bazel"],
    patterns = [r"cc_(binary|library|test)\s*\("],
)

edit(
    id = "loads",
    operations = [
        ensure_load_for_detected(detected),
        # Or with custom mapping:
        ensure_load_for_detected(detected, mapping = {
            "cc_binary": "@my_rules//cc:defs.bzl",
        }),
    ],
)
```

---

### remove_load()

Remove a rule from load statements.

```starlark
remove_load(
    rule,                    # string, required
    file = None,             # string, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `rule` | string | Yes | Rule name to remove |
| `file` | string | No | Specific file (default: all BUILD files) |

**Behavior:**

- If the rule is the only symbol in the load statement, removes the entire load
- Otherwise, removes just that symbol from the load

**Example:**

```starlark
remove_load("deprecated_rule")
remove_load("old_rule", file = "legacy/BUILD")
```

---

## MODULE.bazel Operations

### ensure_bazel_dep()

Add a bazel_dep to MODULE.bazel if not already present.

```starlark
ensure_bazel_dep(
    name,                    # string, required
    version,                 # string, required
    dev_dependency = False,  # bool, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Dependency name (e.g., "rules_cc") |
| `version` | string | Yes | Version (e.g., "0.1.1") |
| `dev_dependency` | bool | No | Mark as dev dependency |

**Example:**

```starlark
ensure_bazel_dep("rules_cc", "0.1.1")
ensure_bazel_dep("rules_testing", "0.6.0", dev_dependency = True)
```

---

### update_bazel_dep()

Update the version of an existing bazel_dep.

```starlark
update_bazel_dep(
    name,                    # string, required
    version,                 # string, required
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Dependency name |
| `version` | string | Yes | New version |

**Example:**

```starlark
update_bazel_dep("rules_cc", "0.2.0")
```

---

### remove_bazel_dep()

Remove a bazel_dep from MODULE.bazel.

```starlark
remove_bazel_dep(
    name,                    # string, required
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Dependency name to remove |

**Example:**

```starlark
remove_bazel_dep("deprecated_rules")
```

---

## .bazelrc Operations

### remove_flag()

Remove a flag from .bazelrc.

```starlark
remove_flag(
    flag,                    # string, required
    file = ".bazelrc",       # string, optional
    config = None,           # string, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `flag` | string | Yes | Flag name (without `--`) |
| `file` | string | No | File to modify (default: ".bazelrc") |
| `config` | string | No | Only remove from specific config section |

**Example:**

```starlark
remove_flag("experimental_enable_bzlmod")
remove_flag("incompatible_strict_action_env", config = "build")
remove_flag("jobs", file = ".bazelrc.user")
```

---

### replace_flag()

Replace a flag in .bazelrc.

```starlark
replace_flag(
    old,                     # string, required
    new,                     # string, required
    file = ".bazelrc",       # string, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `old` | string | Yes | Flag text to replace (include `--` if present) |
| `new` | string | Yes | Replacement text (empty to just remove) |
| `file` | string | No | File to modify (default: ".bazelrc") |

**Example:**

```starlark
replace_flag(
    old = "--incompatible_enable_cc_toolchain_resolution=false",
    new = "--incompatible_enable_cc_toolchain_resolution=true",
)

# Remove a flag entirely
replace_flag(
    old = "--experimental_old_flag",
    new = "",
)
```

---

### ensure_flag()

Add a flag to .bazelrc if not already present.

```starlark
ensure_flag(
    flag,                    # string, required
    value = None,            # string, optional
    config = "build",        # string, optional
    file = ".bazelrc",       # string, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `flag` | string | Yes | Flag name (without `--`) |
| `value` | string | No | Flag value (omit for boolean flags) |
| `config` | string | No | Config section (default: "build") |
| `file` | string | No | File to modify (default: ".bazelrc") |

**Example:**

```starlark
ensure_flag("keep_going")                           # build --keep_going
ensure_flag("jobs", value = "auto")                 # build --jobs=auto
ensure_flag("test_output", value = "errors", config = "test")  # test --test_output=errors
```

---

## Generic Operations

### replace_text()

Replace text in files.

```starlark
replace_text(
    old,                     # string, required
    new,                     # string, required
    files = [],              # list[string], required
    regex = False,           # bool, optional
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `old` | string | Yes | Text or pattern to find |
| `new` | string | Yes | Replacement text |
| `files` | list[string] | Yes | File patterns to process |
| `regex` | bool | No | Treat `old` as regex pattern |

**Example:**

```starlark
# Simple text replacement
replace_text(
    old = "old_function",
    new = "new_function",
    files = ["*.bzl"],
)

# Regex replacement
replace_text(
    old = r'version\s*=\s*"[^"]+"',
    new = 'version = "2.0.0"',
    files = ["MODULE.bazel"],
    regex = True,
)
```

---

### copy_file()

Copy a file.

```starlark
copy_file(
    src,                     # string, required
    dst,                     # string, required
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `src` | string | Yes | Source file path |
| `dst` | string | Yes | Destination file path |

**Example:**

```starlark
copy_file(
    src = "templates/BUILD.template",
    dst = "new_package/BUILD",
)
```

---

### delete_file()

Delete a file.

```starlark
delete_file(
    path,                    # string, required
)
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | File path or glob pattern |

**Example:**

```starlark
delete_file("WORKSPACE.bzlmod")
delete_file("*.bak")  # Glob pattern
```

---

## Modules (New API)

The new namespaced module API groups related functions together for better discoverability and reduced namespace pollution. These are the recommended APIs going forward.

### project module

Functions for accessing project context information.

#### project.context()

Get project context information (memoized per run).

```starlark
project.context() -> struct
```

**Returns:** A struct with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `root` | string | Project root directory |
| `bazel_version` | string | Bazel version from .bazelversion (empty if not found) |
| `use_bzlmod` | bool | True if MODULE.bazel exists |
| `use_workspace` | bool | True if WORKSPACE exists |
| `module_name` | string | Module name from MODULE.bazel |
| `module_version` | string | Module version from MODULE.bazel |

**Example:**

```starlark
ctx = project.context()
if ctx.use_bzlmod:
    print("Using bzlmod at", ctx.root)
if ctx.bazel_version:
    print("Bazel version:", ctx.bazel_version)
```

---

#### project.deps()

Get list of bazel_dep() entries from MODULE.bazel.

```starlark
project.deps() -> list[dict]
```

**Returns:** List of dicts, each with:

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Module name |
| `version` | string | Module version |
| `repo_name` | string | Optional repo_name override (empty if not set) |
| `dev_dependency` | bool | True if this is a dev dependency |

**Example:**

```starlark
deps = project.deps()
for dep in deps:
    print(dep["name"], "at version", dep["version"])
    if dep["dev_dependency"]:
        print("  (dev dependency)")
```

---

#### project.compatibility()

Get bazel_compatibility constraints from MODULE.bazel.

```starlark
project.compatibility() -> list[string]
```

**Returns:** List of constraint strings (BCR format), e.g., `[">=6.4.0", "<=9.0.0", "-7.1.0"]`

**Example:**

```starlark
compat = project.compatibility()
ctx = project.context()
if ctx.bazel_version and not version.matches(ctx.bazel_version, compat):
    fail("Project Bazel version doesn't match compatibility constraints")
```

---

### version module

Functions for version parsing and comparison.

#### version.parse()

Parse a version string into its components.

```starlark
version.parse(s) -> struct
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `s` | string | Yes | Version string to parse |

**Returns:** A struct with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `major` | int | Major version number |
| `minor` | int | Minor version number |
| `patch` | int | Patch version number |
| `prerelease` | string | Prerelease suffix (empty if stable) |
| `string` | string | Original version string |

**Example:**

```starlark
v = version.parse("7.0.0-rc1")
v.major       # 7
v.minor       # 0
v.patch       # 0
v.prerelease  # "rc1"

# Check major version
ctx = project.context()
if version.parse(ctx.bazel_version).major >= 8:
    print("Running on Bazel 8+")
```

---

#### version.compare()

Compare two version strings.

```starlark
version.compare(v1, v2) -> int
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `v1` | string | Yes | First version string |
| `v2` | string | Yes | Second version string |

**Returns:**

| Value | Meaning |
|-------|---------|
| `-1` | v1 < v2 |
| `0` | v1 == v2 |
| `1` | v1 > v2 |

**Example:**

```starlark
version.compare("7.0.0", "8.0.0")  # -1
version.compare("8.0.0", "7.0.0")  # 1
version.compare("7.0.0", "7.0.0")  # 0
```

---

#### version.matches()

Check if a version satisfies BCR-style constraints.

```starlark
version.matches(version, constraints) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Version string to check |
| `constraints` | list[string] | Yes | List of constraint strings |

**Constraint operators:**

| Operator | Description | Example |
|----------|-------------|---------|
| `>=` | Greater than or equal | `>=6.4.0` |
| `<=` | Less than or equal | `<=9.0.0` |
| `>` | Greater than | `>7.0.0` |
| `<` | Less than | `<8.0.0` |
| `==` | Exactly equal | `==7.0.0` |
| `!=` | Not equal | `!=7.1.0` |
| `-` | Exclude version | `-7.1.0` |

**Example:**

```starlark
version.matches("7.7.1", [">=6.4.0", "<=9.0.0"])  # True
version.matches("7.1.0", [">=6.4.0", "-7.1.0"])   # False (excluded)
version.matches("5.0.0", [">=6.4.0"])             # False
```

---

#### version.at_least()

Check if version >= minimum.

```starlark
version.at_least(version, minimum) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Version string to check |
| `minimum` | string | Yes | Minimum required version |

**Example:**

```starlark
ctx = project.context()
if not version.at_least(ctx.bazel_version, "8.0.0"):
    fail("This recipe requires Bazel 8.0.0 or later")
```

---

#### version.in_range()

Check if min <= version <= max.

```starlark
version.in_range(version, min, max) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Version string to check |
| `min` | string | Yes | Minimum version (inclusive) |
| `max` | string | Yes | Maximum version (inclusive) |

**Example:**

```starlark
ctx = project.context()

# Check if running on Bazel 7.x
if version.in_range(ctx.bazel_version, "7.0.0", "7.99.99"):
    print("Running on Bazel 7.x")
```

---

### fs module

Functions for filesystem operations (sandboxed to project directory).

#### fs.read()

Read file contents as string.

```starlark
fs.read(path, default=None) -> string
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | Path to the file (relative to project) |
| `default` | any | No | Default value if file doesn't exist |

**Example:**

```starlark
content = fs.read("MODULE.bazel")
version = fs.read(".bazel-version", default="7.0.0")
```

---

#### fs.exists()

Check if file or directory exists.

```starlark
fs.exists(path) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | Path to check (relative to project) |

**Example:**

```starlark
if fs.exists("MODULE.bazel"):
    # Running with bzlmod
    pass
```

---

#### fs.glob()

Find files matching a glob pattern.

```starlark
fs.glob(pattern, exclude=[]) -> list[string]
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pattern` | string | Yes | Glob pattern (supports `**`) |
| `exclude` | list[string] | No | Patterns to exclude |

**Example:**

```starlark
build_files = fs.glob("**/BUILD")
star_files = fs.glob("**/*.star", exclude=["**/test_*.star"])
```

---

#### fs.list_dir()

List directory contents.

```starlark
fs.list_dir(path, recursive=False) -> list[string]
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | Directory path (relative to project) |
| `recursive` | bool | No | Include subdirectories (default: False) |

**Example:**

```starlark
entries = fs.list_dir("src")
all_files = fs.list_dir(".", recursive=True)
```

---

#### fs.parse_build()

Parse a BUILD file and return structure.

```starlark
fs.parse_build(path) -> BuildFile
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | Path to BUILD file |

**Returns:** A BuildFile object with these methods:

| Method | Returns | Description |
|--------|---------|-------------|
| `rules(kind=None)` | list[Rule] | Rules, optionally filtered by kind (supports `cc_*` wildcards) |
| `loads()` | list[Load] | Load statements |

**Example:**

```starlark
bf = fs.parse_build("src/BUILD")
for rule in bf.rules("cc_*"):
    print(rule.name, rule.kind)
for load in bf.loads():
    print(load.path, load.symbols)
```

---

### flags module

Functions for querying the Bazel flag database. Requires flag database to be loaded (via `--flagdb-path` or equivalent).

#### flags.exists()

Check if a flag exists in the given Bazel version.

```starlark
flags.exists(name, version) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Flag name (without leading `--`) |
| `version` | string | Yes | Bazel version to check |

**Example:**

```starlark
if flags.exists("incompatible_enable_cc_toolchain_resolution", "8.0.0"):
    print("Flag exists in Bazel 8.0.0")
```

---

#### flags.get()

Get detailed information about a flag.

```starlark
flags.get(name, version) -> struct | None
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Flag name (without leading `--`) |
| `version` | string | Yes | Bazel version to check |

**Returns:** A struct with flag details or `None` if not found:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Flag name |
| `short` | string | Short description |
| `type` | string | Flag type |
| `default` | string | Default value |
| `description` | string | Full description |
| `tags` | list[string] | Tags (e.g., "deprecated", "experimental") |
| `sources` | list[string] | Commands where flag is available |
| `is_deprecated` | bool | True if flag is deprecated |
| `is_experimental` | bool | True if flag is experimental |
| `is_incompatible` | bool | True if flag is an incompatible change flag |

**Example:**

```starlark
flag = flags.get("incompatible_enable_cc_toolchain_resolution", "8.0.0")
if flag:
    print("Default:", flag.default)
    print("Deprecated:", flag.is_deprecated)
```

---

#### flags.diff()

Compute differences in flags between two Bazel versions.

```starlark
flags.diff(v1, v2) -> struct
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `v1` | string | Yes | Source version |
| `v2` | string | Yes | Target version |

**Returns:** A struct with:

| Field | Type | Description |
|-------|------|-------------|
| `from_version` | string | Source version |
| `to_version` | string | Target version |
| `added` | list[Flag] | Flags added in v2 |
| `removed` | list[Flag] | Flags removed in v2 |
| `changed` | list[FlagChange] | Flags that changed |

Each `FlagChange` has `name`, `from`, `to`, and `change_type` (e.g., "default_changed", "deprecated").

**Example:**

```starlark
diff = flags.diff("7.0.0", "8.0.0")
print("Added flags:", len(diff.added))
print("Removed flags:", len(diff.removed))
for change in diff.changed:
    if change.change_type == "deprecated":
        print("Newly deprecated:", change.name)
```

---

#### flags.incompatible()

Get all `--incompatible_*` flags for a Bazel version.

```starlark
flags.incompatible(version) -> list[Flag]
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Bazel version |

**Example:**

```starlark
incompatible = flags.incompatible("8.0.0")
for flag in incompatible:
    print(flag.name, "default:", flag.default)
```

---

#### flags.deprecated()

Get all deprecated flags for a Bazel version.

```starlark
flags.deprecated(version) -> list[Flag]
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Bazel version |

**Example:**

```starlark
deprecated = flags.deprecated("8.0.0")
for flag in deprecated:
    print("Consider removing:", flag.name)
```

---

## Deprecated Functions (Removed in v2.0)

> **Warning:** The following global functions are deprecated. Use the namespaced module equivalents instead.

| Deprecated Function | New Module Function |
|--------------------|--------------------|
| `get_project_context()` | `project.context()` |
| `get_bazel_deps()` | `project.deps()` |
| `get_bazel_compatibility()` | `project.compatibility()` |
| `parse_version(s)` | `version.parse(s)` |
| `version_compare(v1, v2)` | `version.compare(v1, v2)` |
| `version_matches(v, cs)` | `version.matches(v, cs)` |
| `version_at_least(v, min)` | `version.at_least(v, min)` |
| `version_in_range(v, min, max)` | `version.in_range(v, min, max)` |
| `read_file(path)` | `fs.read(path)` |
| `file_exists(path)` | `fs.exists(path)` |
| `glob(pattern)` | `fs.glob(pattern)` |
| `list_dir(path)` | `fs.list_dir(path)` |
| `parse_build_file(path)` | `fs.parse_build(path)` |

When using deprecated functions, you'll see warnings like:
```
warning: read_file is deprecated, use fs.read instead
```

### Migration Example

**Before (deprecated):**
```starlark
ctx = get_project_context()
if version_at_least(ctx.bazel_version, "8.0.0"):
    content = read_file("MODULE.bazel")
    deps = get_bazel_deps()
```

**After (new API):**
```starlark
ctx = project.context()
if version.at_least(ctx.bazel_version, "8.0.0"):
    content = fs.read("MODULE.bazel")
    deps = project.deps()
```

---

## Project Context Functions

> **Deprecated:** Use `project.*` module instead. See [Deprecated Functions](#deprecated-functions-removed-in-v20).

These functions provide information about the project being migrated.

### get_project_context()

Get project context information (memoized per run).

```starlark
get_project_context() -> struct
```

**Returns:** A struct with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `root` | string | Project root directory |
| `bazel_version` | string | Bazel version from .bazelversion (empty if not found) |
| `use_bzlmod` | bool | True if MODULE.bazel exists |
| `use_workspace` | bool | True if WORKSPACE exists |
| `module_name` | string | Module name from MODULE.bazel |
| `module_version` | string | Module version from MODULE.bazel |

**Example:**

```starlark
ctx = get_project_context()
if ctx.use_bzlmod:
    print("Using bzlmod at", ctx.root)
if ctx.bazel_version:
    print("Bazel version:", ctx.bazel_version)
```

---

### get_bazel_deps()

Get list of bazel_dep() entries from MODULE.bazel.

```starlark
get_bazel_deps() -> list[dict]
```

**Returns:** List of dicts, each with:

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Module name |
| `version` | string | Module version |
| `repo_name` | string | Optional repo_name override (empty if not set) |
| `dev_dependency` | bool | True if this is a dev dependency |

**Example:**

```starlark
deps = get_bazel_deps()
for dep in deps:
    print(dep["name"], "at version", dep["version"])
    if dep["dev_dependency"]:
        print("  (dev dependency)")
```

---

### get_bazel_compatibility()

Get bazel_compatibility constraints from MODULE.bazel.

```starlark
get_bazel_compatibility() -> list[string]
```

**Returns:** List of constraint strings (BCR format), e.g., `[">=6.4.0", "<=9.0.0", "-7.1.0"]`

**Example:**

```starlark
compat = get_bazel_compatibility()
ctx = get_project_context()
if ctx.bazel_version and not version_matches(ctx.bazel_version, compat):
    fail("Project Bazel version doesn't match compatibility constraints")
```

---

## Version Utility Functions

> **Deprecated:** Use `version.*` module instead. See [Deprecated Functions](#deprecated-functions-removed-in-v20).

These functions help with version comparisons and constraint checking.

### version_matches()

Check if a version satisfies BCR-style constraints.

```starlark
version_matches(
    version,                 # string, required
    constraints,             # list[string], required
) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Version string to check (e.g., "7.7.1") |
| `constraints` | list[string] | Yes | List of constraint strings |

**Constraint operators:**

| Operator | Description | Example |
|----------|-------------|---------|
| `>=` | Greater than or equal | `>=6.4.0` |
| `<=` | Less than or equal | `<=9.0.0` |
| `>` | Greater than | `>7.0.0` |
| `<` | Less than | `<8.0.0` |
| `==` | Exactly equal | `==7.0.0` |
| `!=` | Not equal | `!=7.1.0` |
| `-` | Exclude version | `-7.1.0` |

**Example:**

```starlark
version_matches("7.7.1", [">=6.4.0", "<=9.0.0"])  # True
version_matches("7.1.0", [">=6.4.0", "-7.1.0"])   # False (excluded)
version_matches("5.0.0", [">=6.4.0"])             # False
```

---

### version_compare()

Compare two version strings.

```starlark
version_compare(
    v1,                      # string, required
    v2,                      # string, required
) -> int
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `v1` | string | Yes | First version string |
| `v2` | string | Yes | Second version string |

**Returns:**

| Value | Meaning |
|-------|---------|
| `-1` | v1 < v2 |
| `0` | v1 == v2 |
| `1` | v1 > v2 |

**Example:**

```starlark
version_compare("7.0.0", "8.0.0")  # -1
version_compare("8.0.0", "7.0.0")  # 1
version_compare("7.0.0", "7.0.0")  # 0
```

---

### parse_version()

Parse a version string into its components.

```starlark
parse_version(
    s,                       # string, required
) -> struct
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `s` | string | Yes | Version string to parse |

**Returns:** A struct with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `major` | int | Major version number |
| `minor` | int | Minor version number |
| `patch` | int | Patch version number |
| `prerelease` | string | Prerelease suffix (empty if stable) |
| `string` | string | Original version string |

**Example:**

```starlark
v = parse_version("7.0.0-rc1")
v.major       # 7
v.minor       # 0
v.patch       # 0
v.prerelease  # "rc1"
v.string      # "7.0.0-rc1"

# Check major version
ctx = get_project_context()
if parse_version(ctx.bazel_version).major >= 8:
    print("Running on Bazel 8+")
```

---

### version_at_least()

Check if version >= minimum.

```starlark
version_at_least(
    version,                 # string, required
    minimum,                 # string, required
) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Version string to check |
| `minimum` | string | Yes | Minimum required version |

**Example:**

```starlark
ctx = get_project_context()
if not version_at_least(ctx.bazel_version, "8.0.0"):
    fail("This recipe requires Bazel 8.0.0 or later")
```

---

### version_in_range()

Check if min <= version <= max.

```starlark
version_in_range(
    version,                 # string, required
    min,                     # string, required
    max,                     # string, required
) -> bool
```

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `version` | string | Yes | Version string to check |
| `min` | string | Yes | Minimum version (inclusive) |
| `max` | string | Yes | Maximum version (inclusive) |

**Example:**

```starlark
ctx = get_project_context()

# Check if running on Bazel 7.x
if version_in_range(ctx.bazel_version, "7.0.0", "7.99.99"):
    print("Running on Bazel 7.x")

# Check supported version range
if not version_in_range(ctx.bazel_version, "7.0.0", "9.0.0"):
    fail("This recipe only supports Bazel 7.x to 9.x")
```

---

## Error Handling Functions

### fail()

Stop execution immediately with an error. Triggers rollback if configured.

```starlark
fail(msg)  # string, required
```

**Example:**

```starlark
fail("MODULE.bazel not found - this recipe requires bzlmod")
```

---

### warn()

Log a warning message but continue execution.

```starlark
warn(msg)  # string, required
```

**Example:**

```starlark
warn("Flag already removed, skipping")
```

---

### skip()

Skip the current step and continue with the recipe.

```starlark
skip(msg)  # string, required
```

**Example:**

```starlark
skip("No native rules detected, nothing to migrate")
```

---

## Load Statement

Import symbols from other Starlark files.

```starlark
load(path, symbol1, symbol2, ...)
```

**Path formats:**

| Format | Resolution |
|--------|------------|
| `"./path.star"` | Relative to current file |
| `"../path.star"` | Relative to parent directory |
| `"//path.star"` | Relative to recipe search roots |
| `"@stdlib//path.star"` | Built-in standard library (future) |

**Example:**

```starlark
load("./lib/helpers.star", "migration_ops")
load("//common/patterns.star", "CC_PATTERNS", "PY_PATTERNS")
```

---

## Built-in Constants

These constants are available in the standard library (future feature):

```starlark
# @stdlib//rules.star

NATIVE_RULE_PATTERNS = {
    "cc": [r"cc_(binary|library|test|import|shared_library)\s*\("],
    "python": [r"py_(binary|library|test)\s*\("],
    "java": [r"java_(binary|library|test|import|plugin)\s*\("],
    "shell": [r"sh_(binary|test|library)\s*\("],
}

RULE_LOAD_PATHS = {
    "cc_binary": "@rules_cc//cc:defs.bzl",
    "cc_library": "@rules_cc//cc:defs.bzl",
    "py_binary": "@rules_python//python:defs.bzl",
    # ...
}

RULE_DEP_VERSIONS = {
    "rules_cc": "0.1.1",
    "rules_python": "1.4.1",
    "rules_java": "8.6.3",
    "rules_shell": "0.4.1",
}
```

---

## Types

### ScanResult

Future reference returned by `scan()`. Attributes are resolved during execution.

| Attribute | Type | Description |
|-----------|------|-------------|
| `matches` | list[Match] | All pattern matches |
| `files` | list[string] | Files with matches |
| `count` | int | Number of matches |
| `rules` | list[string] | Unique rule names |
| `families` | list[string] | Rule families |

### Match

A single pattern match in a file.

| Field | Type | Description |
|-------|------|-------------|
| `file` | string | Path to file |
| `line` | int | Line number (1-based) |
| `text` | string | Matched text |
| `rule` | string | Rule name if applicable |
| `family` | string | Rule family if applicable |

### FileChange

A modification to a file (returned in results).

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | File path |
| `description` | string | What changed |

---

## See Also

- [Recipes Overview](recipes.md) - Using recipes
- [Authoring Recipes](authoring-recipes.md) - Writing custom recipes
- [Starlark Recipe Spec](../design/starlark-recipes-spec.md) - Design specification
