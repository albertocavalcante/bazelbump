// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"testing"
)

func TestRegex_Match(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.match - matches at the beginning of string
result = re.match(r"hello", "hello world")

recipe(
    name = "regex-match-test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-match-test" {
		t.Errorf("expected name 'regex-match-test', got %q", recipe.Name)
	}
}

func TestRegex_MatchNoMatch(t *testing.T) {
	interp := NewInterpreter()

	// re.match returns an empty list when no match at beginning
	// We test this by using the result in a conditional expression
	content := `
result = re.match(r"^world", "hello world")

# result should be an empty list since "world" is not at the beginning
# Use conditional expression to verify
version = "1.0.0" if len(result) == 0 else fail("expected empty list for non-matching pattern")

recipe(
    name = "regex-match-none-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-match-none-test" {
		t.Errorf("expected name 'regex-match-none-test', got %q", recipe.Name)
	}
}

func TestRegex_Findall(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.findall - finds all matches
text = "cat bat rat"
matches = re.findall(r"[cbr]at", text)

# Verify we found all matches using conditional expression
version = "1.0.0" if len(matches) == 3 else fail("expected 3 matches, got " + str(len(matches)))

recipe(
    name = "regex-findall-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-findall-test" {
		t.Errorf("expected name 'regex-findall-test', got %q", recipe.Name)
	}
}

func TestRegex_FindallEmpty(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.findall with no matches
text = "hello world"
matches = re.findall(r"xyz", text)

# Verify empty list returned
version = "1.0.0" if len(matches) == 0 else fail("expected 0 matches, got " + str(len(matches)))

recipe(
    name = "regex-findall-empty-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-findall-empty-test" {
		t.Errorf("expected name 'regex-findall-empty-test', got %q", recipe.Name)
	}
}

func TestRegex_Sub(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.sub - substitutes matches
text = "cat bat rat"
result = re.sub(r"[cbr]at", "dog", text)

# Verify substitution worked
version = "1.0.0" if result == "dog dog dog" else fail("expected 'dog dog dog', got '" + result + "'")

recipe(
    name = "regex-sub-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-sub-test" {
		t.Errorf("expected name 'regex-sub-test', got %q", recipe.Name)
	}
}

func TestRegex_SubPartial(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.sub with partial matches
text = "hello123world456"
result = re.sub(r"\d+", "NUM", text)

# Verify substitution worked
version = "1.0.0" if result == "helloNUMworldNUM" else fail("expected 'helloNUMworldNUM', got '" + result + "'")

recipe(
    name = "regex-sub-partial-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-sub-partial-test" {
		t.Errorf("expected name 'regex-sub-partial-test', got %q", recipe.Name)
	}
}

func TestRegex_Split(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.split - splits string by pattern
text = "one1two2three3four"
parts = re.split(r"\d", text)

# Verify split worked - use conditional expression to check count and values
valid_len = len(parts) == 4
valid_first = parts[0] == "one"
valid_last = parts[3] == "four"

version = "1.0.0" if (valid_len and valid_first and valid_last) else fail("split failed: got " + str(parts))

recipe(
    name = "regex-split-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-split-test" {
		t.Errorf("expected name 'regex-split-test', got %q", recipe.Name)
	}
}

func TestRegex_Search(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.search - searches for pattern anywhere in string
text = "hello world"
result = re.search(r"world", text)

# Verify search found the pattern (returns match indices, not None)
version = "1.0.0" if result != None else fail("expected match, got None")

recipe(
    name = "regex-search-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-search-test" {
		t.Errorf("expected name 'regex-search-test', got %q", recipe.Name)
	}
}

func TestRegex_Compile(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Test re.compile - compiles pattern for reuse
pattern = re.compile(r"\d+")
text = "abc123def456"
matches = pattern.findall(text)

# Verify compiled pattern works
version = "1.0.0" if len(matches) == 2 else fail("expected 2 matches, got " + str(len(matches)))

recipe(
    name = "regex-compile-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-compile-test" {
		t.Errorf("expected name 'regex-compile-test', got %q", recipe.Name)
	}
}

func TestRegex_BazelPatternMatching(t *testing.T) {
	// Test realistic Bazel pattern matching use case
	interp := NewInterpreter()

	content := `
# Test regex for matching Bazel rule patterns
build_content = """
cc_binary(
    name = "hello",
    srcs = ["hello.cc"],
)

cc_library(
    name = "lib",
    srcs = ["lib.cc"],
)
"""

# Find all cc_* rules
cc_rules = re.findall(r"cc_\w+\s*\(", build_content)
valid_cc_rules = len(cc_rules) == 2

# Extract rule names
names = re.findall(r'name\s*=\s*"([^"]+)"', build_content)
valid_names = len(names) == 2

version = "1.0.0" if (valid_cc_rules and valid_names) else fail("pattern matching failed")

recipe(
    name = "regex-bazel-test",
    version = version,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "regex-bazel-test" {
		t.Errorf("expected name 'regex-bazel-test', got %q", recipe.Name)
	}
}
