# ADR-006: Agent Scope for v1.0

**Status:** Accepted
**Date:** 2025-02-03
**Deciders:** Alberto Cavalcante

## Context

bazelbump includes AI agent capabilities for assisting with Bazel upgrades. We need to define clear boundaries for v1.0:

- What can the agent do?
- What requires LLM vs deterministic logic?
- What's deferred to future versions?

## Decision

**v1.0 agent scope: "Useful assistance without full autonomy"**

The agent should be able to perform useful tasks beyond simple explanations, but not fully autonomous multi-step planning.

## Capability Tiers

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    AGENT CAPABILITY TIERS (v1.0)                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  TIER 1: DETERMINISTIC (No LLM Required)                      ✅ v1.0      │
│  ═══════════════════════════════════════                                    │
│  These always work, even without LLM access:                                │
│                                                                              │
│  • Detect deprecated flags → auto-generate removal/rename                   │
│  • Detect native rule usage → auto-generate load() statements              │
│  • Detect Starlark API deprecations → auto-generate replacements           │
│  • Detect incompatible bazel_compatibility → suggest version bump          │
│  • Match known error patterns → suggest documented fixes                   │
│  • Apply migration recipes → execute recipe steps                          │
│                                                                              │
│  TIER 2: LLM-ASSISTED ANALYSIS (Requires LLM)                 ✅ v1.0      │
│  ════════════════════════════════════════════                               │
│  LLM helps understand and explain:                                          │
│                                                                              │
│  • Unknown build errors → LLM explains root cause                          │
│  • Complex .bzl files → LLM identifies migration needs                     │
│  • Ruleset incompatibility → LLM suggests workarounds                      │
│  • Custom macros → LLM explains what needs to change                       │
│  • Stack traces → LLM translates to actionable guidance                    │
│                                                                              │
│  TIER 3: LLM-ASSISTED GENERATION (Requires LLM)               ✅ v1.0      │
│  ═══════════════════════════════════════════════                            │
│  LLM generates artifacts:                                                   │
│                                                                              │
│  • Generate patches for .bzl files based on deprecation                    │
│  • Generate MODULE.bazel changes for dependency updates                    │
│  • Generate .bazelrc migrations with explanatory comments                  │
│  • Draft bug reports (save locally for later submission)                   │
│  • Generate migration documentation (changelog, notes)                     │
│                                                                              │
│  NOT IN v1.0:                                                    ❌ Future  │
│  ═══════════════                                                            │
│  • Multi-turn conversations with memory                                     │
│  • Autonomous multi-step planning ("figure out how to upgrade")            │
│  • Self-correcting loops (try → fail → adjust → retry)                     │
│  • External tool calling (MCP, web search, etc.)                           │
│  • Learning from past upgrades                                              │
│  • Cross-repository reasoning                                               │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Command Mapping

| Command | Tier | LLM Required | Description |
|---------|------|--------------|-------------|
| `bazelbump analyze` | 1 | No | Deterministic analysis |
| `bazelbump apply` | 1 | No | Apply deterministic transforms |
| `bazelbump diagnose` | 2 | Yes | Explain errors with LLM |
| `bazelbump patch` | 3 | Yes | Generate patches with LLM |
| `bazelbump explain` | 2 | Yes | Explain changes/deprecations |
| `bazelbump document` | 3 | Yes | Generate migration docs |
| `bazelbump draft-bug` | 3 | Yes | Draft bug report |

## Detailed Capabilities

### Tier 1: Deterministic (No LLM)

```bash
# Flag migration
$ bazelbump fix flags
Migrating 5 deprecated flags:
  ✓ --experimental_enable_bzlmod → removed (now default)
  ✓ --incompatible_strict_action_env → removed (now default)
  ✓ --host_copt → --copt (renamed)

# Native rule imports
$ bazelbump fix imports
Adding load() statements to 12 BUILD files:
  ✓ //src:BUILD - added cc_library, cc_binary from @rules_cc
  ✓ //lib:BUILD - added java_library from @rules_java

# Starlark API updates
$ bazelbump fix starlark
Updating 3 deprecated APIs:
  ✓ tools/macros.bzl:45 - native.existing_rule → native.existing_rules().get
  ✓ tools/rules.bzl:23 - cfg = "host" → cfg = "exec"
```

### Tier 2: LLM Analysis

```bash
$ bazelbump diagnose --error-file build_error.log

DIAGNOSIS (via Claude 3.5 Sonnet)
==================================

Error: "no such package '@rules_foo//lib': BUILD file not found"

Root Cause Analysis:
  The rules_foo repository uses a non-standard directory structure
  that changed in version 2.0. The 'lib' directory was moved to 'src/lib'.

  This is a breaking change introduced in rules_foo 2.0.0 that affects
  users who directly reference internal paths.

Confidence: HIGH (based on rules_foo changelog)

Suggested Fix:
  Update your load() statements:
    - load("@rules_foo//lib:defs.bzl", "foo_library")
    + load("@rules_foo//src/lib:defs.bzl", "foo_library")

  Or use the compatibility layer:
    load("@rules_foo//:compat.bzl", "foo_library")

Related:
  - rules_foo 2.0 migration guide: https://...
  - Issue #1234: Breaking change in 2.0
```

### Tier 3: LLM Generation

```bash
$ bazelbump patch tools/macros.bzl --for 9.0.0 --output patch.diff

Analyzing tools/macros.bzl for Bazel 9.0.0 compatibility...

Generated Patch:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--- a/tools/macros.bzl
+++ b/tools/macros.bzl
@@ -43,8 +43,12 @@ def check_target(name):
-    rule = native.existing_rule(name)
-    if rule:
+    # MIGRATION: native.existing_rule() removed in Bazel 9.0
+    # Use native.existing_rules() and look up by name instead.
+    # See: https://bazel.build/rules/lib/globals#native.existing_rules
+    rules = native.existing_rules()
+    rule = rules.get(name)
+    if rule is not None:
         return rule.get("srcs", [])

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Generation Stats:
  Model: claude-3-5-sonnet (via Bedrock)
  Tokens: 1,234 input / 456 output
  Confidence: HIGH

Saved to: patch.diff

[A]pply  [E]dit  [R]egenerate  [C]ancel
```

## Graceful Degradation

When LLM is unavailable, bazelbump still works:

```bash
$ bazelbump diagnose --error-file build_error.log

WARNING: LLM provider unavailable (Bedrock connection failed)
Falling back to pattern matching...

DIAGNOSIS (Deterministic Mode)
==============================

Error matched known pattern: "no such package"

Possible causes:
  1. Repository not declared in MODULE.bazel or WORKSPACE
  2. Repository path changed in newer version
  3. Typo in repository name

Suggested actions:
  - Check MODULE.bazel for the dependency declaration
  - Verify the repository structure in BCR
  - Run 'bazel mod graph' to see resolved dependencies

Note: For detailed analysis, configure LLM provider:
  bazelbump config set llm.provider bedrock
```

## LLM Prompt Engineering

We use structured prompts optimized for code tasks:

```go
// internal/agent/prompts/diagnose.go
var DiagnoseErrorPrompt = `You are an expert Bazel build engineer helping diagnose build errors.

CONTEXT:
- Current Bazel version: {{.CurrentVersion}}
- Target Bazel version: {{.TargetVersion}}
- Project uses: {{.ProjectFeatures}}

ERROR LOG:
{{.ErrorLog}}

TASK:
1. Identify the root cause of this error
2. Explain why it's happening (especially if related to version upgrade)
3. Provide specific, actionable fix suggestions
4. Rate your confidence (LOW/MEDIUM/HIGH)

FORMAT your response as:
## Root Cause
[explanation]

## Why This Happens
[context about Bazel changes if relevant]

## Suggested Fix
[specific code changes or commands]

## Confidence
[LOW/MEDIUM/HIGH] - [brief justification]
`
```

## Consequences

### Positive

- Clear boundaries for v1.0 scope
- Useful without full autonomy
- Graceful degradation without LLM
- Deterministic core always works

### Negative

- Not fully autonomous
- Requires user judgment for LLM suggestions
- Limited to single-step operations

### Future Expansion (v2.0+)

- Multi-turn conversations
- Autonomous planning
- Self-correction loops
- Tool use (MCP)
- Learning from history

## References

- AWS Bedrock Claude: https://docs.anthropic.com/en/api/claude-on-amazon-bedrock
- Prompt engineering: https://docs.anthropic.com/en/docs/build-with-claude/prompt-engineering
