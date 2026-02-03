# Starlark Recipe System Specification

**Status:** Draft v2
**Author:** bazelbump contributors
**Created:** 2025-02-03
**Updated:** 2025-02-03 (incorporated learnings from Tilt, Kurtosis, Copybara, et al.)

## Summary

Enable users to author bazelbump recipes in Starlark, providing a programmable, composable, and Bazel-native configuration experience. Recipes define multi-step migrations with conditionals, loops, reusable functions, and safe sandboxed execution.

## Motivation

### Current State

Recipes are hardcoded in Go. Users cannot:
- Create custom recipes without modifying bazelbump source
- Share recipes between projects
- Compose recipes from reusable parts
- Test recipes in isolation

### Why Starlark?

| Requirement | Starlark | YAML | TOML |
|-------------|----------|------|------|
| Familiar to Bazel users | ✅ | ❌ | ❌ |
| Programmable (loops, conditionals) | ✅ | ❌ | ❌ |
| Composable (functions, load()) | ✅ | ❌ | ❌ |
| Safe sandboxed execution | ✅ | ✅ | ✅ |
| Ecosystem consistency | ✅ | ❌ | ❌ |

### Prior Art

This design is informed by analysis of:
- **Tilt** - Register-then-execute model, extension system
- **Kurtosis** - Multi-phase execution, future references
- **Copybara** - Workflow-centric transformations
- **Skycfg** - Type-safe protobuf configuration
- **Isopod** - Context injection, testing support

See `docs/research/starlark-tools-analysis.md` for detailed analysis.

---

## Specification

### 1. Execution Model

bazelbump uses a **three-phase execution model** (inspired by Kurtosis):

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Interpretation │ ──▶ │   Validation    │ ──▶ │   Execution     │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │                       │
   Parse Starlark          Check deps exist        Run operations
   Build step graph        Validate patterns       Track changes
   Resolve load()          Verify file access      Support rollback
   Return futures          Report all errors       Apply or dry-run
```

**Benefits:**
- Catch all errors before modifying any files
- Better error messages with full context
- Clean dry-run without partial execution
- Enable future references for declarative composition

### 2. Recipe File Format

Recipe files use the `.star` extension and are valid Starlark.

```starlark
# my-recipe.star

recipe(
    name = "my-migration",
    version = "1.0.0",
    description = "Migrate foo to bar",
    min_bazel_version = "8.0.0",
    max_bazel_version = "10.0.0",
    steps = [
        # Steps defined here...
    ],
)
```

A file may define exactly one recipe via the `recipe()` function.

### 3. Built-in Functions

#### 3.1 `recipe()`

Defines a recipe. Must be called exactly once per file.

```starlark
recipe(
    name,                    # string, required: unique identifier
    version = "1.0.0",       # string, required: semver
    description = "",        # string, optional
    min_bazel_version = "",  # string, optional: minimum Bazel version
    max_bazel_version = "",  # string, optional: maximum Bazel version
    steps = [],              # list[Step], required: ordered steps
    on_failure = "rollback", # string, optional: "rollback" | "stop" | "continue"
)
```

#### 3.2 `scan()`

Creates a scan step that detects patterns in the codebase. Returns a **future reference** to scan results.

```starlark
detected = scan(
    id,                      # string, required: unique step ID
    name = "",               # string, optional: display name
    description = "",        # string, optional
    files = [],              # list[string], required: glob patterns
    patterns = [],           # list[string], required: regex patterns
)

# detected is a future reference - usable in later steps
# detected.matches  -> list of Match objects
# detected.files    -> list of files with matches
# detected.count    -> number of matches
```

#### 3.3 `edit()`

Creates an edit step that modifies files.

```starlark
edit(
    id,                      # string, required: unique step ID
    name = "",               # string, optional: display name
    description = "",        # string, optional
    depends_on = [],         # list[string], optional: step IDs (auto-inferred from futures)
    operations = [],         # list[Operation], required
)
```

#### 3.4 `verify()`

Creates a verification step that runs a command.

```starlark
verify(
    id,                      # string, required: unique step ID
    name = "",               # string, optional: display name
    description = "",        # string, optional
    depends_on = [],         # list[string], optional: step IDs
    command = "",            # string, required: shell command
    timeout = "5m",          # string, optional: timeout duration
    allow_failure = False,   # bool, optional
)
```

### 4. Operation Functions

Operations follow **verb_noun** naming for actions and **ensure_** prefix for idempotent operations.

#### 4.1 BUILD File Operations

```starlark
# Add load statement (idempotent)
ensure_load(
    rule,                    # string: rule name (e.g., "cc_binary")
    load_from,               # string: load path (e.g., "@rules_cc//cc:defs.bzl")
    file = None,             # string, optional: specific file (default: all matching)
)

# Add load for each detected rule (uses future reference)
ensure_load_for_detected(
    detected,                # ScanResult: future reference from scan()
    mapping = None,          # dict, optional: rule -> load_path (default: builtin mapping)
)

# Remove load statement
remove_load(
    rule,                    # string: rule name to remove
    file = None,             # string, optional: specific file
)
```

#### 4.2 MODULE.bazel Operations

```starlark
# Add bazel_dep (idempotent)
ensure_bazel_dep(
    name,                    # string: dependency name
    version,                 # string: version
    dev_dependency = False,  # bool, optional
)

# Update bazel_dep version
update_bazel_dep(
    name,                    # string: dependency name
    version,                 # string: new version
)

# Remove bazel_dep
remove_bazel_dep(
    name,                    # string: dependency name
)
```

#### 4.3 .bazelrc Operations

```starlark
# Remove a flag
remove_flag(
    flag,                    # string: flag name (e.g., "incompatible_foo")
    file = ".bazelrc",       # string, optional
    config = None,           # string, optional: specific config section
)

# Replace a flag
replace_flag(
    old,                     # string: flag to replace
    new,                     # string: replacement flag (empty to just remove)
    file = ".bazelrc",       # string, optional
)

# Add a flag (idempotent)
ensure_flag(
    flag,                    # string: flag to add
    value = None,            # string, optional: flag value
    config = "build",        # string, optional: config section
    file = ".bazelrc",       # string, optional
)
```

#### 4.4 Generic Operations

```starlark
# Text replacement in files
replace_text(
    old,                     # string: text to find
    new,                     # string: replacement text
    files = [],              # list[string]: file globs
    regex = False,           # bool, optional: treat old as regex
)

# Copy file
copy_file(
    src,                     # string: source path
    dst,                     # string: destination path
)

# Delete file
delete_file(
    path,                    # string: file path or glob
)
```

### 5. Error Handling

Three-tier error handling (inspired by Copybara/Kurtosis):

```starlark
# Hard failure - stop execution, trigger rollback
fail("MODULE.bazel not found - cannot continue")

# Warning - log and continue
warn("Flag already removed, skipping")

# Skip - skip current step, continue recipe
skip("No native rules detected, nothing to migrate")
```

In verify steps:

```starlark
verify(
    id = "check-build",
    command = "bazel build //...",
    # Specific exit codes that are acceptable
    acceptable_codes = [0],  # default: [0]
    # Or skip code checking entirely
    skip_code_check = False,
)
```

### 6. Context Object

Operations can access a `ctx` object with runtime information:

```starlark
def dynamic_ops(ctx):
    # Project information
    ctx.project_path      # string: root directory
    ctx.bazel_version     # string: detected Bazel version (may be None)

    # Configuration
    ctx.dry_run           # bool: true if --dry-run
    ctx.verbose           # bool: true if --verbose

    # Logging
    ctx.log.info("Processing %d files", count)
    ctx.log.warn("Deprecated pattern found")
    ctx.log.debug("Details: %s", details)

    # Step outputs (for dependencies not using futures)
    ctx.outputs["step-id"]

    return [...]
```

### 7. Future References

Scan results return **future references** that are resolved during execution (inspired by Kurtosis):

```starlark
# During interpretation, this returns a future, not actual results
detected = scan(
    id = "find-rules",
    files = ["**/BUILD", "**/BUILD.bazel"],
    patterns = [r"cc_(binary|library|test)\s*\("],
)

# Future can be used in operations - resolved during execution
edit(
    id = "add-loads",
    operations = [
        # This works! Future is resolved when step executes
        ensure_load_for_detected(detected),

        # Conditional based on future (evaluated at execution)
        ensure_bazel_dep("rules_cc", "0.1.1") if detected.count > 0 else None,
    ],
)
```

**Future Reference Attributes:**

| Attribute | Type | Description |
|-----------|------|-------------|
| `.matches` | list[Match] | All pattern matches |
| `.files` | list[string] | Files containing matches |
| `.count` | int | Number of matches |
| `.rules` | list[string] | Unique rule names found |
| `.families` | set[string] | Rule families (cc, python, etc.) |

### 8. Load/Import System

#### 8.1 Load Statement Syntax

```starlark
# Relative to current file
load("./lib/helpers.star", "my_helper")
load("../common/rules.star", "PATTERNS")

# Relative to recipe search roots (// prefix)
load("//lib/cc.star", "cc_ops")

# Built-in standard library
load("@stdlib//rules.star", "NATIVE_RULE_MAPPING")
load("@stdlib//migrations/cc.star", "cc_autoload_ops")

# Official extensions (curated repository)
load("ext://android.star", "android_migration")
load("ext://go.star", "go_migration")
```

#### 8.2 Load Path Resolution

1. `./path` - Relative to current file's directory
2. `//path` - Relative to recipe search roots (in order)
3. `@stdlib//path` - Built-in standard library (embedded in binary)
4. `ext://name` - Official extension repository

#### 8.3 Cycle Detection

Circular loads are detected during interpretation and result in an error:

```
error: circular load detected:
  recipes/a.star loads //lib/b.star
  //lib/b.star loads //lib/c.star
  //lib/c.star loads recipes/a.star
```

### 9. Recipe Discovery

Recipes are discovered from multiple locations (in priority order):

| Priority | Location | Example |
|----------|----------|---------|
| 1 | Explicit path | `bazelbump recipe run ./my-recipe.star` |
| 2 | Project recipes | `./.bazelbump/recipes/*.star` |
| 3 | Project recipes (alt) | `./recipes/*.star` |
| 4 | User recipes | `~/.config/bazelbump/recipes/*.star` |
| 5 | Built-in recipes | Embedded in bazelbump binary |

Later sources do not override earlier ones with the same name.

### 10. Standard Library

The standard library (`@stdlib//`) provides common patterns:

#### 10.1 Rule Mappings

```starlark
# @stdlib//rules.star

# Pattern constants for detecting native rules
NATIVE_RULE_PATTERNS = {
    "cc": [r"cc_(binary|library|test|import|shared_library)\s*\("],
    "python": [r"py_(binary|library|test)\s*\("],
    "java": [r"java_(binary|library|test|import|plugin)\s*\("],
    "shell": [r"sh_(binary|test|library)\s*\("],
    "proto": [r"(proto_library|cc_proto_library|java_proto_library|py_proto_library)\s*\("],
}

# Load path mappings
RULE_LOAD_PATHS = {
    "cc_binary": "@rules_cc//cc:defs.bzl",
    "cc_library": "@rules_cc//cc:defs.bzl",
    "py_binary": "@rules_python//python:defs.bzl",
    # ... etc
}

# Recommended dependency versions
RULE_DEP_VERSIONS = {
    "rules_cc": "0.1.1",
    "rules_python": "1.4.1",
    "rules_java": "8.6.3",
    "rules_shell": "0.4.1",
}
```

#### 10.2 Migration Helpers

```starlark
# @stdlib//migrations/cc.star

load("@stdlib//rules.star", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")

def cc_autoload_ops(version = None):
    """Generate operations for CC rule autoload migration.

    Args:
        version: rules_cc version (default: from RULE_DEP_VERSIONS)

    Returns:
        List of operations to add CC rule support.
    """
    v = version or RULE_DEP_VERSIONS["rules_cc"]
    return [
        ensure_bazel_dep("rules_cc", v),
        ensure_load("cc_binary", RULE_LOAD_PATHS["cc_binary"]),
        ensure_load("cc_library", RULE_LOAD_PATHS["cc_library"]),
        ensure_load("cc_test", RULE_LOAD_PATHS["cc_test"]),
    ]

def cc_migration_steps():
    """Generate complete CC migration recipe steps."""
    detected = scan(
        id = "detect-cc",
        files = ["**/BUILD", "**/BUILD.bazel"],
        patterns = NATIVE_RULE_PATTERNS["cc"],
    )

    return [
        detected,  # Scan step
        edit(
            id = "migrate-cc",
            operations = [
                ensure_bazel_dep("rules_cc", RULE_DEP_VERSIONS["rules_cc"]),
                ensure_load_for_detected(detected),
            ],
        ),
    ]
```

### 11. CLI Interface

```bash
# List all recipes (discovered + built-in)
bazelbump recipe list [--json]

# Show recipe details
bazelbump recipe show <name>
bazelbump recipe show ./path/to/recipe.star

# Run a recipe
bazelbump recipe run <name> [flags]
bazelbump recipe run ./path/to/recipe.star [flags]

# Validate a recipe without running
bazelbump recipe validate ./path/to/recipe.star

# Flags for 'run'
--dry-run           # Preview changes without applying
--path <dir>        # Project directory (default: .)
--step <id>         # Run only specific step
--verbose           # Verbose output
--json              # JSON output
--no-verify         # Skip verification steps
--continue-on-error # Don't stop on step failure
```

### 12. Testing Support

Recipes can be tested in isolation (inspired by Isopod):

```starlark
# recipes/test/autoload_test.star

load("@stdlib//testing.star", "testing")
load("//autoload.star", "autoload_migration")

def test_detects_cc_binary():
    """Test that cc_binary is detected."""
    t = testing.new()

    # Set up mock filesystem
    t.create_file("BUILD", '''
cc_binary(
    name = "hello",
    srcs = ["hello.cc"],
)
    ''')
    t.create_file("MODULE.bazel", '''
module(name = "test")
    ''')

    # Run recipe in test mode
    result = t.run_recipe(autoload_migration)

    # Assertions
    t.assert_true(result.success)
    t.assert_equals(len(result.changes), 2)  # MODULE.bazel + BUILD

    # Check file contents after migration
    build_content = t.read_file("BUILD")
    t.assert_contains(build_content, 'load("@rules_cc//cc:defs.bzl"')

def test_skips_when_load_exists():
    """Test that existing loads are not duplicated."""
    t = testing.new()

    t.create_file("BUILD", '''
load("@rules_cc//cc:defs.bzl", "cc_binary")

cc_binary(name = "hello")
    ''')

    result = t.run_recipe(autoload_migration)

    # Should succeed but make no changes to BUILD
    t.assert_true(result.success)
    build_changes = [c for c in result.changes if c.path == "BUILD"]
    t.assert_equals(len(build_changes), 0)
```

**Testing Framework API:**

```starlark
# @stdlib//testing.star

def new():
    """Create a new test context."""
    return TestContext()

class TestContext:
    def create_file(self, path, content): ...
    def create_dir(self, path): ...
    def read_file(self, path): ...
    def run_recipe(self, recipe): ...
    def run_step(self, step): ...

    def assert_true(self, condition, msg=None): ...
    def assert_false(self, condition, msg=None): ...
    def assert_equals(self, actual, expected, msg=None): ...
    def assert_contains(self, haystack, needle, msg=None): ...
    def assert_file_exists(self, path): ...
    def assert_file_contains(self, path, content): ...
```

### 13. Error Messages

Errors include file, line, and context information:

```
error: recipes/my-recipe.star:15:5: undefined symbol 'ensure_laod'
  did you mean: ensure_load

error: recipe "my-migration" validation failed:
  - step "edit-1" depends on unknown step "scan-typo"
  - step "edit-2" has no operations
  - pattern "cc_binary(" in step "detect" is not a valid regex: missing \(

error: step "add-deps" failed during execution:
  file: MODULE.bazel
  operation: ensure_bazel_dep("rules_cc", "0.1.1")
  cause: file does not exist
  hint: create MODULE.bazel first or use 'bzlmod init'
```

### 14. Security Model

#### 14.1 Sandboxing

Starlark execution is fully sandboxed:
- ❌ No filesystem access (except through operations)
- ❌ No network access
- ❌ No environment variable access
- ❌ No command execution (except through verify steps)
- ❌ No eval/exec of arbitrary code

#### 14.2 Resource Limits

| Resource | Limit | Rationale |
|----------|-------|-----------|
| Execution steps | 100,000 | Prevent infinite loops |
| Recursion depth | 1,000 | Prevent stack overflow |
| Load depth | 100 | Prevent deep dependency chains |
| String length | 10MB | Prevent memory exhaustion |

#### 14.3 Path Restrictions

- Load paths are restricted to recipe directories and stdlib
- File operations are restricted to project directory
- No access to paths outside project root

---

## Examples

### Example 1: Simple Autoload Migration

```starlark
# recipes/autoload.star

load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_DEP_VERSIONS")

recipe(
    name = "autoload-migration",
    version = "1.0.0",
    description = "Migrate native rules to explicit load statements",
    min_bazel_version = "8.0.0",
    steps = [
        scan(
            id = "detect",
            name = "Detect native rule usage",
            files = ["**/BUILD", "**/BUILD.bazel"],
            patterns = (
                NATIVE_RULE_PATTERNS["cc"] +
                NATIVE_RULE_PATTERNS["python"] +
                NATIVE_RULE_PATTERNS["shell"]
            ),
        ),
        edit(
            id = "add-deps",
            name = "Add module dependencies",
            operations = [
                ensure_bazel_dep("rules_cc", RULE_DEP_VERSIONS["rules_cc"]),
                ensure_bazel_dep("rules_python", RULE_DEP_VERSIONS["rules_python"]),
                ensure_bazel_dep("rules_shell", RULE_DEP_VERSIONS["rules_shell"]),
            ],
        ),
        edit(
            id = "add-loads",
            name = "Add load statements",
            operations = lambda ctx: [
                ensure_load_for_detected(ctx.outputs["detect"]),
            ],
        ),
        verify(
            id = "check",
            name = "Verify build",
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        ),
    ],
)
```

### Example 2: Smart Migration with Future References

```starlark
# recipes/smart-autoload.star

load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS")
load("@stdlib//migrations/cc.star", "cc_autoload_ops")
load("@stdlib//migrations/python.star", "python_autoload_ops")
load("@stdlib//migrations/shell.star", "shell_autoload_ops")

# Scan returns a future reference
detected = scan(
    id = "detect",
    name = "Detect native rules",
    files = ["**/BUILD", "**/BUILD.bazel"],
    patterns = [
        pattern
        for patterns in NATIVE_RULE_PATTERNS.values()
        for pattern in patterns
    ],
)

def build_operations(ctx):
    """Build operations based on what was detected."""
    ops = []
    families = ctx.outputs["detect"].families

    if "cc" in families:
        ops.extend(cc_autoload_ops())
    if "python" in families:
        ops.extend(python_autoload_ops())
    if "shell" in families:
        ops.extend(shell_autoload_ops())

    if not ops:
        skip("No native rules detected")

    return ops

recipe(
    name = "smart-autoload",
    version = "1.0.0",
    description = "Only migrate rules actually used in the project",
    steps = [
        detected,
        edit(
            id = "migrate",
            name = "Add required dependencies and loads",
            operations = build_operations,
        ),
    ],
)
```

### Example 3: Composable Recipe with Library

```starlark
# lib/helpers.star

def migration_steps_for_family(family, dep_name, version, rules, load_base):
    """Generate migration steps for a rule family.

    Args:
        family: Rule family name (for patterns)
        dep_name: Bazel dependency name
        version: Dependency version
        rules: List of rule names
        load_base: Base path for load statements

    Returns:
        Tuple of (scan_step, edit_step)
    """
    detected = scan(
        id = "detect-" + family,
        name = "Detect " + family + " rules",
        files = ["**/BUILD", "**/BUILD.bazel"],
        patterns = [r + r"\s*\(" for r in rules],
    )

    edit_step = edit(
        id = "migrate-" + family,
        name = "Migrate " + family + " rules",
        operations = [ensure_bazel_dep(dep_name, version)] + [
            ensure_load(rule, load_base)
            for rule in rules
        ],
    )

    return (detected, edit_step)
```

```starlark
# recipes/full-migration.star

load("./lib/helpers.star", "migration_steps_for_family")

cc_detect, cc_migrate = migration_steps_for_family(
    family = "cc",
    dep_name = "rules_cc",
    version = "0.1.1",
    rules = ["cc_binary", "cc_library", "cc_test"],
    load_base = "@rules_cc//cc:defs.bzl",
)

py_detect, py_migrate = migration_steps_for_family(
    family = "python",
    dep_name = "rules_python",
    version = "1.4.1",
    rules = ["py_binary", "py_library", "py_test"],
    load_base = "@rules_python//python:defs.bzl",
)

recipe(
    name = "full-migration",
    version = "1.0.0",
    description = "Complete native rule migration",
    steps = [
        cc_detect, cc_migrate,
        py_detect, py_migrate,
        verify(
            id = "final-check",
            name = "Verify everything builds",
            command = "bazel build //...",
        ),
    ],
)
```

---

## Implementation Plan

### Phase 1: Core Foundation

1. Add `go.starlark.net` dependency
2. Implement three-phase execution engine
3. Basic builtins (`recipe`, `scan`, `edit`, `verify`)
4. Context object with outputs
5. Error handling (`fail`, `warn`, `skip`)

**Files:**
- `recipe/starlark/interpreter.go` - Starlark interpreter
- `recipe/starlark/builtins.go` - Built-in functions
- `recipe/starlark/phases.go` - Three-phase execution

### Phase 2: Operations

1. Implement operation functions
2. File pattern matching with glob
3. Change tracking and rollback
4. Dry-run support

**Files:**
- `recipe/starlark/ops_build.go` - BUILD file operations
- `recipe/starlark/ops_module.go` - MODULE.bazel operations
- `recipe/starlark/ops_bazelrc.go` - .bazelrc operations

### Phase 3: Future References

1. Implement future reference types
2. Wire futures through execution phases
3. Auto-dependency inference from futures

**Files:**
- `recipe/starlark/futures.go` - Future reference implementation

### Phase 4: Load System

1. Implement `load()` statement resolution
2. Cycle detection
3. Standard library embedding

**Files:**
- `recipe/starlark/loader.go` - Load resolution
- `recipe/stdlib/` - Standard library .star files

### Phase 5: Discovery & CLI

1. Recipe discovery from standard paths
2. CLI commands (`list`, `show`, `run`, `validate`)
3. JSON output support

**Files:**
- `recipe/discovery.go` - Path scanning
- `cmd/bazelbump/recipe.go` - CLI integration

### Phase 6: Testing Framework

1. Mock filesystem for tests
2. Test runner
3. Assertion functions

**Files:**
- `recipe/starlark/testing.go` - Test framework

---

## Open Questions

1. **Extension repository hosting** - Where should community recipes live?
2. **Recipe versioning** - How to handle breaking changes in recipe DSL?
3. **Async operations** - Should verify steps support async/parallel execution?
4. **Recipe composition** - Can recipes depend on other recipes?

---

## References

- [Starlark Language Spec](https://github.com/bazelbuild/starlark/blob/master/spec.md)
- [go.starlark.net](https://pkg.go.dev/go.starlark.net/starlark)
- [Tilt API Reference](https://docs.tilt.dev/api.html)
- [Kurtosis Multi-Phase Runs](https://docs.kurtosis.com/advanced-concepts/multi-phase-runs/)
- [Copybara Reference](https://github.com/google/copybara/blob/master/docs/reference.md)
- `docs/research/starlark-tools-analysis.md` - Detailed tool analysis
