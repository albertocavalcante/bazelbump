# Handoff: Starlark API Architecture Implementation

## What Was Done

### Commits (bazelbump)
1. `c38c698` - feat(starlark): add namespaced module API for builtins
2. `fd8dee1` - docs: add module API documentation to recipe reference
3. `9ee9ff9` - feat(starlark): add bcr module for Bazel Central Registry queries
4. `628ece6` - feat(starlark/bcr): add latest_stable and is_prerelease functions
5. `d96fad0` - feat(starlark): add exec module for shell command execution

### New Files Created (bazelbump)
```
recipe/starlark/modules.go           # Module infrastructure (makeModule, deprecation helpers)
recipe/starlark/project_module.go    # project.* module
recipe/starlark/version_module.go    # version.* module
recipe/starlark/fs_module.go         # fs.* module
recipe/starlark/flagdb_builtins.go   # flags.* module
recipe/starlark/bcr_module.go        # bcr.* module
recipe/starlark/bcr_module_test.go   # bcr module tests
recipe/starlark/exec_module.go       # exec.* module (NEW)
recipe/starlark/exec_module_test.go  # exec module tests (NEW)
recipe/starlark/modules_test.go      # Tests for all modules
```

### Modified Files (bazelbump)
```
recipe/starlark/interpreter.go       # Register modules, add WithFlagDB/WithBCRClient/WithExecEnabled options
recipe/starlark/builtins_help.go     # Add module documentation
docs/user/recipe-reference.md        # Full API documentation
go.mod / go.sum                      # Added go-bcr dependency
```

### API Implemented (bazelbump)

| Module | Functions |
|--------|-----------|
| `project` | `context()`, `deps()`, `compatibility()` |
| `version` | `parse()`, `compare()`, `matches()`, `at_least()`, `in_range()` |
| `fs` | `read()`, `exists()`, `glob()`, `list_dir()`, `parse_build()` |
| `flags` | `exists()`, `get()`, `diff()`, `incompatible()`, `deprecated()` |
| `bcr` | `metadata()`, `source()`, `latest()`, `latest_stable()`, `versions()`, `exists()`, `version_exists()`, `is_yanked()`, `is_prerelease()` |
| `exec` | `run()`, `which()`, `enabled` |

---

## go-bcr Library Created

**Repo:** https://github.com/albertocavalcante/go-bcr

Lightweight, zero-dependency Go client for the Bazel Central Registry.

### Files
```
go-bcr/
├── go.mod              # github.com/albertocavalcante/go-bcr, Go 1.25
├── doc.go              # Package documentation
├── client.go           # Client, New, Options, methods
├── types.go            # Metadata, Source, Maintainer
├── errors.go           # ErrNotFound, NotFoundError, RequestError
├── client_test.go      # Unit tests
├── example_test.go     # Godoc examples
├── integration_test.go # Live BCR tests (go test -tags=integration)
├── LICENSE             # MIT
├── README.md           # Badges, docs
└── .github/workflows/ci.yml
```

### API
- `client.Metadata(ctx, module)` → `*Metadata`
- `client.Source(ctx, module, version)` → `*Source`
- `client.ModuleFile(ctx, module, version)` → `[]byte`
- `client.Latest(ctx, module)` → `string`
- `client.LatestStable(ctx, module)` → `string`
- `client.Versions(ctx, module)` → `iter.Seq2[string, error]`
- `client.Exists(ctx, module)` → `bool`
- `client.VersionExists(ctx, module, version)` → `bool`
- `bcr.IsPrerelease(version)` → `bool`

---

## exec Module

The exec module provides secure shell command execution from recipes.

### Security Model
- **Disabled by default** - requires `--allow-exec` flag
- Commands execute in project directory sandbox
- `cwd` parameter restricted to subdirectories of project
- Timeout limits enforced (default: 30s, max: 5m)

### API
```starlark
# Check if exec is available
exec.enabled  # bool

# Run a command
result = exec.run(
    "git rev-parse HEAD",
    timeout = "30s",     # optional, default 30s
    capture = True,      # optional, capture stdout/stderr
    check = True,        # optional, raise on non-zero exit
    env = {"KEY": "val"}, # optional environment
    cwd = "subdir",      # optional working directory
)
# result.stdout, result.stderr, result.exit_code, result.success

# Find command path
path = exec.which("bazel")  # Returns "" if not found
```

---

## Testing

All tests pass:
```bash
go test ./recipe/starlark/...  # bazelbump - All pass
go test ./...                   # go-bcr - All pass
go test -tags=integration ./... # go-bcr integration - All pass
```

---

## Usage Examples

### BCR Module
```starlark
# Get latest stable version
latest = bcr.latest_stable("rules_go")

# Check if prerelease
if bcr.is_prerelease(version):
    warn("Using prerelease version")

# Update dependency
update_bazel_dep(name = "rules_go", version = bcr.latest_stable("rules_go"))
```

### Exec Module
```starlark
# Guard for exec availability
if not exec.enabled:
    fail("This recipe requires --allow-exec")

# Get git info
result = exec.run("git rev-parse --short HEAD")
commit = result.stdout.strip()

# Check for required tools
if not exec.which("buildifier"):
    warn("buildifier not found, skipping format")
else:
    exec.run("buildifier -mode=fix .")
```
