# Standing on the Shoulders of Giants

*How bazelbump's recipe system draws from the best ideas in the Starlark ecosystem*

---

## Introduction

> "If I have seen further, it is by standing on the shoulders of giants."
> — Isaac Newton

bazelbump's Starlark recipe system didn't emerge in a vacuum. It represents a synthesis of ideas from some of the most thoughtfully designed developer tools in the ecosystem. This document traces that lineage, acknowledges our inspirations, and explains how we've combined their best ideas into something tailored for Bazel migration workflows.

We believe in learning from those who came before us. Every tool we studied solved real problems for real users, and each offers lessons worth carrying forward.

---

## The Giants We Learned From

### 1. Copybara — The Pioneer of Starlark Transformations

**What it is:** Google's tool for transforming and moving code between repositories.

**What we learned:**

Copybara pioneered the idea of using Starlark for code transformations. Its `core.workflow()` abstraction showed us that complex, multi-step migrations could be expressed declaratively:

```starlark
# Copybara's approach
core.workflow(
    name = "import",
    origin = git.origin(url = "https://github.com/example/repo"),
    destination = git.destination(url = "https://internal/repo"),
    transformations = [
        core.replace("old_api", "new_api"),
        core.move("src/", "lib/"),
    ],
)
```

**Key insight adopted:** *Transformations as composable, declarative operations.* Rather than writing imperative scripts, you declare what you want to happen. The tool figures out how to make it so.

**What we do differently:** Copybara focuses on repository-to-repository workflows. bazelbump focuses on in-place migrations with Bazel-specific knowledge baked in.

---

### 2. Aspect Orion — Starlark-Native BUILD Generation

**What it is:** Aspect's system for writing Gazelle extensions entirely in Starlark (part of aspect-cli).

**What we learned:**

Orion demonstrated that you don't need Go to extend Gazelle. Its multi-stage execution model was revelatory:

```starlark
# Orion's three-stage model
aspect.orion_extension(
    prepare = lambda cfg: aspect.PrepareResult(
        sources = aspect.SourceExtensions(".ts", ".tsx"),
        queries = [aspect.AstQuery(grammar="typescript", query="...")],
    ),
    analyze = lambda ctx: process_query_results(ctx.query_results),
    declare = lambda ctx: ctx.targets.add(kind="ts_library", ...),
)
```

**Key insights adopted:**
- **Staged execution** — Separate "what to scan" from "what to do with results"
- **Query-based file analysis** — Don't just read files; query them structurally
- **Future references** — Declare dependencies on scan results before they exist

**What we do differently:** Orion generates BUILD files; bazelbump modifies existing files. We adapted the staged model for transformation rather than generation.

---

### 3. Buck2 BXL — Graph-Aware Scripting

**What it is:** Buck2's extension language for querying and manipulating the build graph.

**What we learned:**

BXL showed us the power of treating the build as a queryable graph:

```starlark
# BXL's graph-centric approach
def _impl(ctx):
    targets = ctx.uquery().kind("python_library", "//...")
    for target in targets:
        analysis = ctx.analysis(target)
        deps = analysis.providers[DependencyInfo].deps
        ctx.output.print(f"{target}: {len(deps)} deps")
```

**Key insights adopted:**
- **Everything is queryable** — Files, rules, dependencies
- **Explicit materialization** — Side effects happen only when you ask for them
- **Rich context objects** — Pass capabilities through `ctx`, not globals

**What we do differently:** BXL operates on Buck2's target graph. bazelbump operates on files and their contents, with Bazel version awareness layered on top.

---

### 4. Kurtosis — Three-Phase Execution

**What it is:** A platform for packaging and running distributed application environments.

**What we learned:**

Kurtosis's Starlark engine introduced us to rigorous phase separation:

```
Phase 1: Interpretation
  → Parse Starlark, build instruction graph
  → No side effects, just planning

Phase 2: Validation
  → Check all instructions are valid
  → Fail fast with ALL errors, not just the first

Phase 3: Execution
  → Actually do the work
  → With full knowledge that it will succeed
```

**Key insight adopted:** *Validate everything before changing anything.* Users should see all problems upfront, not discover them one at a time as the recipe runs.

**What we do differently:** Kurtosis orchestrates containers; bazelbump orchestrates file transformations. But the phase model translates perfectly.

---

### 5. Tilt — Developer-Friendly Builtins

**What it is:** A tool for Kubernetes development workflows using Starlark (Tiltfiles).

**What we learned:**

Tilt prioritizes developer ergonomics above all else:

```starlark
# Tilt's simple, direct API
config = read_yaml("config.yaml")
version = local("git rev-parse HEAD", quiet=True)

if config.get("experimental"):
    include("./experimental.star")
```

**Key insights adopted:**
- **Simple builtins** — `read_file()`, not `ctx.filesystem.open().read()`
- **Sensible defaults** — Most calls need only one argument
- **Escape hatches** — When you need power, it's there (`local()`)

**What we do differently:** Tilt allows arbitrary command execution. bazelbump is more restrictive for safety—we're modifying your codebase, so we must be careful.

---

### 6. Cirrus CI — Read-Only Safety

**What it is:** A CI platform with Starlark-based configuration generation.

**What we learned:**

Cirrus showed us that read-only file access is often enough:

```starlark
# Cirrus's fs module - read-only by design
load("cirrus", "fs")

def main(ctx):
    if fs.exists("go.mod"):
        return go_tasks()
    if fs.exists("package.json"):
        return node_tasks()
```

**Key insight adopted:** *Reading is safe; writing needs ceremony.* Our scan steps are pure reads. Only edit steps can modify, and they go through validation first.

---

### 7. Starlib — A Community Standard Library

**What it is:** A collection of reusable Starlark modules (regex, HTTP, encoding).

**What we learned:**

Don't reinvent the wheel. Starlib provides battle-tested implementations:

```starlark
# Starlib's re module
load("re.star", "re")

matches = re.findall(r'import "([^"]+)"', source)
cleaned = re.sub(r"\s+", " ", text)
```

**Key insight adopted:** *Embed, don't reimplement.* We use starlib's `re` module rather than writing our own regex implementation.

---

### 8. LUCI's lucicfg — Layered Architecture

**What it is:** Google's configuration generator for LUCI CI infrastructure.

**What we learned:**

lucicfg uses a "microkernel" architecture:

```
Layer 1: Go builtins (low-level, stable)
Layer 2: Starlark stdlib (ergonomic wrappers)
Layer 3: User code (domain-specific)
```

**Key insight adopted:** *Builtins should be minimal; convenience comes from Starlark.* Our Go code provides primitives; our `@stdlib//` provides the nice API.

---

## How bazelbump Synthesizes These Ideas

### Our Execution Model

We adopted Kurtosis's three-phase approach, enhanced with Orion's future references:

```
┌─────────────────────────────────────────────────────────────┐
│  Phase 1: Interpretation                                    │
│  ─────────────────────                                      │
│  • Parse recipe.star                                        │
│  • Execute to build operation graph                         │
│  • Scan steps return future references                      │
│  • No files modified yet                                    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Phase 2: Validation                                        │
│  ──────────────────                                         │
│  • Resolve all file patterns                                │
│  • Check operations are valid                               │
│  • Detect cycles in step dependencies                       │
│  • Report ALL errors at once                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Phase 3: Execution                                         │
│  ─────────────────                                          │
│  • Run scan steps, resolve futures                          │
│  • Apply edit operations                                    │
│  • Track changes for rollback                               │
│  • Run verify steps                                         │
└─────────────────────────────────────────────────────────────┘
```

### Our Builtin Philosophy

From Tilt, we learned that builtins should be simple. From Cirrus, we learned they should be safe. From lucicfg, we learned they should be minimal.

```starlark
# Simple (Tilt-inspired)
content = read_file("BUILD")

# Safe (Cirrus-inspired)
# - Sandboxed to project directory
# - Read-only in scan steps

# Minimal (lucicfg-inspired)
# - Go provides: read_file, glob, parse_build
# - Starlark stdlib provides: NATIVE_RULE_PATTERNS, get_load_path()
```

### Our Operation Model

From Copybara, we learned that transformations should be declarative. From Buck2, we learned that side effects need explicit boundaries:

```starlark
# Declarative (Copybara-inspired)
edit(
    operations = [
        ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
        remove_flag("experimental_enable_bzlmod"),
    ],
)

# Explicit boundaries (Buck2-inspired)
# Operations are registered during interpretation
# Applied atomically during execution
# Rolled back on failure
```

### Our Extension Model

From Orion, we learned that users should be able to write custom logic. From Starlib, we learned to embed proven modules:

```starlark
# Custom logic (Orion-inspired)
def custom_scanner(ctx):
    results = []
    for f in glob("**/*.go"):
        content = read_file(f)
        if "deprecated" in content:
            results.append({"file": f})
    return results

# Proven modules (Starlib-inspired)
# We embed starlib's re module for regex
matches = re.findall(r"pattern", content)
```

---

## Comparison Matrix

| Aspect | Copybara | Orion | BXL | Kurtosis | Tilt | **bazelbump** |
|--------|----------|-------|-----|----------|------|---------------|
| **Primary use** | Repo sync | BUILD gen | Graph query | Containers | K8s dev | Bazel migration |
| **Execution model** | Sequential | Staged | Graph | Three-phase | Direct | Three-phase |
| **File access** | Transform | Query | Artifact | Read | Read/Write | Read + Declare |
| **Side effects** | Immediate | Deferred | Actions | Deferred | Immediate | Deferred |
| **Rollback** | Git | N/A | N/A | Yes | N/A | Yes |
| **Custom logic** | Limited | Full | Full | Full | Full | Full |
| **Sandboxing** | Repo | Project | Buck2 | Container | Limited | Project |

---

## What Makes bazelbump Different

While we've learned from all these tools, bazelbump has a unique focus:

### 1. Bazel Version Awareness

No other tool understands Bazel flag lifecycles:

```starlark
# bazelbump knows when flags were removed/deprecated
analyze(from_version = "7.0.0", to_version = "8.0.0")
# → Automatically detects removed: --experimental_enable_bzlmod
# → Automatically detects changed: --incompatible_disable_starlark_host_transitions
```

### 2. Migration-Specific Primitives

Our builtins are designed for migration workflows:

```starlark
# Not just "add text" but "ensure this load exists"
ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")
# → Idempotent: won't duplicate if already present
# → Smart: adds to existing load() if possible

# Not just "remove line" but "remove this flag"
remove_flag("experimental_enable_bzlmod")
# → Handles all variants: --flag, --flag=value, --noflag
# → Handles all files: .bazelrc, user.bazelrc, etc.
```

### 3. Ecosystem Integration

bazelbump is designed to fit into the Bazel ecosystem:

```starlark
# Understands MODULE.bazel
ensure_bazel_dep("rules_cc", "0.0.9")

# Understands .bazelrc inheritance
# → Scans .bazelrc, user.bazelrc, CI configs

# Can invoke buildifier for formatting
verify(command = "buildifier -lint=fix")
```

---

## Gratitude

We are grateful to the teams behind:

- **Copybara** — For proving Starlark could power transformations
- **Aspect** — For showing the way to 100% Starlark extensibility
- **Buck2** — For the rigorous action/artifact model
- **Kurtosis** — For the three-phase execution insight
- **Tilt** — For prioritizing developer experience
- **Cirrus CI** — For demonstrating safe, read-only patterns
- **Starlib** — For the embeddable standard library
- **lucicfg** — For the layered architecture approach

And to the Starlark language designers at Google, whose decision to create a simple, hermetic, Python-like language made all of this possible.

---

## Conclusion

bazelbump's recipe system is not novel in its individual parts. Its value lies in the synthesis:

- **Copybara's** declarative transformations
- **Orion's** staged execution and future references
- **Kurtosis's** three-phase model with upfront validation
- **Tilt's** simple, ergonomic builtins
- **Cirrus's** safety-first file access
- **Starlib's** embeddable modules
- **lucicfg's** layered architecture

Combined with deep Bazel version knowledge and migration-specific primitives, this creates a tool uniquely suited to helping teams upgrade Bazel safely and systematically.

We hope that by documenting our influences, we can help others building in this space. And we hope that someday, someone will stand on our shoulders too.

---

*"Good artists copy; great artists steal."* — Picasso (maybe)

*"We stand on the shoulders of giants, and occasionally we bring snacks."* — The bazelbump team
