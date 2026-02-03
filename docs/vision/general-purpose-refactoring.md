# Vision: A General-Purpose Starlark Refactoring Engine

*From bazelbump to something bigger*

---

## The Realization

What started as a tool to help upgrade Bazel versions has revealed a more powerful idea: **a general-purpose, Starlark-based engine for large-scale code refactoring**.

The core primitives we built for bazelbump—file scanning, pattern matching, safe transformations, dry-run execution, rollback support—are not Bazel-specific. They're universal.

---

## The Core Engine

```
┌─────────────────────────────────────────────────────────────┐
│                    Recipe Engine Core                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Starlark DSL                                               │
│  ├── Familiar syntax (BUILD files, Tiltfiles, etc.)        │
│  ├── Safe execution (no eval, no network, sandboxed)       │
│  └── Deterministic and reproducible                         │
│                                                             │
│  Three-Phase Execution                                       │
│  ├── Interpret: Parse recipe, build operation graph         │
│  ├── Validate: Check everything before touching files       │
│  └── Execute: Apply changes with rollback support           │
│                                                             │
│  Primitives                                                  │
│  ├── File I/O: read_file, glob, list_dir, file_exists      │
│  ├── Patterns: regex (re.*), structural matching            │
│  ├── Transforms: replace, insert, remove, ensure           │
│  └── Verification: run commands, check results              │
│                                                             │
│  Safety                                                      │
│  ├── Dry-run mode (always preview first)                   │
│  ├── Diff-style output (see exactly what changes)          │
│  ├── Snapshot/rollback (undo if needed)                    │
│  └── Validation before execution (fail fast)               │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

This engine is **domain-agnostic**. It doesn't know about Bazel, Go, Python, or any specific technology. It just knows how to:

1. Find files
2. Match patterns
3. Transform content
4. Do it safely

---

## Domain Plugins

The power comes from **domain plugins** that add specialized knowledge:

```
                         ┌─────────────────┐
                         │  Recipe Engine  │
                         │     (core)      │
                         └────────┬────────┘
                                  │
          ┌───────────────────────┼───────────────────────┐
          │                       │                       │
          ▼                       ▼                       ▼
   ┌─────────────┐        ┌─────────────┐        ┌─────────────┐
   │  bazelbump  │        │   (future)  │        │   (future)  │
   │             │        │             │        │             │
   │  Bazel      │        │  Go         │        │  Python     │
   │  upgrades   │        │  migrations │        │  refactors  │
   └─────────────┘        └─────────────┘        └─────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
   • Flag database         • AST parsing          • AST parsing
   • .bazelrc parsing      • Import analysis      • Import analysis
   • BUILD parsing         • Type awareness       • Type awareness
   • Version knowledge     • Go module info       • pyproject.toml
```

### bazelbump (Current Focus)

The first and most mature domain plugin. Adds:

- **Bazel flag database** — Knowledge of flags across 93+ versions
- **Version diffing** — What changed between Bazel X and Y
- **.bazelrc parsing** — Understand config inheritance
- **BUILD file parsing** — Structured access to rules and loads
- **MODULE.bazel support** — bzlmod migrations
- **Native rule knowledge** — Which rules need explicit loads

### Future Domains (Ideas)

| Domain | What It Would Add |
|--------|-------------------|
| **Go** | `parse_go_file()`, import analysis, `go.mod` awareness |
| **Python** | `parse_python_file()`, import rewriting, type hints |
| **JavaScript** | `parse_js_file()`, ESM/CJS detection, package.json |
| **Rust** | `parse_rust_file()`, Cargo.toml, edition migrations |
| **Kubernetes** | YAML parsing, schema validation, API version updates |
| **Terraform** | HCL parsing, provider upgrades, state awareness |
| **Docker** | Dockerfile parsing, base image updates |
| **SQL** | Schema migrations, query rewrites |

---

## Why Starlark?

We chose Starlark deliberately:

1. **Familiar** — Millions of developers know it from Bazel, Buck2, Tilt
2. **Safe** — No arbitrary code execution, no network access, deterministic
3. **Readable** — Python-like syntax is approachable
4. **Embeddable** — Easy to integrate into Go tools
5. **Proven** — Battle-tested in production build systems

Starlark is the **configuration language that can also compute**. Unlike YAML (static) or full Python (unsafe), Starlark hits the sweet spot for refactoring recipes.

---

## The User Experience

### Writing a Recipe

```starlark
# my-migration.star

def find_deprecated_calls(ctx):
    """Custom scanner - full control."""
    results = []
    for f in glob("**/*.py"):
        content = read_file(f)
        for match in re.findall(r"old_api\.(\w+)\(", content):
            results.append({"file": f, "func": match})
    return results

def generate_fixes(ctx):
    """Generate fixes from scan results."""
    ops = []
    for r in ctx.outputs["scan"]:
        ops.append(replace_text(
            old = "old_api." + r["func"],
            new = "new_api." + r["func"],
            files = [r["file"]],
        ))
    return ops

recipe(
    name = "migrate-to-new-api",
    version = "1.0.0",
    steps = [
        scan(id = "scan", impl = find_deprecated_calls),
        edit(id = "fix", operations = generate_fixes, depends_on = ["scan"]),
        verify(id = "test", command = "pytest"),
    ],
)
```

### Running a Recipe

```bash
# Preview changes
recipes run my-migration.star --dry-run

# Output:
# ==> Running recipe: migrate-to-new-api (dry-run)
#
# src/handlers.py
# ───────────────
# @@ -15,1 +15,1 @@
# - result = old_api.fetch_user(id)
# + result = new_api.fetch_user(id)
#
# src/utils.py
# ────────────
# @@ -8,1 +8,1 @@
# - old_api.init()
# + new_api.init()
#
# Summary: 2 files, 2 changes
# Run without --dry-run to apply.

# Apply changes
recipes run my-migration.star

# Validate a recipe
recipes validate my-migration.star

# List available builtins
recipes builtins
```

---

## Comparison to Existing Tools

| Tool | Language | Scope | Extensibility | Safety |
|------|----------|-------|---------------|--------|
| **OpenRewrite** | Java | JVM only | Recipe catalog | High |
| **Comby** | CLI | Any text | Patterns only | Medium |
| **jscodeshift** | JS | JS/TS only | Full code | Low |
| **Semgrep** | YAML | Many | Rules | High |
| **ast-grep** | YAML | Many | Rules | High |
| **Sourcegraph** | YAML+shell | Any | Limited | Medium |
| **This** | Starlark | Any | Full code | High |

**Our differentiation:**
- **Full custom logic** in a safe language (not just patterns/rules)
- **Familiar syntax** (Starlark, not a new DSL)
- **Batteries included** (file I/O, regex, validation, dry-run)
- **Domain plugins** for specialized knowledge

---

## Roadmap

### Phase 1: Perfect the Bazel Domain ✅ (Current)

bazelbump is our proving ground:

- [x] Core engine (three-phase execution)
- [x] Starlark DSL with builtins
- [x] File I/O, regex, BUILD parsing
- [x] Recipe discovery and templates
- [x] Validation and dry-run
- [x] Diff-style output
- [ ] Real-world testing on OSS projects
- [ ] Built-in recipes for common Bazel migrations
- [ ] Buildifier integration
- [ ] Production hardening

### Phase 2: Extract Core Engine

Once bazelbump is solid:

- [ ] Extract `recipes-core` package
- [ ] Define domain plugin interface
- [ ] Make bazelbump a plugin on top of core
- [ ] Document plugin authoring

### Phase 3: Additional Domains

Add more domain plugins based on demand:

- [ ] Go migrations (`go-recipes`)
- [ ] Python refactoring (`py-recipes`)
- [ ] JavaScript/TypeScript (`js-recipes`)
- [ ] Generic text transformations

### Phase 4: Ecosystem

Build the community:

- [ ] Recipe catalog (like OpenRewrite's)
- [ ] Recipe testing framework
- [ ] IDE integration
- [ ] CI/CD integration

---

## Naming Thoughts

The general-purpose tool needs a name. Options:

| Name | Pros | Cons |
|------|------|------|
| `recipes` | Simple, memorable, describes the model | Generic |
| `starlark-refactor` | Descriptive | Long |
| `codemod` | Industry term | Taken (Facebook) |
| `forge` | Evocative (shaping code) | Common word |
| `weave` | Pattern-based transformation | Abstract |
| `morph` | Transformation | Common word |
| `recipe` | Singular, clean | Conflicts with cooking |

For now, **bazelbump** remains the name while we focus on the Bazel domain. The general-purpose extraction can be named later.

---

## Philosophy

### Start Specific, Generalize Later

We're not building a general-purpose tool first and hoping it works. We're:

1. **Solving a real problem** (Bazel upgrades are painful)
2. **Building the right primitives** (they happen to be general)
3. **Proving it works** (on real Bazel projects)
4. **Extracting the general case** (once we understand the domain)

This is the opposite of premature abstraction.

### Recipes as the Unit of Migration

The "recipe" metaphor is powerful because:

- Recipes are **shareable** — Write once, use everywhere
- Recipes are **composable** — Combine smaller recipes
- Recipes are **testable** — Verify before running
- Recipes are **versionable** — Track changes over time
- Recipes are **discoverable** — Catalog and search

### Safety First

Large-scale refactoring is scary. Our answer:

1. **Always dry-run first** — See changes before applying
2. **Validate before execute** — Catch errors early
3. **Show diffs** — Understand exactly what changes
4. **Support rollback** — Undo if needed
5. **Sandbox execution** — Recipes can't do arbitrary things

---

## Conclusion

bazelbump started as a Bazel upgrade tool. It's becoming a **Starlark-based refactoring engine** that happens to have deep Bazel knowledge.

The path forward:

1. **Now:** Make bazelbump excellent for Bazel upgrades
2. **Next:** Extract the general engine
3. **Later:** Add more domains

We're standing on the shoulders of Copybara, OpenRewrite, Comby, and others. Our contribution is combining their best ideas with Starlark's safety and familiarity.

The goal: **Make large-scale refactoring as easy as writing a BUILD file.**

---

*"The best way to predict the future is to invent it."* — Alan Kay
