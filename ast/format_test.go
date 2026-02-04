// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"
	"testing"
)

// TestFormat_StringEscaping tests that string escape sequences are preserved.
// This was a bug found in Envoy: strings like "touch \"$(@D)/file\"" were
// being corrupted to "touch "$(@D)/file"" (losing the backslash escapes).
//
// Bug discovered during Envoy validation - the formatter was outputting
// StringExpr.Value (decoded) instead of StringExpr.Token (original with escapes).
//
// Envoy reference: commit 772676aa95f21d617b9c43b5b723eed03c9025e5
func TestFormat_StringEscaping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string // substrings that must be in output
	}{
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/bazel/external/BUILD#L25
			// Original: cmd = "touch \"$(@D)/empty.cc\""
			name: "escaped quotes in genrule cmd",
			input: `genrule(
    name = "test",
    cmd = "touch \"$(@D)/empty.cc\"",
)`,
			contains: []string{`"touch \"$(@D)/empty.cc\""`},
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/bazel/external/BUILD#L61
			// Original: cmd = "mkdir -p $$(dirname $@) && cp $< $@"
			name: "shell variables with dollar sign",
			input: `genrule(
    name = "test",
    cmd = "mkdir -p $$(dirname $@) && cp $< $@",
)`,
			contains: []string{`$$`, `$@`, `$<`},
		},
		{
			name: "backslash in path",
			input: `cc_library(
    name = "test",
    copts = ["-I/path\\with\\backslash"],
)`,
			contains: []string{`\\`},
		},
		{
			name: "newline escape in string",
			input: `genrule(
    name = "test",
    cmd = "echo 'line1\nline2'",
)`,
			contains: []string{`\n`},
		},
		{
			name: "tab escape in string",
			input: `genrule(
    name = "test",
    cmd = "echo 'col1\tcol2'",
)`,
			contains: []string{`\t`},
		},
		{
			name: "multiple escapes combined",
			input: `genrule(
    name = "test",
    cmd = "echo \"path\\to\\file\"\necho done",
)`,
			contains: []string{`\\`, `\n`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			output := string(f.Format())

			// Verify output is valid Starlark (round-trip test)
			_, err = ParseFileContent("BUILD", []byte(output))
			if err != nil {
				t.Errorf("Format() produced invalid Starlark: %v\nOutput:\n%s", err, output)
			}

			// Verify required substrings are present
			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("Format() output missing %q\nOutput:\n%s", want, output)
				}
			}
		})
	}
}

// TestFormat_ParenExpr tests parenthesized expressions.
// This was a bug found in Envoy: expressions like (len(X) + len(Y)) were
// being output as <*bzl.ParenExpr> instead of the actual expression.
//
// The formatter was missing a case for bzl.ParenExpr, causing it to fall
// through to the default case which outputs the type name.
//
// Envoy reference: commit 772676aa95f21d617b9c43b5b723eed03c9025e5
func TestFormat_ParenExpr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/tools/code/BUILD#L13-L16
			// Original:
			//   FUZZ_FILTER_COUNT = (
			//       len(READFILTER_FUZZ_FILTERS) +
			//       len(READFILTER_NOFUZZ_FILTERS)
			//   )
			name: "simple paren expression",
			input: `TOTAL = (
    len(LIST_A) +
    len(LIST_B)
)`,
			contains: []string{"(", "len(LIST_A)", "len(LIST_B)", ")"},
		},
		{
			name: "paren in arithmetic",
			input: `VALUE = (10 + 5) * 2`,
			contains: []string{"(10 + 5)", "* 2"},
		},
		{
			name: "nested parens",
			input: `RESULT = ((a + b) * (c + d))`,
			contains: []string{"((a + b) * (c + d))"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			output := string(f.Format())

			// Must not contain the fallback <*bzl.ParenExpr>
			if strings.Contains(output, "<*bzl.") {
				t.Errorf("Format() output contains unformatted AST node\nOutput:\n%s", output)
			}

			// Verify output is valid Starlark
			_, err = ParseFileContent("BUILD", []byte(output))
			if err != nil {
				t.Errorf("Format() produced invalid Starlark: %v\nOutput:\n%s", err, output)
			}

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("Format() output missing %q\nOutput:\n%s", want, output)
				}
			}
		})
	}
}

// TestFormat_TupleExpr tests tuple expressions.
// This was a bug found in Envoy: tuples like (FUZZ_COUNT, PATH) were
// being output as <*bzl.TupleExpr> instead of the actual tuple.
//
// The formatter was missing a case for bzl.TupleExpr.
//
// Envoy reference: commit 772676aa95f21d617b9c43b5b723eed03c9025e5
func TestFormat_TupleExpr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/tools/code/BUILD#L70-L73
			// Original:
			//   """ % (
			//       FUZZ_FILTER_COUNT,
			//       PATH,
			//   ),
			name:     "tuple in string format",
			input:    `MSG = "Values: %s, %s" % (VALUE_A, VALUE_B)`,
			contains: []string{"(VALUE_A, VALUE_B)"},
		},
		{
			name:     "tuple assignment",
			input:    `(a, b) = get_values()`,
			contains: []string{"a", "b", "get_values()"},
		},
		{
			name:     "tuple in function call",
			input:    `func(args = (1, 2, 3))`,
			contains: []string{"(1, 2, 3)"},
		},
		{
			name:     "empty tuple",
			input:    `EMPTY = ()`,
			contains: []string{"()"},
		},
		{
			name:     "single element tuple",
			input:    `SINGLE = (42,)`,
			contains: []string{"42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			output := string(f.Format())

			// Must not contain the fallback
			if strings.Contains(output, "<*bzl.") {
				t.Errorf("Format() output contains unformatted AST node\nOutput:\n%s", output)
			}

			// Verify output is valid Starlark
			_, err = ParseFileContent("BUILD", []byte(output))
			if err != nil {
				t.Errorf("Format() produced invalid Starlark: %v\nOutput:\n%s", err, output)
			}

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("Format() output missing %q\nOutput:\n%s", want, output)
				}
			}
		})
	}
}

// TestFormat_Comprehension tests list and dict comprehensions.
// This was a bug found in Envoy: comprehensions like [x for x in items]
// were being output as <*bzl.Comprehension> instead of the actual comprehension.
//
// The formatter was missing cases for bzl.Comprehension, bzl.ForClause, and
// bzl.IfClause. Envoy uses comprehensions to generate multiple similar targets.
//
// Envoy reference: commit 772676aa95f21d617b9c43b5b723eed03c9025e5
func TestFormat_Comprehension(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "simple list comprehension",
			input:    `DOUBLED = [x * 2 for x in ITEMS]`,
			contains: []string{"[", "x * 2", "for x in ITEMS", "]"},
		},
		{
			name:     "list comprehension with if",
			input:    `FILTERED = [x for x in ITEMS if x > 0]`,
			contains: []string{"for x in ITEMS", "if x > 0"},
		},
		{
			name:     "dict comprehension",
			input:    `MAPPING = {k: v for k, v in PAIRS}`,
			contains: []string{"{", "k: v", "for k, v in PAIRS", "}"},
		},
		{
			name:     "nested comprehension",
			input:    `FLAT = [y for x in MATRIX for y in x]`,
			contains: []string{"for x in MATRIX", "for y in x"},
		},
		{
			name:     "comprehension with function call",
			input:    `NAMES = [f.name for f in get_files()]`,
			contains: []string{"f.name", "for f in get_files()"},
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/test/common/http/BUILD#L104-L131
			// Original: Generates fuzz tests for http1/http2 protocols using comprehension
			name: "envoy-style fuzz test comprehension",
			input: `[
    envoy_cc_fuzz_test(
        name = name + "_fuzz_test",
        corpus = name + "_corpus",
    )
    for name in FUZZ_TESTS
]`,
			contains: []string{"envoy_cc_fuzz_test", "for name in FUZZ_TESTS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			output := string(f.Format())

			// Must not contain the fallback
			if strings.Contains(output, "<*bzl.") {
				t.Errorf("Format() output contains unformatted AST node\nOutput:\n%s", output)
			}

			// Verify output is valid Starlark
			_, err = ParseFileContent("BUILD", []byte(output))
			if err != nil {
				t.Errorf("Format() produced invalid Starlark: %v\nOutput:\n%s", err, output)
			}

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("Format() output missing %q\nOutput:\n%s", want, output)
				}
			}
		})
	}
}

// TestFormat_RoundTrip tests that parse -> format -> parse produces valid output.
// These are real-world patterns from Envoy that previously caused issues.
//
// Each test case is derived from actual Envoy BUILD files that failed during
// AST transformation validation.
//
// Envoy reference: commit 772676aa95f21d617b9c43b5b723eed03c9025e5
func TestFormat_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/bazel/external/BUILD#L22-L27
			name: "genrule with shell escapes",
			input: `genrule(
    name = "empty_cc",
    outs = ["empty.cc"],
    cmd = "touch \"$(@D)/empty.cc\"",
    visibility = ["//visibility:public"],
)`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/tools/code/BUILD#L50-L79
			// Simplified from the checked genrule that uses triple-quoted heredoc cmd
			name: "genrule with heredoc-style cmd",
			input: `genrule(
    name = "checked",
    outs = ["checked.txt"],
    cmd = """
    $(location :check) \\
        --fix \\
        -l warn \\
      > $@ 2>&1 || :
    """,
)`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/tools/code/BUILD#L13-L16
			name: "arithmetic with parens",
			input: `FUZZ_FILTER_COUNT = (
    len(READFILTER_FUZZ_FILTERS) +
    len(READFILTER_NOFUZZ_FILTERS)
)`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/tools/code/BUILD#L35-L36
			name: "string format with tuple",
			input: `py_console_script_binary(
    name = "check",
    args = [
        "--extensions_fuzzed_count=%s" % FUZZ_FILTER_COUNT,
        "--path=%s" % PATH,
    ],
)`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/test/common/http/BUILD#L104-L131
			name: "list comprehension generating targets",
			input: `[
    envoy_cc_fuzz_test(
        name = protocol + "_codec_impl_fuzz_test",
        corpus = protocol + "_codec_impl_corpus",
        deps = [
            ":codec_impl_fuzz_proto_cc_proto",
            "//source/common/http:conn_manager_lib",
        ],
    )
    for protocol in [
        "http1",
        "http2",
    ]
]`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/bazel/external/BUILD#L33-L44
			name: "select with dict",
			input: `cc_library(
    name = "gperftools",
    deps = select({
        "//bazel:debug_tcmalloc": [
            "@gperftools//:stacktrace",
            "@gperftools//:tcmalloc_debug",
        ],
        "//conditions:default": [
            "@gperftools//:cpu_profiler",
            "@gperftools//:tcmalloc",
        ],
    }),
)`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/test/common/http/BUILD#L1-L11
			// This is the CRITICAL pattern that broke 96% of Envoy files with text-based approach
			name: "multi-line load with many symbols",
			input: `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_benchmark_test",
    "envoy_cc_benchmark_binary",
    "envoy_cc_fuzz_test",
    "envoy_cc_test",
    "envoy_cc_test_library",
    "envoy_package",
    "envoy_proto_library",
    "envoy_select_enable_http3",
)`,
		},
		{
			// Source: https://github.com/envoyproxy/envoy/blob/772676aa95f21d617b9c43b5b723eed03c9025e5/bazel/external/BUILD#L58-L64
			name: "complex genrule with multiple escapes",
			input: `genrule(
    name = "numa_archive",
    srcs = [":numa_static"],
    outs = ["lib/libnuma.a"],
    cmd = "mkdir -p $$(dirname $@) && cp $< $@",
    target_compatible_with = ["@platforms//os:linux"],
    visibility = ["//visibility:public"],
)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse original
			f1, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent(original) error = %v", err)
			}

			// Format
			output := f1.Format()

			// Parse formatted output
			f2, err := ParseFileContent("BUILD", output)
			if err != nil {
				t.Fatalf("ParseFileContent(formatted) error = %v\nFormatted output:\n%s", err, string(output))
			}

			// Format again (should be stable)
			output2 := f2.Format()

			// Second format should equal first (idempotent)
			if string(output) != string(output2) {
				t.Errorf("Format() is not idempotent:\nFirst:\n%s\nSecond:\n%s", string(output), string(output2))
			}

			// Must not contain unformatted AST nodes
			if strings.Contains(string(output), "<*bzl.") {
				t.Errorf("Format() output contains unformatted AST node:\n%s", string(output))
			}
		})
	}
}

// TestFormat_NoFallbackNodes ensures we don't output <*bzl.Type> for any expression.
func TestFormat_NoFallbackNodes(t *testing.T) {
	// A comprehensive BUILD file that exercises many node types
	input := `load("@rules_cc//cc:defs.bzl", "cc_library", "cc_test")
load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)

licenses(["notice"])

CONSTANT = 42
STRING = "hello"
LIST = [1, 2, 3]
DICT = {"a": 1, "b": 2}
TUPLE_VAL = (1, 2, 3)
PAREN_EXPR = (1 + 2)
COMPREHENSION = [x * 2 for x in LIST if x > 1]
DICT_COMP = {k: v for k, v in DICT.items()}
BINARY = 1 + 2
UNARY = -1
DOT = foo.bar
INDEX = list[0]
CALL = func(arg1, arg2)
FORMAT = "value: %s" % (VALUE,)

cc_library(
    name = "test",
    srcs = ["test.cc"],
    hdrs = ["test.h"],
    deps = select({
        "//conditions:default": [],
        ":special": ["//special:lib"],
    }),
    copts = ["-DFOO=\"bar\""],
)

genrule(
    name = "gen",
    outs = ["out.txt"],
    cmd = "echo \"$(@D)\" > $@",
)

[
    cc_test(
        name = name + "_test",
        srcs = [name + "_test.cc"],
    )
    for name in ["foo", "bar"]
]
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	output := string(f.Format())

	// Must not contain any unformatted AST nodes
	if strings.Contains(output, "<*bzl.") {
		t.Errorf("Format() output contains unformatted AST node(s):\n%s", output)
	}

	// Must be valid Starlark
	_, err = ParseFileContent("BUILD", []byte(output))
	if err != nil {
		t.Errorf("Format() produced invalid Starlark: %v\nOutput:\n%s", err, output)
	}
}

// TestFormat_EnsureLoadPreservesContent tests that EnsureLoad + Format
// preserves the original file content (except for the added load).
//
// This is the integration test that validates the original bug is fixed:
// The text-based ensure_load implementation was inserting new load statements
// INSIDE multi-line load statements, corrupting 96% of Envoy BUILD files.
//
// Example of the bug (text-based):
//   load(                           <-- Text-based found "load(" here
//   load("@rules_cc//...")          <-- And inserted new load INSIDE!
//       "//bazel:envoy_build_system.bzl",
//       ...
//   )
//
// The AST-based approach correctly identifies statement boundaries.
//
// Envoy reference: commit 772676aa95f21d617b9c43b5b723eed03c9025e5
func TestFormat_EnsureLoadPreservesContent(t *testing.T) {
	input := `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)

licenses(["notice"])  # Apache 2

envoy_package()

envoy_cc_library(
    name = "filter_test",
    srcs = ["filter_test.cc"],
    copts = ["-DTEST=\"value\""],
)

genrule(
    name = "gen",
    cmd = "touch \"$(@D)/out\"",
    outs = ["out"],
)
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	// Add a new load
	_, err = EnsureLoad(f, EnsureLoadOptions{
		Module:   "@rules_cc//cc:defs.bzl",
		Symbols:  []string{"cc_test"},
		Position: LoadPositionTop,
	})
	if err != nil {
		t.Fatalf("EnsureLoad() error = %v", err)
	}

	output := string(f.Format())

	// Verify output is valid
	_, err = ParseFileContent("BUILD", []byte(output))
	if err != nil {
		t.Fatalf("Format() produced invalid Starlark: %v\nOutput:\n%s", err, output)
	}

	// Verify original content is preserved
	mustContain := []string{
		"envoy_build_system.bzl",
		"envoy_cc_library",
		"envoy_package",
		`licenses(["notice"])`,
		`name = "filter_test"`,
		`-DTEST=\"value\"`, // Escape must be preserved
		`$(@D)/out`,        // Shell variable must be preserved
	}

	for _, want := range mustContain {
		if !strings.Contains(output, want) {
			t.Errorf("Format() output missing %q\nOutput:\n%s", want, output)
		}
	}

	// Verify the new load was added
	if !strings.Contains(output, "@rules_cc//cc:defs.bzl") {
		t.Errorf("New load not added\nOutput:\n%s", output)
	}
}

// TestEscapeString tests the escapeString helper function directly.
func TestEscapeString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		triple bool
		want   string
	}{
		{
			name:   "simple string",
			input:  "hello",
			triple: false,
			want:   "hello",
		},
		{
			name:   "backslash",
			input:  `path\to\file`,
			triple: false,
			want:   `path\\to\\file`,
		},
		{
			name:   "double quote",
			input:  `say "hello"`,
			triple: false,
			want:   `say \"hello\"`,
		},
		{
			name:   "newline",
			input:  "line1\nline2",
			triple: false,
			want:   `line1\nline2`,
		},
		{
			name:   "tab",
			input:  "col1\tcol2",
			triple: false,
			want:   `col1\tcol2`,
		},
		{
			name:   "carriage return",
			input:  "line1\rline2",
			triple: false,
			want:   `line1\rline2`,
		},
		{
			name:   "triple quote preserves newline",
			input:  "line1\nline2",
			triple: true,
			want:   "line1\nline2",
		},
		{
			name:   "triple quote preserves double quote",
			input:  `say "hello"`,
			triple: true,
			want:   `say "hello"`,
		},
		{
			name:   "complex shell command",
			input:  `touch "$(@D)/empty.cc"`,
			triple: false,
			want:   `touch \"$(@D)/empty.cc\"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeString(tt.input, tt.triple)
			if got != tt.want {
				t.Errorf("escapeString(%q, %v) = %q, want %q", tt.input, tt.triple, got, tt.want)
			}
		})
	}
}
