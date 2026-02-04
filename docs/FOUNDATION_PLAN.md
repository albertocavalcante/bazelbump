# bazelbump Foundation Plan

*Last updated: 2026-02-04*

This document outlines the foundational work needed before implementing features from `MIGRATION_STRATEGY.md`. The goal is to build solid, reusable primitives that unlock multiple features.

---

## Executive Summary

We need **5 foundational packages** before we can implement bisect integration, prerequisite checking, and agentic workflows:

1. **`pkg/project`** - Project context detection
2. **`pkg/bcr`** - BCR/registry client
3. **Version constraint parsing** - Enhance `internal/version`
4. **Starlark context builtins** - Expose Go packages to recipes
5. **`exec()` builtin** - Run commands from Starlark

---

## 1. Project Context Detection (`pkg/project`)

### Problem

Recipes currently have no way to understand the project they're operating on:
- What Bazel version does the project use?
- Is it using WORKSPACE or bzlmod?
- What are its bazel_dep dependencies?
- What's the project root?

### Proposed API

```go
package project

// Context holds detected project information.
type Context struct {
    Root           string            // Project root (where WORKSPACE/MODULE.bazel lives)
    BazelVersion   version.Version   // From .bazelversion
    UseBzlmod      bool              // True if MODULE.bazel exists and bzlmod enabled
    UseWorkspace   bool              // True if WORKSPACE exists
    BazelDeps      []BazelDep        // Parsed from MODULE.bazel
    Compatibility  []string          // From module(bazel_compatibility = [...])
}

// BazelDep represents a bazel_dep() entry.
type BazelDep struct {
    Name          string
    Version       string
    RepoName      string  // Optional repo_name override
    DevDependency bool
}

// Detect analyzes a directory and returns project context.
func Detect(path string) (*Context, error)

// DetectBazelVersion reads .bazelversion from path or parents.
func DetectBazelVersion(path string) (version.Version, error)

// ParseModuleBazel parses a MODULE.bazel file.
func ParseModuleBazel(path string) (*ModuleFile, error)
```

### Starlark Builtins

```starlark
# Get project context (memoized per run)
ctx = get_project_context()
ctx.bazel_version    # "7.7.1"
ctx.use_bzlmod       # True/False
ctx.use_workspace    # True/False
ctx.root             # "/path/to/project"

# Get bazel_deps from MODULE.bazel
deps = get_bazel_deps()
# [{"name": "rules_cc", "version": "0.0.17"}, ...]

# Get bazel_compatibility constraints
compat = get_bazel_compatibility()
# [">=6.4.0"]
```

### Why This Matters

- **Prerequisites**: Check if project Bazel version meets recipe requirements
- **Smart defaults**: Auto-detect whether to modify WORKSPACE or MODULE.bazel
- **Validation**: Verify dependencies before migration

---

## 2. Version Constraint Parsing

### Problem

The current `internal/version` package handles basic semver but doesn't support:
- BCR-style constraints: `>=6.4.0`, `<=9.0.0`, `-7.1.0`
- Bazelisk formats: `7.x`, `latest`, `latest-1`, `last_green`
- Multiple constraints: `[">=6.4.0", "<=9.0.0", "-7.1.0"]`

### Proposed Enhancements

```go
package version

// Constraint represents a version constraint.
type Constraint interface {
    Matches(v Version) bool
    String() string
}

// ParseConstraint parses a single constraint string.
// Supports: >=, <=, >, <, ==, !=, - (exclude)
func ParseConstraint(s string) (Constraint, error)

// ParseConstraints parses a list of BCR-style constraints.
// e.g., [">=6.4.0", "<=9.0.0", "-7.1.0"]
func ParseConstraints(ss []string) (ConstraintSet, error)

// ConstraintSet represents multiple constraints (all must match).
type ConstraintSet []Constraint

func (cs ConstraintSet) Matches(v Version) bool

// ParseBazeliskVersion parses bazelisk version formats.
// Supports: 7.0.0, 7.x, latest, latest-1, last_green, rolling
func ParseBazeliskVersion(s string) (*BazeliskVersion, error)
```

### Starlark Builtins

```starlark
# Check if version matches constraints
version_matches("7.7.1", [">=6.4.0", "<=9.0.0"])  # True
version_matches("7.1.0", [">=6.4.0", "-7.1.0"])   # False (excluded)

# Compare versions
version_compare("7.0.0", "8.0.0")  # -1
version_compare("8.0.0", "7.0.0")  # 1
version_compare("7.0.0", "7.0.0")  # 0

# Parse and validate version strings
v = parse_version("7.0.0-rc1")
v.major   # 7
v.minor   # 0
v.patch   # 0
v.prerelease  # "rc1"
```

### Why This Matters

- **Prerequisites**: Validate `min_bazel_version` and `max_bazel_version` in recipes
- **BCR compatibility**: Check if module versions work with target Bazel version
- **Bisect**: Parse version ranges for `--bisect=GOOD..BAD`

---

## 3. BCR/Registry Client (`pkg/bcr`)

### Problem

We can't check if a module version is compatible with a target Bazel version without fetching BCR metadata.

### Proposed API

```go
package bcr

const DefaultRegistry = "https://bcr.bazel.build"

// Client provides BCR access.
type Client struct {
    BaseURL string
    Cache   Cache  // Optional local cache
}

// Module represents BCR module metadata.
type Module struct {
    Name     string
    Versions []string
}

// ModuleVersion represents a specific version's metadata.
type ModuleVersion struct {
    Name              string
    Version           string
    BazelCompatibility []string  // From MODULE.bazel
    Dependencies      []Dependency
    Yanked           bool
    YankedReason     string
}

// GetModule fetches module metadata.
func (c *Client) GetModule(name string) (*Module, error)

// GetModuleVersion fetches specific version metadata.
func (c *Client) GetModuleVersion(name, version string) (*ModuleVersion, error)

// LatestCompatible finds the latest version compatible with Bazel version.
func (c *Client) LatestCompatible(module string, bazelVersion version.Version) (string, error)

// CheckCompatibility checks if a module@version works with a Bazel version.
func (c *Client) CheckCompatibility(module, moduleVersion string, bazelVersion version.Version) (bool, error)
```

### Starlark Builtins

```starlark
# Check module compatibility with target Bazel version
check_bcr_compatibility("rules_cc", "0.0.17", "9.0.0")  # True/False

# Get latest compatible version
latest = get_latest_compatible("rules_java", "9.0.0")  # "8.5.1"

# Get all versions of a module
versions = get_module_versions("rules_cc")  # ["0.0.1", "0.0.2", ...]

# Check if version is yanked
is_yanked("rules_cc", "0.0.5")  # True/False
```

### Why This Matters

- **Prerequisites**: Validate that dependencies will work with target Bazel
- **Auto-update**: Suggest latest compatible versions for bazel_deps
- **Migration planning**: Know which deps need updates for Bazel 9

---

## 4. Flag Database Starlark Builtins

### Problem

The `flagdb` package exists but isn't accessible from Starlark recipes.

### Proposed Builtins

```starlark
# Check if flag exists in a version
flag_exists("enable_bzlmod", "8.0.0")  # True

# Get flag details
flag = get_flag("enable_bzlmod", "8.0.0")
flag.name        # "enable_bzlmod"
flag.default     # "true"
flag.deprecated  # False
flag.description # "..."

# Get flags that changed between versions
changes = flag_diff("7.0.0", "8.0.0")
changes.added     # [Flag, ...]
changes.removed   # [Flag, ...]
changes.changed   # [FlagChange, ...]

# Get incompatible flags for a version
incompatible = get_incompatible_flags("9.0.0")

# Get deprecated flags
deprecated = get_deprecated_flags("8.0.0")
```

### Why This Matters

- **Version-aware recipes**: Only apply changes relevant to target version
- **Migration planning**: Know which flags need removal/addition
- **Validation**: Check that recipes don't reference non-existent flags

---

## 5. `exec()` Builtin for Starlark

### Problem

Custom scan/edit functions can't run external commands. The `run_command` operation only executes during the edit phase, not during scanning.

### Proposed Builtin

```starlark
# Execute a command and get result
result = exec(
    command = "bazel query //...",
    timeout = "30s",      # Optional, default 30s
    capture = True,       # Capture stdout/stderr
    check = True,         # Fail if exit code != 0
    env = {"FOO": "bar"}, # Optional env vars
    cwd = ".",            # Working directory
)

result.stdout      # Captured stdout
result.stderr      # Captured stderr
result.exit_code   # Exit code (0 = success)
result.success     # True if exit_code == 0

# Use in custom scan function
def find_targets(ctx):
    result = exec("bazel query 'kind(cc_library, //...)'")
    if not result.success:
        return []
    return result.stdout.strip().split("\n")
```

### Security Considerations

- Commands run in the project directory (sandboxed by default)
- Optional allowlist for permitted commands
- Timeout enforcement to prevent hangs
- No shell expansion by default (use `shell=True` to enable)

### Why This Matters

- **Bisect**: Run bazelisk bisect from recipes
- **Flexible scanning**: Query Bazel, run custom tools
- **Verification**: Custom build/test checks in recipes

---

## Implementation Order

### Phase 1: Core Primitives (1-2 weeks)

1. **Enhance `internal/version`** with constraint parsing
2. **Create `pkg/project`** with context detection
3. **Add Starlark builtins**: `get_project_context()`, `version_matches()`, `parse_version()`

### Phase 2: External Integration (1-2 weeks)

4. **Create `pkg/bcr`** with registry client
5. **Add Starlark builtins**: `check_bcr_compatibility()`, `get_latest_compatible()`
6. **Add `exec()` builtin** for command execution

### Phase 3: Flag Integration (1 week)

7. **Expose flagdb to Starlark**: `flag_exists()`, `flag_diff()`, `get_incompatible_flags()`
8. **Add prerequisite validation** to recipe execution

---

## Proposed Package Structure

```
bazelbump/
├── internal/
│   └── version/
│       ├── version.go       # Existing
│       ├── constraint.go    # NEW: Constraint parsing
│       └── bazelisk.go      # NEW: Bazelisk version formats
├── pkg/
│   ├── project/             # NEW
│   │   ├── context.go       # Project detection
│   │   ├── module.go        # MODULE.bazel parsing
│   │   └── workspace.go     # WORKSPACE detection
│   ├── bcr/                 # NEW
│   │   ├── client.go        # HTTP client
│   │   ├── cache.go         # Local cache
│   │   └── types.go         # Module, Version types
│   └── ...
├── recipe/starlark/
│   ├── interpreter.go       # Existing
│   ├── context_builtins.go  # NEW: Project context builtins
│   ├── version_builtins.go  # NEW: Version comparison builtins
│   ├── bcr_builtins.go      # NEW: BCR query builtins
│   ├── flagdb_builtins.go   # NEW: Flag database builtins
│   └── exec_builtin.go      # NEW: exec() builtin
```

---

## Success Criteria

After this foundational work, we should be able to:

1. ✅ Write a recipe that checks if the project's Bazel version meets requirements
2. ✅ Query BCR to find the latest compatible version of a module
3. ✅ Run bazelisk commands from within a recipe
4. ✅ Check which flags changed between Bazel versions
5. ✅ Automatically validate prerequisites before recipe execution

---

## Next Steps

1. **Review this plan** - Any gaps or concerns?
2. **Start with `pkg/project`** - Most immediately useful
3. **Add constraint parsing** - Required by project detection
4. **Iterate on Starlark API** - Get feedback on builtin names/signatures

---

*This plan prioritizes building blocks over features. Each piece should be small, testable, and immediately useful.*
