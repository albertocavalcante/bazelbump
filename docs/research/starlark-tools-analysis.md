# Starlark as Configuration/Extension Language: Tool Analysis

**Research Date:** 2026-02-03
**Purpose:** Inform bazelbump recipe system design decisions

## Executive Summary

This analysis examines eight tools that use Starlark as a configuration/extension language. Key findings:

1. **Two-phase execution** (interpretation + execution) is the gold standard for validation
2. **Context objects** are the universal pattern for state passing
3. **Domain-specific builtins** should be verbs (`add_service`, `ensure_load`)
4. **Future references** enable declarative-yet-dynamic configuration
5. **Sandboxing** is non-negotiable; all tools restrict filesystem/network access

---

## Tool-by-Tool Analysis

### 1. Tilt (Tiltfile)

**Overview:** Kubernetes development tool for local container orchestration.

#### Core API Design

Tilt exposes a rich set of domain-specific functions organized by concern:

```python
# Container/Image Management
docker_build(ref, context, live_update=[...])
custom_build(ref, command, deps)
k8s_yaml(yaml, allow_duplicates=False)
k8s_resource(workload, new_name, port_forwards)

# Local Development
local_resource(name, cmd, deps, serve_cmd)
local(command, quiet=False, env={})

# Configuration
config.parse()
config.define_string(name, args, default, usage)
config.define_bool(name, args, default, usage)
```

**Key Insight:** Functions register configuration rather than execute immediately. Tilt collects all registrations, then executes.

#### Extension Patterns

```python
# Official extension loading
load('ext://restart_process', 'docker_build_with_restart')
docker_build_with_restart('my-image', './src')

# Local extensions
load('./lib/helpers.star', 'my_helper')
```

Tilt maintains a [curated extension repository](https://github.com/tilt-dev/tilt-extensions) with vetted, approved extensions.

#### Execution Model

1. Tiltfile executes on startup
2. Functions register configuration (not execute)
3. Tilt assembles "resources" from registrations
4. File watching triggers re-execution

#### Load/Import System

- `load(path, *symbols)` - Compile-time loading with selective imports
- `load('ext://name', ...)` - Official extension resolution
- `load_dynamic(path)` - Runtime loading (returns dict)
- `include(path)` - Discouraged legacy pattern

#### Context/State Passing

```python
# Environment access
os.getenv('KEY', default)
os.getcwd()
config.main_dir  # Root Tiltfile directory

# Kubernetes context
k8s_context()    # Cluster context name
k8s_namespace()  # Target namespace
```

#### Error Handling

```python
fail(msg)    # Stop execution, propagate error
exit(code)   # Stop without error state
warn(msg)    # Non-fatal warning
```

#### Best Practices from Tilt

1. **Batch operations** - Functions accept one or many items
2. **Multiple input types** - Ports accept int, string, or PortForward object
3. **Automatic inference** - Image-to-YAML matching happens automatically
4. **Idempotency required** - Deploy commands must be idempotent

---

### 2. Aspect CLI (AXL - Aspect Extension Language)

**Overview:** Bazel tooling with Starlark-based BUILD file generation and workflow extensions.

#### Core API Design

Aspect uses `.axl` files with a special `aspect` global symbol:

```python
# BUILD file generation extensions
def configure(ctx):
    # Called during `aspect configure`
    for file in ctx.files:
        if file.endswith('.proto'):
            ctx.add_rule('proto_library', name=..., srcs=[file])
```

#### Extension Patterns

```yaml
# .aspect/cli/config.yaml
configure:
  plugins:
    - WORKSPACE_relative/path/to/my_plugin.star
```

Extensions can be:
- Local `.star` or `.axl` files
- From the [aspect-extensions](https://github.com/aspect-extensions) repository

#### Key Design Decisions

- **No compilation required** - Starlark is interpreted, enabling fast iteration
- **Gazelle interop** - AXL plugins can coexist with Go-based Gazelle extensions
- **Bazel-native** - Uses Starlark dialect familiar to Bazel users

---

### 3. Gazelle

**Overview:** BUILD file generator with language-plugin architecture.

#### Core API Design

Gazelle uses Go interfaces rather than Starlark, but provides directive-based configuration:

```python
# gazelle:prefix github.com/example/project
# gazelle:proto disable

gazelle(name = "gazelle")
```

#### Extension Patterns

Extensions are Go packages implementing the `Language` interface:

```go
// NewLanguage returns a Language implementation
func NewLanguage() language.Language

// Language interface methods
GenerateRules(...)
Resolve(...)
Configure(...)
```

```python
# Using in BUILD files
gazelle_binary(
    name = "my_gazelle",
    languages = [
        "@rules_python_gazelle_plugin//python",
        "@bazel_gazelle//language/go",
    ],
)
```

#### Key Insight for bazelbump

Gazelle's directive system (`# gazelle:key value`) shows how inline configuration can complement file-based config.

---

### 4. Kurtosis

**Overview:** Distributed system testing and development using Starlark.

#### Core API Design

Kurtosis introduces a **Plan** object as the central orchestration primitive:

```python
def run(plan):
    # Add a service
    postgres = plan.add_service(
        name = "postgres",
        config = ServiceConfig(
            image = "postgres:15",
            ports = {"postgres": PortSpec(5432)},
            env_vars = {"POSTGRES_PASSWORD": "test"},
        ),
    )

    # Execute commands on services
    result = plan.exec(
        service_name = "postgres",
        recipe = ExecRecipe(command = ["psql", "-c", "SELECT 1"]),
    )

    # Verify conditions
    plan.verify(result["code"], "==", 0)

    # Wait for conditions
    plan.wait(
        service_name = "api",
        recipe = GetHttpRequestRecipe(endpoint = "/health"),
        field = "code",
        assertion = "==",
        target_value = 200,
        timeout = "5m",
    )
```

#### Three-Phase Execution Model (Critical Pattern)

Kurtosis uses a **multi-phase run** architecture:

| Phase | What Happens | Errors Caught |
|-------|--------------|---------------|
| **Interpretation** | Starlark executes, Plan methods queue instructions | Syntax errors, undefined symbols |
| **Validation** | Plan is validated for consistency | Missing ports, duplicate services, invalid references |
| **Execution** | Instructions execute in order | Runtime errors, service failures |

```python
# During interpretation, this returns a "future reference", not the actual IP
postgres_ip = postgres.ip_address  # Not available yet!

# Kurtosis replaces future references during execution
api = plan.add_service(
    name = "api",
    config = ServiceConfig(
        env_vars = {"DB_HOST": postgres.ip_address},  # Works!
    ),
)
```

**Key Insight:** Future references enable declarative configuration while supporting dynamic values.

#### Error Handling

```python
# Assertions
plan.verify(value, "==", expected)  # Fails plan if false

# Acceptable codes
plan.exec(..., acceptable_codes=[0, 1])  # Multiple success codes
plan.exec(..., skip_code_check=True)     # Ignore exit code
```

#### Best Practice: File Artifacts

```python
# Render config files with Go templates
config = plan.render_templates(
    config = {
        "/app/config.yaml": struct(
            template = "host: {{.Host}}\nport: {{.Port}}",
            data = {"Host": postgres.ip_address, "Port": "5432"},
        )
    },
    name = "app-config",
)

# Pass to services
plan.add_service(
    name = "app",
    config = ServiceConfig(
        files = {"/config": config},
    ),
)
```

---

### 5. Copybara

**Overview:** Google's code transformation tool for repository migrations.

#### Core API Design

Copybara uses a **workflow-centric** model:

```python
core.workflow(
    name = "default",
    origin = git.github_origin(
        url = "https://github.com/google/copybara.git",
        ref = "master",
    ),
    destination = git.destination(
        url = "file:///tmp/foo",
        push = "master",
    ),
    authoring = authoring.pass_thru("Default <default@example.com>"),
    transformations = [
        core.replace(
            before = "//internal/path",
            after = "//external/path",
            paths = glob(["**/BUILD"]),
        ),
        core.move("", "third_party/copybara"),
    ],
)
```

#### Transformation Operations

```python
# Text manipulation
core.replace(before, after, paths)
core.filter_replace(...)
core.todo_replace(...)

# File operations
core.copy(before, after)
core.move(before, after)
core.remove(glob)
core.rename(before, after)

# Advanced
core.dynamic_transform(impl_function)
core.verify_match(regex, paths)
core.reverse(transformations)  # Generate inverse!

# Metadata
metadata.add_header(text)
metadata.map_author(mapping)
metadata.scrubber(patterns)
```

#### Error Handling

```python
# Graceful failure handling
core.fail_with_noop("message")  # Skip without error

# Explicit error reporting
feedback.context.error("message")
feedback.context.noop("message")
```

#### Key Insight: Bidirectional Workflows

Copybara supports bidirectional synchronization by swapping origin/destination and using `origin_files` vs `destination_files`:

```python
# Pull workflow (external -> internal)
destination_files = glob(["third_party/copybara/**"])

# Push workflow (internal -> external)
origin_files = glob(["third_party/copybara/**"])
```

---

### 6. Skycfg (Stripe)

**Overview:** Starlark-based configuration for Protocol Buffers.

#### Core API Design

Skycfg is minimalist, focused on protobuf generation:

```python
pb = proto.package("google.protobuf")

def main(ctx):
    return [pb.StringValue(value = "Hello, world!")]
```

#### Go Integration

```go
// Load configuration
config, err := skycfg.Load(ctx, "config.sky")

// Execute with context variables
messages, err := config.Main(ctx, skycfg.WithVars(starlark.StringDict{
    "env": starlark.String("production"),
}))
```

#### Key Design Decision: Type Safety via Protobuf

```python
# Type errors are caught at interpretation time
pb.StringValue(value = 123)  # Error: expected string, got int
```

#### Load Path Customization

```python
# Supports Bazel-style labels
load("//config/common/kubernetes.sky", "kubernetes")

# Custom resolvers for alternative syntaxes
```

---

### 7. Isopod (Cruise Automation)

**Overview:** Kubernetes configuration without YAML, using Protocol Buffers directly.

#### Core API Design

```python
appsv1 = proto.package("k8s.io.api.apps.v1")
corev1 = proto.package("k8s.io.api.core.v1")

def install(ctx):
    kube.put(
        name = "nginx",
        namespace = ctx.namespace,
        data = [
            appsv1.Deployment(
                metadata = metav1.ObjectMeta(name = "nginx"),
                spec = appsv1.DeploymentSpec(
                    replicas = 3,
                    template = corev1.PodTemplateSpec(...),
                ),
            ),
        ],
    )
```

#### Built-in Modules

```python
# Kubernetes operations
kube.put(name, namespace, data)
kube.delete(name, namespace, kind)
kube.get(name, namespace, kind, wait)
kube.exists(name, namespace, kind)

# Secrets management
vault.read(path)
vault.write(path, data)
vault.exist(path)

# Utilities
base64.encode(data)
base64.decode(data)
uuid.v4()
http.get(url)
http.post(url, body)
hash.sha256(data)
sleep(seconds)
error(message)
```

#### Context Passing

```bash
# CLI context injection
isopod apply --context namespace=production,env=prod
```

```python
def clusters(ctx):
    if ctx.env == "prod":
        return PROD_CLUSTERS
    return DEV_CLUSTERS

def addons(ctx):
    return [
        addon("ingress", "configs/ingress.ipd", ctx),
    ]
```

#### Unit Testing Support

```python
# In my_addon_test.ipd
def test_install():
    t = testing.new()
    t.ctx.namespace = "test-ns"

    install(t.ctx)

    deployment = t.kube.get("nginx", "test-ns", "Deployment")
    t.assert_equals(deployment.spec.replicas, 3)
```

#### Key Insight: Dual Runtime

Isopod implements separate production and test runtimes:
- **Production runtime** - Real I/O operations
- **Test runtime** - Mocked builtins for hermetic tests

---

### 8. Qri (Dataset Transforms)

**Overview:** Dataset transformation using Starlark for ETL operations.

#### Core API Design

```python
load("http.star", "http")

def download(ctx):
    """Download is called before transform to fetch external data."""
    data = http.get("https://api.example.com/data.json")
    return data

def transform(ds, ctx):
    """Transform is the required function that modifies the dataset."""
    raw = ctx.download  # Access download results

    processed = []
    for item in raw:
        processed.append({
            "name": item["name"].upper(),
            "count": len(item["values"]),
        })

    ds.set_body(processed)
    ds.set_meta({
        "title": "Processed Data",
        "description": "Transformed from raw API data",
    })
```

#### Special Functions

| Function | When Called | Purpose |
|----------|-------------|---------|
| `download(ctx)` | Before transform | Fetch external data |
| `transform(ds, ctx)` | Main execution | Transform dataset |

#### Key Insight: Sandboxed Network Access

All network access must happen in `download()`, which runs in a controlled phase. The `transform()` function has no network access.

---

## Common Patterns Across Tools

### 1. Naming Conventions

| Category | Pattern | Examples |
|----------|---------|----------|
| Actions | `verb_noun` | `add_service`, `ensure_load`, `remove_flag` |
| Getters | `get_noun` | `get_service`, `get_files_artifact` |
| Predicates | `is_noun` / `has_noun` | `is_valid`, `has_dependency` |
| Configurations | `noun_config` | `ServiceConfig`, `PortSpec` |

### 2. Execution Models

| Model | Tools | Characteristics |
|-------|-------|-----------------|
| **Register-then-execute** | Tilt, Copybara | Functions register config, host executes later |
| **Multi-phase** | Kurtosis | Interpretation -> Validation -> Execution |
| **Immediate** | Skycfg, Isopod | Functions execute immediately during interpretation |

### 3. Context/State Patterns

All tools use a context object for state:

```python
# Kurtosis
def run(plan):
    plan.add_service(...)

# Copybara
core.workflow(origin=..., destination=...)

# Isopod
def install(ctx):
    kube.put(namespace=ctx.namespace, ...)

# Qri
def transform(ds, ctx):
    data = ctx.download
```

### 4. Load/Import Patterns

| Pattern | Syntax | Tools |
|---------|--------|-------|
| Selective import | `load("path", "symbol")` | All |
| Extension prefix | `load("ext://name", ...)` | Tilt |
| Relative paths | `load("./lib/helpers.star", ...)` | Most |
| Label syntax | `load("//path/file.bzl", ...)` | Bazel-adjacent |

### 5. Error Handling Approaches

```python
# Explicit failure
fail("message")                    # Universal
error("message")                   # Isopod

# Soft failure
warn("message")                    # Tilt
core.fail_with_noop("message")     # Copybara

# Assertions
plan.verify(value, "==", expected) # Kurtosis
t.assert_equals(actual, expected)  # Isopod tests
```

### 6. Async/Blocking Operations

| Approach | Tools | How It Works |
|----------|-------|--------------|
| **Future references** | Kurtosis | Return placeholders, resolve at execution |
| **Phased downloads** | Qri | `download()` runs before `transform()` |
| **Blocking inline** | Tilt | `local()` blocks during execution |

### 7. Host Capability Exposure

| Capability | How Exposed | Security Model |
|------------|-------------|----------------|
| File system | Limited builtins (`read_file`) | Read-only, path restrictions |
| Network | Phased (`download()`) or explicit module | Sandboxed, logged |
| Commands | Explicit functions (`local()`, `exec()`) | Logged, timeout controls |
| Environment | `os.getenv()` or context injection | Read-only |

---

## Recommendations for bazelbump

### 1. Adopt Multi-Phase Execution (from Kurtosis)

**Current spec** has steps executing in sequence. Consider:

```
Phase 1: Interpretation
  - Parse Starlark
  - Build step graph
  - Resolve dependencies

Phase 2: Validation
  - Check file patterns match files
  - Verify step dependencies exist
  - Validate operation parameters

Phase 3: Execution
  - Run steps in topological order
  - Track changes for rollback
```

**Benefits:**
- Catch errors before any files are modified
- Better error messages with full context
- Enables dry-run without partial execution

### 2. Use Future References for Scan Results (from Kurtosis)

```python
# Current spec - lambda for deferred access
edit(
    operations = lambda ctx: generate_ops(ctx.outputs["scan"]),
)

# Proposed - future references
detected = scan(
    id = "detect",
    patterns = [...],
)

edit(
    operations = [
        ensure_load_for_each(detected.rules),  # Future reference
    ],
)
```

### 3. Add Domain-Specific Builtins (from all tools)

Following naming conventions:

```python
# Analysis (verbs describing the action)
scan_files(patterns, file_globs)
detect_native_rules(file_globs)
find_deprecated_flags(bazelrc_path)

# Mutations (ensure_ prefix for idempotent operations)
ensure_load(rule, load_from, file)
ensure_bazel_dep(name, version)
remove_flag(flag, bazelrc_path)
replace_flag(old, new, bazelrc_path)

# Verification
verify_build(targets, timeout)
verify_test(targets, timeout)
```

### 4. Structured Error Handling (from Copybara/Kurtosis)

```python
# Soft failure - continue with warning
warn("Flag already removed, skipping")

# Recoverable failure - skip step
skip("No native rules detected")

# Hard failure - stop execution
fail("MODULE.bazel not found")

# Assertions in verify steps
verify(
    condition = build_succeeded,
    message = "Build failed after migration",
)
```

### 5. Extension System (from Tilt)

```python
# Built-in recipes (shipped with bazelbump)
load("@bazelbump//recipes:autoload.star", "autoload_migration")

# Official extensions (community-maintained)
load("ext://community/android.star", "android_migration")

# Local extensions
load("./lib/helpers.star", "custom_ops")
```

### 6. Testing Support (from Isopod)

```python
# recipes/test/autoload_test.star
def test_detects_cc_rules():
    t = testing.new()
    t.set_file("BUILD", """
cc_binary(name = "foo")
    """)

    result = detect_native_rules(t.ctx)

    t.assert_equals(len(result), 1)
    t.assert_equals(result[0].rule, "cc_binary")
```

### 7. Context Design (synthesized from all tools)

```python
def build_operations(ctx):
    # Project information
    ctx.project_path      # Root directory
    ctx.bazel_version     # Detected version

    # Step outputs (future references in validation, real in execution)
    ctx.outputs["scan-step-id"]

    # Configuration
    ctx.config.dry_run
    ctx.config.verbose

    # Utilities
    ctx.log.info("Processing...")
    ctx.log.warn("Deprecated pattern found")

    return [...]
```

### 8. Built-in Standard Library (from Tilt/Copybara)

```python
# @bazelbump//lib:rules.star
NATIVE_RULE_PATTERNS = {
    "cc": [r"cc_(binary|library|test|import)\s*\("],
    "python": [r"py_(binary|library|test)\s*\("],
    "shell": [r"sh_(binary|test|library)\s*\("],
    "java": [r"java_(binary|library|test|import|plugin)\s*\("],
    "proto": [r"(proto_library|.*_proto_library)\s*\("],
}

RULE_LOAD_PATHS = {
    "cc_binary": "@rules_cc//cc:defs.bzl",
    "cc_library": "@rules_cc//cc:defs.bzl",
    # ...
}

def cc_autoload_ops(version = "0.1.1"):
    """Generate operations for CC rule autoload migration."""
    return [
        ensure_bazel_dep("rules_cc", version),
        ensure_load("cc_binary", RULE_LOAD_PATHS["cc_binary"]),
        ensure_load("cc_library", RULE_LOAD_PATHS["cc_library"]),
        ensure_load("cc_test", RULE_LOAD_PATHS["cc_test"]),
    ]
```

---

## Implementation Priority

Based on this research, recommended implementation order:

### Phase 1: Core Foundation
1. Multi-phase execution (interpretation/validation/execution)
2. Basic builtins (`recipe`, `scan`, `edit`, `verify`)
3. Context object with outputs
4. Error handling (`fail`, `warn`, `skip`)

### Phase 2: Operations
1. `ensure_load`, `ensure_bazel_dep`, `remove_flag`
2. File pattern matching
3. Change tracking and dry-run

### Phase 3: Extension System
1. `load()` statement support
2. Discovery from standard paths
3. Standard library with common patterns

### Phase 4: Advanced
1. Future references
2. Testing framework
3. Extension repository

---

## References

- [Tilt API Reference](https://docs.tilt.dev/api.html)
- [Tilt Extensions Repository](https://github.com/tilt-dev/tilt-extensions)
- [Aspect CLI Starlark Docs](https://docs.aspect.build/cli/starlark/)
- [Kurtosis Plan API](https://docs.kurtosis.com/api-reference/starlark-reference/plan/)
- [Kurtosis Multi-Phase Runs](https://docs.kurtosis.com/advanced-concepts/multi-phase-runs/)
- [Copybara Reference](https://github.com/google/copybara/blob/master/docs/reference.md)
- [Skycfg README](https://github.com/stripe/skycfg)
- [Isopod README](https://github.com/cruise-automation/isopod)
- [Qri Starlark Transforms](https://github.com/qri-io/startf)
- [go.starlark.net Package](https://pkg.go.dev/go.starlark.net/starlark)
- [Gazelle Extension Guide](https://github.com/bazelbuild/bazel-gazelle/blob/master/extend.md)
