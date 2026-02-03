# bazelbump Roadmap

*Last updated: 2026-02-03*

This document outlines the planned development roadmap for bazelbump. It reflects learnings from our research into the Starlark and code transformation ecosystems.

---

## Current State (v0.1)

✅ **Core Analysis Engine**
- Flag detection (removed, deprecated, changed defaults)
- Version diffing between Bazel releases
- .bazelrc scanning and parsing

✅ **CLI**
- `analyze` - Detect issues for upgrade
- `apply` - Apply automatic fixes
- `diff` - Show flag changes between versions
- `flags` - List flags with filters
- `recipe` - List, show, run recipes

✅ **Starlark Recipe System**
- Three-phase execution (interpret → validate → execute)
- Recipe discovery (project, user, builtin)
- Standard library (`@stdlib//`)
- Basic operations (ensure_load, remove_flag, etc.)

✅ **Documentation**
- User guides (getting-started, commands, recipes, authoring)
- Design docs (spec, research, philosophy)

---

## Phase 1: 100% Starlark Authoring ✅ COMPLETE

**Goal:** Enable users to write fully custom recipes without modifying Go code.

### 1.1 File I/O Builtins ✅

```starlark
content = read_file("src/BUILD")
exists = file_exists("MODULE.bazel")
files = list_dir("src/", recursive=True)
builds = glob("**/BUILD", exclude=["third_party/**"])
```

**Implemented:**
- [x] `read_file(path, default=None)` - Read file as string
- [x] `file_exists(path)` - Check if file exists
- [x] `list_dir(path, recursive=False)` - List directory contents
- [x] `glob(pattern, exclude=[])` - Match files by pattern

### 1.2 Regex Module ✅

```starlark
if re.match(r"^bazel_dep\(", line): ...
imports = re.findall(r'load\("([^"]+)"', content)
new_content = re.sub(r"--experimental_(\w+)", r"--\1", content)
```

**Implemented:**
- [x] Embed `github.com/qri-io/starlib/re`
- [x] Expose as `re` module in interpreter

### 1.3 BUILD File Parsing ✅

```starlark
build = parse_build_file("src/BUILD")
cc_rules = build.rules(kind="cc_*")
loads = build.loads()
```

**Implemented:**
- [x] `parse_build_file(path)` - Parse BUILD to structured object
- [x] `BuildFile.rules(kind=None)` - Get rules, optionally filtered
- [x] `BuildFile.loads()` - Get load statements
- [x] `Rule.name`, `Rule.kind`, `Rule.attr(name)`, `Rule.line`

### 1.4 Custom Scanner Functions ✅

```starlark
def find_deprecated_apis(ctx):
    results = []
    for file in glob("**/*.go"):
        content = read_file(file)
        for match in re.findall(r"deprecated\.(\w+)\(", content):
            results.append({"file": file, "api": match})
    return results

scan(id = "deprecated-apis", impl = find_deprecated_apis)
```

**Implemented:**
- [x] Support `impl` parameter on `scan()` step
- [x] Pass `ctx` with project_path, dry_run, outputs
- [x] Return value becomes step output for downstream steps

### 1.5 Custom Transform Functions ✅

```starlark
def generate_fixes(ctx):
    ops = []
    for result in ctx.outputs["deprecated-apis"]:
        ops.append(replace_text(old="deprecated." + result["api"], new="newpkg." + result["api"], files=[result["file"]]))
    return ops

edit(id = "apply-fixes", operations = generate_fixes, depends_on = ["deprecated-apis"])
```

**Implemented:**
- [x] Support callable for `operations` parameter
- [x] Pass `ctx` with outputs from previous steps
- [x] Callable returns list of operations

### 1.6 Authoring UX ✅

**Implemented:**
- [x] `recipe validate` - Validate recipes without running
- [x] `recipe builtins` - List/document available builtins
- [x] `recipe init` - Scaffold recipes from templates
- [x] Diff-style dry-run output with colors
- [x] JSON output for tooling

---

## Phase 2: Production Readiness

**Goal:** Make bazelbump reliable for real-world Bazel upgrades.

### 2.1 Real-World Testing

- [ ] Test on 5+ open-source Bazel projects
- [ ] Test on projects using bzlmod (MODULE.bazel)
- [ ] Test on projects using WORKSPACE
- [ ] Document edge cases and limitations

### 2.2 Built-in Recipes

Ship recipes for common migrations:

- [ ] `autoload-migration` - Add load() for native rules (Bazel 8→9)
- [ ] `bzlmod-migration` - WORKSPACE to MODULE.bazel
- [ ] `flag-cleanup` - Remove all deprecated flags for target version
- [ ] `incompatible-flags` - Set incompatible flags explicitly

### 2.3 Buildifier Integration

Preserve code style after edits:

```starlark
recipe(
    name = "my-migration",
    post_edit = "buildifier",  # Auto-format after edits
    steps = [...],
)
```

**Implementation:**
- [ ] Detect buildifier in PATH
- [ ] Run buildifier on modified BUILD files
- [ ] Option to skip formatting

### 2.4 Snapshot & Rollback

Enable safe experimentation:

```bash
# Create snapshot before changes
bazelbump snapshot create

# Apply recipe
bazelbump recipe run my-recipe

# If something went wrong
bazelbump snapshot restore

# List snapshots
bazelbump snapshot list
```

**Implementation:**
- [ ] `snapshot create` - Copy files to .bazelbump/snapshots/
- [ ] `snapshot restore [id]` - Restore from snapshot
- [ ] `snapshot list` - Show available snapshots
- [ ] Auto-snapshot before `apply` and `recipe run`

### 2.5 Better Error Messages

- [ ] Show file:line for all errors
- [ ] Suggest fixes for common mistakes
- [ ] Link to documentation for each error code

---

## Phase 3: Ecosystem Integration

**Goal:** Make bazelbump a good citizen in the Bazel ecosystem.

### 3.1 CI Integration

```yaml
# GitHub Actions
- name: Check Bazel upgrade compatibility
  uses: albertocavalcante/bazelbump-action@v1
  with:
    target-version: "9.0.0"
    fail-on-issues: true
```

**Implementation:**
- [ ] GitHub Action wrapper
- [ ] Machine-readable output (JSON, SARIF)
- [ ] Exit codes (0=ok, 1=issues, 2=error)
- [ ] PR comments with findings

### 3.2 IDE Integration

- [ ] VS Code extension for inline warnings
- [ ] LSP server for .bazelrc files
- [ ] Quick fixes from diagnostics

### 3.3 Recipe Catalog

Community-contributed recipes:

```bash
# Install recipe from catalog
bazelbump recipe install rules-kotlin-migration

# Search catalog
bazelbump recipe search "kotlin"

# Publish your recipe
bazelbump recipe publish ./my-recipe.star
```

**Implementation:**
- [ ] Central recipe registry (GitHub-based initially)
- [ ] Version constraints for recipes
- [ ] Recipe testing framework
- [ ] Recipe signing/verification

---

## Phase 4: Advanced Features

**Goal:** Handle complex migration scenarios.

### 4.1 Multi-Step Migration Plans

For major version jumps (e.g., Bazel 5 → 9):

```bash
bazelbump plan --from 5.0.0 --to 9.0.0

# Output:
# Step 1: Upgrade to 6.0.0 (23 changes)
# Step 2: Upgrade to 7.0.0 (18 changes)
# Step 3: Upgrade to 8.0.0 (31 changes)
# Step 4: Upgrade to 9.0.0 (12 changes)
#
# Total: 84 changes across 4 steps
# Estimated effort: Medium

bazelbump plan apply --step 1
```

### 4.2 Dependency Analysis

Understand rule dependencies before migration:

```bash
bazelbump deps analyze

# Output:
# rules_cc: 0.0.1 → 0.0.9 required for Bazel 9.0
# rules_java: 5.0.0 → 7.0.0 required for Bazel 9.0
# rules_python: OK (compatible)
```

### 4.3 Starlark Rule Migration

Analyze and migrate custom .bzl files:

```starlark
scan(
    id = "custom-rules",
    patterns = ["**/*.bzl"],
    query = ast_query("function_def"),
)

edit(
    id = "update-rules",
    operations = [
        # Update deprecated Starlark APIs
        replace_text("ctx.new_file(", "ctx.actions.declare_file("),
        replace_text("ctx.file_action(", "ctx.actions.write("),
    ],
)
```

### 4.4 Interactive Mode

Guided migration with prompts:

```bash
bazelbump migrate --interactive

# ? Target Bazel version: 9.0.0
# ? Found 3 deprecated flags. Remove them? [Y/n]
# ? Found native rule usage in 12 files. Add load statements? [Y/n]
# ? Run buildifier after changes? [Y/n]
#
# Applying changes...
# ✓ Removed 3 flags from .bazelrc
# ✓ Added load() to 12 BUILD files
# ✓ Formatted with buildifier
#
# Migration complete!
```

---

## Non-Goals

Things we explicitly won't do:

- **Full Starlark analysis** — We won't replicate Bazel's Starlark interpreter
- **Build execution** — We analyze, not build
- **Non-Bazel migrations** — Focus on Bazel ecosystem only
- **Breaking determinism** — Recipes must be reproducible

---

## Contributing

Want to help? Here's how:

1. **Test on your project** — Report issues and edge cases
2. **Write recipes** — Share migrations that helped you
3. **Improve docs** — Fix typos, add examples
4. **Code contributions** — See issues labeled `good first issue`

---

## Timeline

| Phase | Target | Status |
|-------|--------|--------|
| Phase 1 | Q1 2026 | 🔄 In Progress |
| Phase 2 | Q2 2026 | 📋 Planned |
| Phase 3 | Q3 2026 | 📋 Planned |
| Phase 4 | Q4 2026 | 💭 Future |

---

*This roadmap is a living document. Priorities may shift based on community feedback and real-world usage.*
