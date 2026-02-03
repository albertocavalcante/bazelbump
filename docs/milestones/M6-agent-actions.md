# Milestone 6: Agent Actions

**Target:** Weeks 10-11
**Status:** Not Started
**Depends On:** M5 (LLM Integration)

## Objective

Implement LLM-assisted agent capabilities: diagnose, patch, explain, document.

## Deliverables

### 1. Diagnose Command

```bash
$ bazelbump diagnose --error-file build_error.log

DIAGNOSIS
=========

Error: "no such package '@rules_foo//lib'"

Root Cause (via Claude 3.5 Sonnet):
  The rules_foo module changed its directory structure in v2.0.
  The 'lib' path no longer exists - it was moved to 'src/lib'.

Confidence: HIGH

Suggested Fix:
  Update load statements in affected files:
  - load("@rules_foo//lib:defs.bzl", ...)
  + load("@rules_foo//src/lib:defs.bzl", ...)

Affected Files:
  - //src:BUILD (line 3)
  - //tools:BUILD (line 7)

[A]pply fix  [S]how diff  [I]gnore
```

### 2. Patch Command

```bash
$ bazelbump patch tools/macros.bzl --for 9.0.0

Analyzing for Bazel 9.0.0 compatibility...

Generated Patch:
--- a/tools/macros.bzl
+++ b/tools/macros.bzl
@@ -43,8 +43,10 @@
-    rule = native.existing_rule(name)
+    # Updated for Bazel 9.0: native.existing_rule() removed
+    rules = native.existing_rules()
+    rule = rules.get(name)

Generation: claude-3-5-sonnet | Confidence: HIGH

[A]pply  [E]dit  [R]egenerate  [C]ancel
```

### 3. Explain Command

```bash
$ bazelbump explain --flag incompatible_enable_bzlmod

FLAG: --incompatible_enable_bzlmod

Timeline:
  - Bazel 6.0: Introduced as experimental
  - Bazel 7.0: Enabled by default (with flag to disable)
  - Bazel 8.0: Flag deprecated
  - Bazel 9.0: Flag REMOVED (bzlmod always enabled)

What it does:
  Enables the new Bazel Module system (bzlmod) which replaces
  WORKSPACE-based external dependency management.

Impact on your project:
  Your .bazelrc has --noincompatible_enable_bzlmod to disable bzlmod.
  In Bazel 9.0, this flag no longer exists and bzlmod is always on.

Migration steps:
  1. Remove --noincompatible_enable_bzlmod from .bazelrc
  2. Create MODULE.bazel if not exists
  3. Migrate WORKSPACE deps to bazel_dep() calls
  4. Run 'bazel mod tidy' to generate lockfile

References:
  - https://bazel.build/external/migration
  - https://bazel.build/external/module
```

### 4. Document Command

```bash
$ bazelbump document --output MIGRATION.md

Generated: MIGRATION.md

# Bazel 8.0.0 → 9.0.0 Migration Notes

## Summary
Migrated on 2025-02-03 by bazelbump v1.0.0

## Changes Made

### Flag Migrations (3)
| Flag | Action | Reason |
|------|--------|--------|
| --experimental_enable_bzlmod | Removed | Now default |
| --host_copt | Renamed | Use --copt |

### Starlark API Updates (2)
- `tools/macros.bzl:45` - Updated native.existing_rule()
- `build/rules.bzl:23` - Updated cfg="host" to cfg="exec"

### Load Statement Additions (5)
- Added @rules_cc loads to 5 BUILD files

## Verification
- Build: PASSED
- Tests: PASSED (3 skipped - see known_issues.yaml)

## Rollback
If issues occur:
```
bazelbump rollback snap_01HQ6Z
```
```

### 5. Agent Engine

```go
// pkg/agent/engine.go
type Engine struct {
    llm       llm.Provider
    datastore *datastore.DataStore
    prompts   *PromptLibrary
}

type DiagnoseRequest struct {
    ErrorLog      string
    CurrentVersion string
    TargetVersion  string
    ProjectContext *ProjectContext
}

type DiagnoseResponse struct {
    RootCause     string
    Explanation   string
    SuggestedFix  *SuggestedFix
    Confidence    Confidence
    References    []string
}

func (e *Engine) Diagnose(ctx context.Context, req *DiagnoseRequest) (*DiagnoseResponse, error)
func (e *Engine) GeneratePatch(ctx context.Context, req *PatchRequest) (*PatchResponse, error)
func (e *Engine) Explain(ctx context.Context, req *ExplainRequest) (*ExplainResponse, error)
func (e *Engine) Document(ctx context.Context, req *DocumentRequest) (*DocumentResponse, error)
```

### 6. Prompt Library

```go
// pkg/agent/prompts/prompts.go
var DiagnosePrompt = `You are an expert Bazel build engineer...

CONTEXT:
- Current Bazel version: {{.CurrentVersion}}
- Target Bazel version: {{.TargetVersion}}

ERROR:
{{.ErrorLog}}

Analyze and provide:
1. Root cause
2. Why this happens
3. Specific fix
4. Confidence level (LOW/MEDIUM/HIGH)
`

var PatchPrompt = `You are an expert at migrating Bazel/Starlark code...`
var ExplainPrompt = `You are a Bazel documentation expert...`
var DocumentPrompt = `Generate migration documentation...`
```

## Package Structure

```
pkg/
├── agent/
│   ├── engine.go
│   ├── engine_test.go
│   ├── diagnose.go
│   ├── patch.go
│   ├── explain.go
│   ├── document.go
│   └── prompts/
│       ├── prompts.go
│       ├── diagnose.go
│       ├── patch.go
│       └── document.go

cmd/bazelbump/
├── diagnose.go
├── patch.go
├── explain.go
└── document.go
```

## Tasks

- [ ] Implement agent engine
- [ ] Create prompt library
- [ ] Implement diagnose action
- [ ] Implement patch generation
- [ ] Implement explain action
- [ ] Implement document generation
- [ ] Add confidence scoring
- [ ] Add interactive mode (accept/edit/regenerate)
- [ ] Handle LLM unavailable gracefully
- [ ] Add tests with mocked LLM
- [ ] Document prompts and customization

## Acceptance Criteria

1. Diagnose provides useful explanations for errors
2. Patch generation produces valid Starlark code
3. Explain gives comprehensive flag/API information
4. Document creates useful migration notes
5. Graceful fallback when LLM unavailable
6. Interactive mode allows editing before apply

## Notes

- Prompts are critical - iterate on them
- Log LLM interactions for debugging
- Consider caching repeated queries
- Allow prompt customization via config
