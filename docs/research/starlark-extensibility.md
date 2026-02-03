# 100% Starlark Extensibility: Research and Patterns

**Research Date:** 2026-02-03
**Purpose:** Inform bazelbump recipe system design for full Starlark authoring capability

## Executive Summary

This research examines how various tools enable 100% Starlark extensibility, where users can write custom logic entirely in Starlark without needing to modify the host application's source code. Key findings:

1. **Aspect Orion** provides the most relevant model for BUILD file generation via Starlark
2. **Buck2 BXL** offers the most comprehensive graph introspection and action execution
3. **Tilt** demonstrates practical patterns for file I/O and command execution
4. **Starlib** provides reusable modules (regex, http, encoding) that can be embedded

The critical insight: **enabling 100% Starlark authoring requires exposing carefully designed builtins that bridge Starlark's sandboxed execution to host capabilities**.

---

## 1. Aspect's Starlark Gazelle Extensions (Orion)

### 1.1 Overview

Aspect Build's [Orion](https://github.com/aspect-build/aspect-gazelle/tree/main/language/orion) is a Gazelle language implementation that allows writing BUILD file generation extensions entirely in Starlark (using a dialect called AXL - Aspect Extension Language).

**Repository:** https://github.com/aspect-build/aspect-gazelle

### 1.2 How It Works

Orion embeds a Starlark interpreter as a Gazelle "language" plugin. It exposes a top-level `aspect` symbol that provides the extension API:

```starlark
# Example Starlark extension for shell files
aspect.orion_extension(
    id = "rules_sh",
    prepare = lambda cfg: aspect.PrepareResult(
        sources = aspect.SourceExtensions(".bash", ".sh"),
    ),
    declare = lambda ctx: ctx.targets.add(
        kind = "sh_library",
        name = "shell",
        attrs = {
            "srcs": [s.path for s in ctx.sources],
        },
    ),
)
```

### 1.3 Lifecycle / Stages

Orion implements a **multi-stage execution model**:

| Stage | Purpose | Callback Signature |
|-------|---------|-------------------|
| **Prepare** | Declare which files to process and queries to run | `prepare(ctx: PrepareContext) -> PrepareResult` |
| **Analyze** | Analyze query results, declare symbols | `analyze(ctx: AnalyzeContext) -> error` |
| **Declare** | Declare targets for BUILD files | `declare(ctx: DeclareTargetsContext) -> DeclareTargetsResult` |

All stages are optional. Stages execute sequentially per BUILD file, but extensions within a stage may run in parallel.

### 1.4 Primitives / Builtins Exposed

#### Extension Registration

```starlark
# Register an extension
aspect.orion_extension(
    id = "my-ext",           # Unique identifier
    properties = {...},       # Configuration properties
    prepare = func,           # Prepare callback
    analyze = func,           # Analyze callback
    declare = func,           # Declare callback
)

# Register a rule kind
aspect.register_rule_kind(
    name = "my_rule",
    From = "//my:rules.bzl",
    NonEmptyAttrs = {"srcs"},
    MergeableAttrs = {"deps"},
    ResolveAttrs = {"deps"},
)

# Configure properties via BUILD directives
# gazelle:my_property value
```

#### Source Matchers

```starlark
# Match specific files
aspect.SourceFiles("file1.txt", "file2.txt")

# Match by extension
aspect.SourceExtensions(".ts", ".tsx", ".js")

# Match via glob (slower)
aspect.SourceGlobs("*.proto", "src/**/*.go")
```

#### Query Types (File Analysis)

```starlark
# AST queries using tree-sitter
aspect.AstQuery(
    filter = "*.ts",
    grammar = "typescript",
    query = "(import_statement source: (string) @import)",
)

# Regex queries
aspect.RegexQuery(
    filter = "*.go",
    expression = r"import\s+\"(?P<pkg>[^\"]+)\"",
)

# Raw file content
aspect.RawQuery(filter = "*.json")

# JSON queries using jq
aspect.JsonQuery(
    filter = "package.json",
    query = ".dependencies | keys[]",
)

# YAML queries using yq
aspect.YamlQuery(
    filter = "*.yaml",
    query = ".services[].name",
)
```

#### Target Declaration

```starlark
# In declare stage
ctx.targets.add(
    name = "my_target",
    kind = "ts_library",
    attrs = {
        "srcs": [s.path for s in ctx.sources],
        "deps": [aspect.Import(id="lodash", provider="npm")],
    },
    symbols = ["lodash"],  # Exported symbols
)

ctx.targets.remove(name = "old_target")
```

#### Import Resolution

```starlark
# Placeholder for labels resolved after declaration
aspect.Import(
    id = "lodash",           # Symbol identifier
    provider = "npm",        # Provider type
    optional = False,        # Error if not resolved
    src = "index.ts:5",      # Debug info
)

# Labels
aspect.Label(
    repo = "npm",
    pkg = "node_modules/lodash",
    name = "lodash",
)
```

#### Utilities

```starlark
path.join("dir", "subdir", "file.txt")
path.dirname("/foo/bar/baz.txt")  # "/foo/bar"
path.base("/foo/bar/baz.txt")     # "baz.txt"
path.ext("/foo/bar/baz.txt")      # ".txt"
```

### 1.5 Key Design Decisions

1. **Read-only file access** - Extensions read files through queries, not direct file I/O
2. **Declarative output** - Extensions declare targets; Gazelle handles writing BUILD files
3. **No direct side effects** - All modifications go through the target declaration API
4. **Go interop** - Starlark extensions coexist with Go-based Gazelle languages
5. **Configuration via directives** - `# gazelle:key value` in BUILD files

### 1.6 Loading Extensions

Extensions are discovered from:
- `${ORION_EXTENSIONS_DIR}/*.axl`
- `${ORION_EXTENSIONS}` comma-separated paths
- `.aspect/cli/config.yaml` plugin declarations

---

## 2. Buck2 BXL (Buck Extension Language)

### 2.1 Overview

[BXL](https://buck2.build/docs/bxl/) is Buck2's Starlark-based scripting system for graph introspection and custom build operations.

**Documentation:** https://buck2.build/docs/bxl/

### 2.2 Key Capabilities

BXL enables:
- Query the build graph (uquery, cquery)
- Inspect target attributes and providers
- Create and execute actions
- Generate artifacts
- Custom command-line arguments

### 2.3 API Design

```starlark
# Define a BXL function
def _impl(ctx):
    # Query the graph
    targets = ctx.uquery().kind("python_library", "//...")

    # Inspect attributes
    for target in targets:
        deps = ctx.cquery().deps(target)
        attrs = target.attrs

    # Access providers via analysis
    analysis = ctx.analysis(target)
    providers = analysis.providers

    # Run actions
    bxl_actions = ctx.bxl_actions()
    artifact = bxl_actions.actions.write("output.txt", "content")

    # Ensure artifacts are materialized
    ctx.output.ensure(artifact)

    # Print results
    ctx.output.print("Found %d targets" % len(targets))

my_bxl = bxl_main(
    impl = _impl,
    cli_args = {
        "target": cli_args.target_label(),
        "verbose": cli_args.bool(default=False),
    },
)
```

### 2.4 Primitives Exposed

| Category | Functions |
|----------|-----------|
| **Query** | `ctx.uquery()`, `ctx.cquery()`, `ctx.aquery()` |
| **Analysis** | `ctx.analysis(target).providers` |
| **Actions** | `ctx.bxl_actions().actions.write()`, `.run()`, `.copy_file()` |
| **Output** | `ctx.output.print()`, `ctx.output.ensure()`, `ctx.output.ensure_multiple()` |
| **Paths** | `get_path_without_materialization(artifact)` |
| **CLI Args** | `cli_args.string()`, `.bool()`, `.int()`, `.list()`, `.target_label()` |

### 2.5 File Reading

BXL provides artifact-based file reading:

```starlark
# In dynamic output context
content = artifacts[my_artifact].read_string()
lines = artifacts[my_artifact].read_lines()
data = artifacts[my_artifact].read_json()
```

### 2.6 Side Effects Model

- **Actions are the unit of side effects** - All modifications happen through actions
- **Artifacts are tracked** - Buck2's DICE graph tracks all file dependencies
- **Caching is automatic** - BXL scripts benefit from Buck2's incremental computation
- **No direct file writes** - All output goes through `actions.write()` and `ctx.output.ensure()`

### 2.7 Key Design Decisions

1. **Graph-centric** - Everything operates on the target/action graph
2. **Incremental by default** - Leverages DICE for caching
3. **Action-based I/O** - No direct file system access
4. **Explicit materialization** - Artifacts must be ensured to be written

---

## 3. Tilt (Tiltfile)

### 3.1 Overview

[Tilt](https://tilt.dev/) uses Starlark (Tiltfiles) for Kubernetes development orchestration.

**API Reference:** https://docs.tilt.dev/api.html

### 3.2 File I/O Builtins

```starlark
# Read file as string
content = read_file("config.yaml")
content = read_file("optional.txt", default="fallback")

# Parse structured files
data = read_json("package.json")
data = read_yaml("config.yaml")
docs = read_yaml_stream("multi-doc.yaml")

# List directory contents
files = listdir("./src", recursive=True)

# Watch for changes (triggers re-execution)
watch_file("Dockerfile")
```

### 3.3 Command Execution

```starlark
# Run command, get stdout
output = local("git rev-parse HEAD", quiet=True)

# Run with environment
output = local("npm install", env={"NODE_ENV": "production"})

# Background process as a resource
local_resource(
    name = "api-server",
    cmd = "go build -o api ./cmd/api",
    serve_cmd = "./api",
    deps = ["./cmd/api"],
)
```

### 3.4 Encoding/Decoding

```starlark
# JSON
json_str = encode_json({"key": "value"})
obj = decode_json('{"key": "value"}')

# YAML
yaml_str = encode_yaml({"key": "value"})
obj = decode_yaml("key: value")

# Blobs (for file content manipulation)
b = blob("raw content")
```

### 3.5 Module Loading

```starlark
# Load from local file
load("./lib/helpers.star", "my_func")

# Load from official extensions
load("ext://restart_process", "docker_build_with_restart")

# Dynamic loading (runtime)
symbols = load_dynamic("./generated.star")
```

### 3.6 Environment Access

```starlark
# Environment variables
value = os.getenv("MY_VAR", default="fallback")
os.putenv("MY_VAR", "value")

# Paths
cwd = os.getcwd()
full_path = os.path.join(cwd, "subdir", "file.txt")
dir_name = os.path.dirname(path)
base_name = os.path.basename(path)
```

### 3.7 Key Design Decisions

1. **Register-then-execute** - Functions register config, Tilt executes later
2. **File watching built-in** - Changes trigger automatic re-execution
3. **Local commands allowed** - `local()` can run arbitrary shell commands
4. **Extension ecosystem** - Curated `ext://` repository

---

## 4. Cirrus CI Starlark

### 4.1 Overview

[Cirrus CI](https://cirrus-ci.org/) uses Starlark for CI task configuration generation.

**Documentation:** https://cirrus-ci.org/guide/programming-tasks/

### 4.2 File System Module

Cirrus provides a **read-only** `fs` module:

```starlark
load("cirrus", "fs")

def main(ctx):
    # Check file existence
    if fs.exists("go.mod"):
        return go_tasks()

    # Check if path is directory
    if fs.isdir("./tests"):
        return test_tasks()

    # Read file content
    content = fs.read(".cirrus.yml")  # Returns string or None

    # List directory
    entries = fs.readdir("./configs")  # Returns list of names or None
```

### 4.3 Key Constraints

- **Read-only** - No file writing capability
- **No command execution** - Tasks are generated, not executed in Starlark
- **Sandboxed** - All paths resolved relative to project root
- **Remote loading** - Can load files from GitHub/GitLab URLs

---

## 5. Starlib (qri-io)

### 5.1 Overview

[Starlib](https://github.com/qri-io/starlib) is a community standard library for Starlark, providing common modules that can be embedded in any Starlark host.

**Repository:** https://github.com/qri-io/starlib

### 5.2 Available Modules

| Module | Purpose | Key Functions |
|--------|---------|---------------|
| **re** | Regular expressions | `re.match()`, `re.sub()`, `re.findall()` |
| **http** | HTTP client | `http.get()`, `http.post()`, `http.put()` |
| **encoding/json** | JSON encoding | `json.encode()`, `json.decode()` |
| **encoding/yaml** | YAML encoding | `yaml.encode()`, `yaml.decode()` |
| **encoding/csv** | CSV files | `csv.read()`, `csv.write()` |
| **encoding/base64** | Base64 | `base64.encode()`, `base64.decode()` |
| **hash** | Cryptographic hashes | `hash.md5()`, `hash.sha256()` |
| **time** | Time operations | `time.now()`, `time.parse()` |
| **bsoup** | HTML parsing | Beautiful Soup-like API |
| **geo** | Geospatial | Coordinate calculations |
| **math** | Mathematics | Standard math functions |
| **xlsx** | Excel files | Read spreadsheets |
| **zipfile** | ZIP archives | Archive operations |

### 5.3 Regex Module Example

```starlark
load("re.star", "re")

# Match and extract groups
m = re.match(r"(\w+)@(\w+\.com)", "user@example.com")
if m:
    user = m.group(1)
    domain = m.group(2)

# Find all matches
imports = re.findall(r'import "([^"]+)"', source)

# Substitution
cleaned = re.sub(r"\s+", " ", text)
```

### 5.4 HTTP Module Example

```starlark
load("http.star", "http")

# GET request
resp = http.get("https://api.example.com/data")
if resp.status_code == 200:
    data = resp.json()

# POST request
resp = http.post(
    "https://api.example.com/items",
    json_body={"name": "item"},
    headers={"Authorization": "Bearer token"},
)
```

### 5.5 Integration Pattern

Starlib modules are designed to be selectively included:

```go
import (
    "github.com/qri-io/starlib/re"
    "github.com/qri-io/starlib/encoding/json"
)

// Register modules with interpreter
predeclared := starlark.StringDict{
    "re":   re.Module,
    "json": json.Module,
}
```

---

## 6. LUCI Configuration Generator (lucicfg)

### 6.1 Overview

[lucicfg](https://chromium.googlesource.com/infra/luci/luci-go/+/master/lucicfg) is Google's configuration generator for LUCI CI, using Starlark with extensive builtins.

### 6.2 Architecture

lucicfg uses a "microkernel" architecture:
- **Go kernel** provides low-level builtins
- **Starlark stdlib** wraps builtins with nicer API
- **User code** builds on stdlib

### 6.3 Notable Builtins

```starlark
# File operations
io.read_file("path.txt")

# Protocol buffer support
proto.new_descriptor_set(
    name = "myprotos",
    blob = io.read_file("myprotos.descpb"),
)

# Regex (Go-flavored)
re.match(pattern, string)
re.subst(pattern, repl, string)

# Hashes
hash.sha256("content")

# Go templates
template.render(tmpl_string, data)
```

### 6.4 Key Design Decisions

1. **Proto-native** - First-class protobuf message support
2. **Layered stdlib** - Builtins wrapped in Starlark for ergonomics
3. **Output generation** - Emits config files from graph traversal
4. **Validation** - `lucicfg validate` checks without generating

---

## 7. Key Patterns for 100% Starlark Authoring

### 7.1 Required Builtins by Category

Based on all tools analyzed, here are the primitives typically needed:

#### File I/O

| Builtin | Purpose | Security Model |
|---------|---------|----------------|
| `read_file(path)` | Read file content | Sandboxed to project dir |
| `read_json(path)` | Parse JSON file | Sandboxed |
| `read_yaml(path)` | Parse YAML file | Sandboxed |
| `file_exists(path)` | Check existence | Read-only |
| `list_dir(path)` | List directory | Read-only |
| `glob(pattern)` | Match files | Read-only |

#### String/Pattern Operations

| Builtin | Purpose | Notes |
|---------|---------|-------|
| `re.match()` | Regex matching | Go or RE2 flavor |
| `re.findall()` | Find all matches | |
| `re.sub()` | Regex substitution | |
| `glob.match()` | Glob pattern match | |

#### Encoding

| Builtin | Purpose |
|---------|---------|
| `json.encode()` / `json.decode()` | JSON serialization |
| `yaml.encode()` / `yaml.decode()` | YAML serialization |
| `base64.encode()` / `base64.decode()` | Base64 |

#### Output/Side Effects

| Builtin | Purpose | Security Model |
|---------|---------|----------------|
| `write_file(path, content)` | Write file | Sandboxed, logged |
| `print()` / `log.info()` | Logging | Always allowed |
| `fail(msg)` | Abort execution | |

### 7.2 Side Effect Handling Patterns

Three primary approaches observed:

#### Pattern A: Register-then-Execute (Tilt, Copybara)

```starlark
# Starlark registers intent
ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")

# Host application executes later
# - Validates all registrations
# - Applies changes atomically
```

**Pros:** Safe, enables dry-run, good error messages
**Cons:** Limited dynamic behavior

#### Pattern B: Action-Based (Buck2 BXL)

```starlark
# Create action, returns artifact reference
artifact = actions.write("output.txt", content)

# Must explicitly materialize
ctx.output.ensure(artifact)
```

**Pros:** Caching, incrementality, explicit dependencies
**Cons:** Complex mental model

#### Pattern C: Direct I/O with Restrictions (Tilt)

```starlark
# Direct execution, but sandboxed
content = read_file("config.yaml")
output = local("git rev-parse HEAD")
```

**Pros:** Simple, intuitive
**Cons:** Harder to make hermetic

### 7.3 Sandbox/Security Patterns

All tools implement sandboxing:

| Aspect | Approach |
|--------|----------|
| **File system** | Restricted to project directory |
| **Network** | Disabled or explicit opt-in |
| **Commands** | Through dedicated builtins, logged |
| **Environment** | Read-only or explicit vars only |
| **Resource limits** | Execution steps, memory, time |

Core Starlark guarantees leveraged:
- Deterministic execution
- No reflection or eval
- No mutable global state
- Hermetic by default

### 7.4 Context/State Passing

Universal pattern: context objects

```starlark
def my_function(ctx):
    # Project info
    ctx.project_path
    ctx.bazel_version

    # Configuration
    ctx.config.dry_run

    # Previous step outputs
    ctx.outputs["scan-step"]

    # Logging
    ctx.log.info("message")
```

### 7.5 Load/Module System

Consistent patterns for code reuse:

```starlark
# Relative imports
load("./lib/helpers.star", "func")

# Absolute (from roots)
load("//common/rules.star", "PATTERNS")

# Standard library
load("@stdlib//rules.star", "RULE_MAPPING")

# Extensions
load("ext://community/android.star", "android_ops")
```

---

## 8. Recommendations for bazelbump

### 8.1 Proposed Builtin Categories

Based on this research, bazelbump should expose:

#### Core File Operations

```starlark
# Reading (sandboxed to project)
read_file(path, default=None) -> string
read_json(path, default=None) -> dict
read_yaml(path, default=None) -> dict
file_exists(path) -> bool
list_dir(path, recursive=False) -> list[string]
glob(pattern, exclude=[]) -> list[string]
```

#### Pattern Matching

```starlark
# Regex (embed starlib/re or similar)
re.match(pattern, text) -> Match | None
re.findall(pattern, text) -> list[string]
re.sub(pattern, replacement, text) -> string

# Glob patterns
glob.match(pattern, path) -> bool
```

#### BUILD/Starlark Parsing

```starlark
# Parse BUILD files to structured data
parse_build_file(path) -> BuildFile
parse_starlark(content) -> AST

# Query helpers
find_rule(build_file, name) -> Rule | None
find_rules(build_file, kind) -> list[Rule]
find_loads(build_file) -> list[LoadStatement]
```

#### Output Operations

```starlark
# Declarative (register-then-execute model)
ensure_load(rule, load_from, file=None)
ensure_bazel_dep(name, version, dev_dependency=False)
remove_flag(flag, file=".bazelrc")
replace_text(old, new, files)

# These register changes; host applies atomically after validation
```

#### Logging/Control Flow

```starlark
log.info(msg, *args)
log.warn(msg, *args)
log.debug(msg, *args)

fail(msg)      # Hard failure
warn(msg)      # Continue with warning
skip(msg)      # Skip current step
```

### 8.2 Execution Model

Adopt **three-phase execution** (like Kurtosis):

```
Phase 1: Interpretation
  - Parse Starlark
  - Execute to build operation graph
  - Resolve load() statements
  - Return future references for scans

Phase 2: Validation
  - Verify all file patterns match files
  - Check dependencies exist
  - Validate operation parameters
  - Report ALL errors before any modification

Phase 3: Execution
  - Run scan steps, resolve futures
  - Apply edit operations
  - Track changes for potential rollback
  - Run verify steps
```

### 8.3 Custom Scanning Logic

Enable users to write custom scanners:

```starlark
def custom_scanner(ctx):
    """Find deprecated API usage."""
    results = []
    for file in glob("**/*.go"):
        content = read_file(file)
        for match in re.findall(r"deprecated\.(\w+)\(", content):
            results.append({
                "file": file,
                "api": match.group(1),
            })
    return results

# Register as scan step
detected = scan(
    id = "deprecated-apis",
    impl = custom_scanner,
)
```

### 8.4 Custom Transformations

Enable users to write custom transforms:

```starlark
def custom_transform(ctx, detected):
    """Generate migration operations based on scan."""
    ops = []
    for result in detected:
        if result["api"] == "OldFunc":
            ops.append(replace_text(
                old = "deprecated.OldFunc(",
                new = "newpkg.NewFunc(",
                files = [result["file"]],
            ))
    return ops

edit(
    id = "migrate-apis",
    operations = custom_transform,
)
```

### 8.5 Security Model

```yaml
# Allowed:
- Read files within project directory
- Pattern matching with regex/glob
- Parsing BUILD/Starlark files
- Registering file modifications
- Logging

# Disallowed (no builtins provided):
- Network access
- Arbitrary command execution (only via verify steps)
- Environment variables (except explicit injection)
- File writes outside project
- eval/exec of dynamic code

# Resource limits:
- execution_steps: 100000
- recursion_depth: 1000
- load_depth: 100
- string_length: 10MB
- timeout: 5m
```

### 8.6 Implementation Approach

1. **Use go.starlark.net** as the interpreter
2. **Embed starlib modules** (re, json, yaml) for common operations
3. **Create custom builtins** for Bazel-specific operations
4. **Implement future references** for scan results
5. **Build operation graph** during interpretation
6. **Validate before execution** for safe rollback

---

## 9. References

### Primary Sources

- [Aspect Gazelle Orion README](https://github.com/aspect-build/aspect-gazelle/blob/main/language/orion/README.md)
- [Buck2 BXL Documentation](https://buck2.build/docs/bxl/)
- [Buck2 BXL RFC](https://buck2.build/docs/rfcs/implemented/bxl/)
- [Tilt API Reference](https://docs.tilt.dev/api.html)
- [Cirrus CI Starlark Guide](https://cirrus-ci.org/guide/programming-tasks/)
- [Starlib Repository](https://github.com/qri-io/starlib)
- [LUCI lucicfg](https://chromium.googlesource.com/infra/luci/luci-go/+/master/lucicfg)

### Starlark Core

- [Starlark Language Spec](https://github.com/bazelbuild/starlark/blob/master/spec.md)
- [go.starlark.net Package](https://pkg.go.dev/go.starlark.net/starlark)
- [Bazel repository_ctx](https://bazel.build/rules/lib/builtins/repository_ctx)

### Related bazelbump Docs

- `docs/research/starlark-tools-analysis.md` - Detailed tool patterns
- `docs/design/starlark-recipes-spec.md` - Recipe system specification

---

## Appendix A: Comparison Matrix

| Feature | Orion | BXL | Tilt | Cirrus | Starlib |
|---------|-------|-----|------|--------|---------|
| File read | Via queries | Via artifacts | `read_file()` | `fs.read()` | N/A |
| File write | Via target API | Via actions | N/A | N/A | N/A |
| Regex | Via queries | N/A | N/A | N/A | `re` module |
| JSON | Via queries | `read_json()` | `read_json()` | N/A | `json` module |
| HTTP | N/A | N/A | N/A | N/A | `http` module |
| Commands | N/A | Via actions | `local()` | N/A | N/A |
| Caching | Gazelle cache | DICE | File watch | N/A | N/A |
| Sandboxed | Yes | Yes | Partial | Yes | Depends on host |

## Appendix B: Sample bazelbump Builtin Implementation

```go
// Example Go implementation of read_file builtin
func builtinReadFile(
    thread *starlark.Thread,
    b *starlark.Builtin,
    args starlark.Tuple,
    kwargs []starlark.Tuple,
) (starlark.Value, error) {
    var path string
    var defaultVal starlark.Value = starlark.None

    if err := starlark.UnpackArgs(
        "read_file", args, kwargs,
        "path", &path,
        "default?", &defaultVal,
    ); err != nil {
        return nil, err
    }

    // Get context from thread
    ctx := getContext(thread)

    // Validate path is within project
    absPath, err := ctx.ResolvePath(path)
    if err != nil {
        return nil, fmt.Errorf("read_file: invalid path: %w", err)
    }

    // Read file
    content, err := os.ReadFile(absPath)
    if err != nil {
        if os.IsNotExist(err) && defaultVal != starlark.None {
            return defaultVal, nil
        }
        return nil, fmt.Errorf("read_file: %w", err)
    }

    return starlark.String(content), nil
}
```
