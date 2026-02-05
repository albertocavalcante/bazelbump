# Next Steps: Building Blocks for Envoy Migration

## Current Capabilities Summary

| Feature | Status | Notes |
|---------|--------|-------|
| AST-aware BUILD transformations | ✅ Done | Multi-line loads handled correctly |
| Syntax validation | ✅ Done | Validates after each modification |
| Pattern detection | ✅ Done | Fast regex-based scanning |
| Recipe system | ✅ Done | Starlark recipes with steps/operations |
| Recipe testing | ✅ Done | `bazelbump recipe test` with skytest |
| BCR queries | ✅ Done | `bcr.latest_stable()`, etc. |
| Exec module | ✅ Done | `exec.run()` for shell commands |
| Flag database | ✅ Done | 0.17.1 to 9.0.0 |
| Checkpoint/resume | ✅ Done | State management |
| **Bisect** | ❌ Missing | Find breaking version |
| **Version bump orchestration** | ❌ Missing | 7→8→9 with testing |
| **Staged rollout** | ❌ Missing | Per-directory migration |
| **Documentation** | ⚠️ Incomplete | Needs examples, guides |

---

## Priority 1: Bisect Command

### Why It's Critical

When migrating Envoy from Bazel 7.x to 9.x:
1. Build breaks somewhere between versions
2. Need to identify exactly which version introduced the breakage
3. Then determine if it's a flag change, API change, or bug

### Proposed CLI

```
bazelbump bisect [flags]

Flags:
  --from VERSION       Known-good Bazel version (required)
  --to VERSION         Known-bad or target version (default: latest)
  --test CMD           Test command (default: "bazel build //...")
  --target TARGET      Build target (default: "//...")
  --auto               Run tests automatically (non-interactive)
  --skip-prerelease    Skip prerelease versions
  --max-steps N        Max bisect iterations (default: 20)
  --use-bazelisk       Use bazelisk for version switching (default: true)
```

### Example Usage

```bash
# Find which version breaks between 7.0.0 and 8.0.0
$ bazelbump bisect --from 7.0.0 --to 8.0.0 --auto

Fetching versions between 7.0.0 and 8.0.0...
Found 15 versions to bisect.

Testing 7.3.0... PASS (bazel build //... exit 0)
Testing 7.5.0... FAIL (exit 1)
Testing 7.4.0... PASS
Testing 7.4.1... FAIL

First bad version: 7.4.1

Changes in 7.4.1:
  - Flag --incompatible_disable_starlark_host_transitions enabled by default
  - Flag --incompatible_no_implicit_file_export enabled by default

Suggested fixes:
  1. Add to .bazelrc: build --noincompatible_disable_starlark_host_transitions
  2. Run: bazelbump analyze --to 7.4.1
```

### Implementation Plan

```go
// cmd/bazelbump/bisect.go
type BisectConfig struct {
    FromVersion     string
    ToVersion       string
    TestCommand     string
    Target          string
    Auto            bool
    SkipPrerelease  bool
    MaxSteps        int
    UseBazelisk     bool
}

type BisectResult struct {
    FirstBadVersion string
    LastGoodVersion string
    StepsTaken      int
    FlagChanges     []FlagChange
    TestLog         string
}

func runBisect(cfg *BisectConfig) (*BisectResult, error) {
    // 1. Get versions between from and to
    versions, err := getVersionRange(cfg.FromVersion, cfg.ToVersion, cfg.SkipPrerelease)

    // 2. Binary search
    bad := len(versions) - 1
    good := 0

    for good < bad {
        mid := (good + bad) / 2
        version := versions[mid]

        result, err := testVersion(version, cfg)
        if result.Pass {
            good = mid + 1
        } else {
            bad = mid
        }
    }

    // 3. Analyze changes in first bad version
    firstBad := versions[bad]
    changes := analyzeFlagChanges(versions[bad-1], firstBad)

    return &BisectResult{
        FirstBadVersion: firstBad,
        LastGoodVersion: versions[bad-1],
        FlagChanges:     changes,
    }, nil
}

func testVersion(version string, cfg *BisectConfig) (*TestResult, error) {
    // Set USE_BAZEL_VERSION for bazelisk
    env := os.Environ()
    env = append(env, "USE_BAZEL_VERSION="+version)

    cmd := exec.Command("bash", "-c", cfg.TestCommand)
    cmd.Env = env
    cmd.Dir = cfg.ProjectPath

    output, err := cmd.CombinedOutput()
    return &TestResult{
        Pass:    err == nil,
        Output:  string(output),
        Version: version,
    }, nil
}
```

### Files to Create

| File | Purpose |
|------|---------|
| `cmd/bazelbump/bisect.go` | CLI command implementation |
| `bisect/bisect.go` | Core bisect algorithm |
| `bisect/versions.go` | Version enumeration from flag DB |
| `bisect/test.go` | Test execution with version switching |

---

## Priority 2: Version Bump Orchestration

### Why It's Needed

Large migrations need intermediate checkpoints:
- 7.0.0 → 7.4.0 → 8.0.0 → 8.2.0 → 9.0.0
- Test at each milestone
- Rollback if broken
- Apply version-specific fixes

### Proposed CLI

```bash
# Show migration path
$ bazelbump migrate plan --from 7.0.0 --to 9.0.0

Migration plan from 7.0.0 to 9.0.0:

Step 1: 7.0.0 → 7.4.0 (last 7.x before breaking changes)
  - 3 flag changes to address
  - Estimated: autoload-migration not yet needed

Step 2: 7.4.0 → 8.0.0 (major version)
  - 12 flag changes
  - Breaking: --incompatible_disallow_empty_glob
  - Requires: autoload-migration recipe

Step 3: 8.0.0 → 9.0.0 (major version)
  - 8 flag changes
  - Requires: explicit loads for all native rules

# Execute migration with testing
$ bazelbump migrate run --from 7.0.0 --to 9.0.0 --test "bazel test //..."
```

### Recipe Integration

```starlark
# recipes/version-bump.star
recipe(
    name = "version-bump",
    version = "1.0.0",
    description = "Orchestrate Bazel version migration",
    requires = ["exec"],
    steps = [
        # Step 1: Detect current version and target
        scan(
            id = "analyze",
            impl = _analyze_migration,
        ),

        # Step 2: Apply version-specific fixes
        edit(
            id = "apply_fixes",
            depends_on = ["analyze"],
            operations = _get_operations,
        ),

        # Step 3: Verify build still works
        verify(
            id = "test",
            depends_on = ["apply_fixes"],
            command = "bazel build //...",
        ),
    ],
)

def _analyze_migration(ctx):
    current = project.context().bazel_version
    target = ctx.args.get("target", "9.0.0")

    # Use flag database to find changes
    changes = flags.diff(current, target)

    return struct(
        current = current,
        target = target,
        changes = changes,
        needs_autoload = version.at_least(target, "8.0.0"),
    )
```

---

## Priority 3: Documentation

### Structure Needed

```
docs/
├── user/
│   ├── getting-started.md          ✅ Exists
│   ├── commands.md                 ✅ Exists
│   ├── recipes.md                  ✅ Exists
│   ├── authoring-recipes.md        ✅ Exists (needs examples)
│   └── recipe-reference.md         ✅ Exists (needs completion)
├── guides/                         ❌ MISSING
│   ├── migrating-major-versions.md # 7→8→9 strategy
│   ├── bisecting-failures.md       # Using bisect effectively
│   ├── large-project-tips.md       # Envoy-scale advice
│   └── ci-integration.md           # GitHub Actions examples
├── cookbook/                       ❌ MISSING
│   ├── common-patterns.md          # Recipe patterns
│   ├── custom-validators.md        # Using exec for validation
│   └── staged-rollout.md           # Per-directory migration
└── reference/
    └── builtins-complete.md        # Every builtin with examples
```

### Key Guides Needed

1. **Major Version Migration Guide**
   - When to use recipes vs manual fixes
   - Understanding flag changes
   - Testing strategy

2. **Bisect Tutorial**
   - Finding breaking changes
   - Interpreting results
   - Common patterns

3. **Large Project Strategy**
   - Checkpoint strategy
   - Parallel testing approach
   - Rollback planning

---

## Implementation Order

### Sprint 1: Bisect (Enables finding problems)
1. [ ] Create `bisect/` package
2. [ ] Implement version enumeration from flag database
3. [ ] Implement binary search with bazelisk
4. [ ] Create `bisect` CLI command
5. [ ] Add flag change analysis for identified version
6. [ ] Write bisect guide

### Sprint 2: Migration Planning (Enables solving problems)
1. [ ] Add `migrate plan` command
2. [ ] Add `migrate run` command with checkpoints
3. [ ] Create version-bump recipe template
4. [ ] Add milestone detection logic
5. [ ] Write migration guide

### Sprint 3: Documentation & Polish
1. [ ] Complete recipe reference
2. [ ] Create cookbook with patterns
3. [ ] Write CI integration guide
4. [ ] Add Envoy-specific guide
5. [ ] Add more example recipes

---

## Quick Wins (Do Now)

These can be done immediately without architectural changes:

1. **Add `--include`/`--exclude` to recipe run** for staged rollout
2. **Add `bazel.versions()` builtin** in recipes to access flag database
3. **Document existing builtins** more thoroughly
4. **Create template recipes** for common rule migrations

---

## Success Criteria for Envoy

Envoy migration is "supported" when:

1. ✅ `ensure_load` works on 100% of BUILD files (DONE)
2. ⬜ `bazelbump bisect` finds breaking version in <10 steps
3. ⬜ Can migrate 7→8→9 with verified checkpoints
4. ⬜ Clear documentation for Envoy-scale projects
5. ⬜ Recipe examples for common patterns

---

*Last updated: 2025-02-05*
