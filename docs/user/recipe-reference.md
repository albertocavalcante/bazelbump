# Recipe Reference

Complete API reference for bazelbump Starlark recipes.

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
