// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"strings"
	"testing"
)

func TestVersionMatches(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "simple range match",
			script: `
def check(ctx):
    if not version_matches("7.0.0", [">=6.0.0", "<=8.0.0"]):
        fail(msg="7.0.0 should match range")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "simple range no match below",
			script: `
def check(ctx):
    if version_matches("5.0.0", [">=6.0.0", "<=8.0.0"]):
        fail(msg="5.0.0 should not match range")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "simple range no match above",
			script: `
def check(ctx):
    if version_matches("9.0.0", [">=6.0.0", "<=8.0.0"]):
        fail(msg="9.0.0 should not match range")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "BCR exclude",
			script: `
def check(ctx):
    if version_matches("7.1.0", [">=6.0.0", "-7.1.0"]):
        fail(msg="7.1.0 should be excluded")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "BCR exclude different version passes",
			script: `
def check(ctx):
    if not version_matches("7.2.0", [">=6.0.0", "-7.1.0"]):
        fail(msg="7.2.0 should pass (not excluded)")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "empty constraints match all",
			script: `
def check(ctx):
    if not version_matches("7.0.0", []):
        fail(msg="empty constraints should match all")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "boundary inclusive >=",
			script: `
def check(ctx):
    if not version_matches("7.0.0", [">=7.0.0"]):
        fail(msg="7.0.0 should match >=7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "boundary inclusive <=",
			script: `
def check(ctx):
    if not version_matches("7.0.0", ["<=7.0.0"]):
        fail(msg="7.0.0 should match <=7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "boundary exclusive >",
			script: `
def check(ctx):
    if version_matches("7.0.0", [">7.0.0"]):
        fail(msg="7.0.0 should not match >7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "boundary exclusive <",
			script: `
def check(ctx):
    if version_matches("7.0.0", ["<7.0.0"]):
        fail(msg="7.0.0 should not match <7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "invalid version",
			script: `
result = version_matches("invalid", [">=7.0.0"])
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
		{
			name: "invalid constraint",
			script: `
result = version_matches("7.0.0", [">=invalid"])
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter()
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "less than",
			script: `
def check(ctx):
    if version_compare("7.0.0", "8.0.0") != -1:
        fail(msg="7.0.0 should be less than 8.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "greater than",
			script: `
def check(ctx):
    if version_compare("8.0.0", "7.0.0") != 1:
        fail(msg="8.0.0 should be greater than 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "equal",
			script: `
def check(ctx):
    if version_compare("7.0.0", "7.0.0") != 0:
        fail(msg="7.0.0 should equal 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "minor version difference",
			script: `
def check(ctx):
    if version_compare("7.1.0", "7.0.0") != 1:
        fail(msg="7.1.0 should be greater than 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "patch version difference",
			script: `
def check(ctx):
    if version_compare("7.0.1", "7.0.0") != 1:
        fail(msg="7.0.1 should be greater than 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "prerelease less than stable",
			script: `
def check(ctx):
    if version_compare("7.0.0-rc1", "7.0.0") != -1:
        fail(msg="7.0.0-rc1 should be less than 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "invalid v1",
			script: `
result = version_compare("invalid", "7.0.0")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
		{
			name: "invalid v2",
			script: `
result = version_compare("7.0.0", "invalid")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter()
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "simple version",
			script: `
def check(ctx):
    v = parse_version("7.0.0")
    if v.major != 7:
        fail(msg="major should be 7")
    if v.minor != 0:
        fail(msg="minor should be 0")
    if v.patch != 0:
        fail(msg="patch should be 0")
    if v.prerelease != "":
        fail(msg="prerelease should be empty")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version with all parts",
			script: `
def check(ctx):
    v = parse_version("8.1.2")
    if v.major != 8:
        fail(msg="major should be 8")
    if v.minor != 1:
        fail(msg="minor should be 1")
    if v.patch != 2:
        fail(msg="patch should be 2")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "prerelease version",
			script: `
def check(ctx):
    v = parse_version("7.0.0-rc1")
    if v.major != 7:
        fail(msg="major should be 7")
    if v.prerelease != "rc1":
        fail(msg="prerelease should be 'rc1', got " + v.prerelease)
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "string attribute",
			script: `
def check(ctx):
    v = parse_version("7.0.0-rc1")
    if v.string != "7.0.0-rc1":
        fail(msg="string should be '7.0.0-rc1'")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "invalid version",
			script: `
v = parse_version("invalid")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter()
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "version greater",
			script: `
def check(ctx):
    if not version_at_least("8.0.0", "7.0.0"):
        fail(msg="8.0.0 should be at least 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version equal",
			script: `
def check(ctx):
    if not version_at_least("7.0.0", "7.0.0"):
        fail(msg="7.0.0 should be at least 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version less",
			script: `
def check(ctx):
    if version_at_least("6.0.0", "7.0.0"):
        fail(msg="6.0.0 should not be at least 7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "invalid version",
			script: `
result = version_at_least("invalid", "7.0.0")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
		{
			name: "invalid minimum",
			script: `
result = version_at_least("7.0.0", "invalid")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid minimum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter()
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVersionInRange(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "in range",
			script: `
def check(ctx):
    if not version_in_range("7.5.0", "7.0.0", "8.0.0"):
        fail(msg="7.5.0 should be in range 7.0.0-8.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "at min boundary",
			script: `
def check(ctx):
    if not version_in_range("7.0.0", "7.0.0", "8.0.0"):
        fail(msg="7.0.0 should be in range (min boundary)")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "at max boundary",
			script: `
def check(ctx):
    if not version_in_range("8.0.0", "7.0.0", "8.0.0"):
        fail(msg="8.0.0 should be in range (max boundary)")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "below range",
			script: `
def check(ctx):
    if version_in_range("6.0.0", "7.0.0", "8.0.0"):
        fail(msg="6.0.0 should not be in range")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "above range",
			script: `
def check(ctx):
    if version_in_range("9.0.0", "7.0.0", "8.0.0"):
        fail(msg="9.0.0 should not be in range")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "invalid version",
			script: `
result = version_in_range("invalid", "7.0.0", "8.0.0")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid version",
		},
		{
			name: "invalid min",
			script: `
result = version_in_range("7.5.0", "invalid", "8.0.0")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid min",
		},
		{
			name: "invalid max",
			script: `
result = version_in_range("7.5.0", "7.0.0", "invalid")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`,
			wantErr:     true,
			errContains: "invalid max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter()
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVersionBuiltinsIntegration(t *testing.T) {
	// Test combining multiple version builtins in realistic scenarios
	script := `
def check_version_compatibility(ctx):
    # Scenario: Check if project bazel version meets recipe requirements
    bazel_version = "8.0.0"
    min_version = "7.0.0"
    max_version = "9.0.0"

    # Method 1: Using version_matches
    if not version_matches(bazel_version, [">=" + min_version, "<=" + max_version]):
        fail(msg="Version check failed with version_matches")

    # Method 2: Using version_in_range
    if not version_in_range(bazel_version, min_version, max_version):
        fail(msg="Version check failed with version_in_range")

    # Method 3: Using version_compare
    if version_compare(bazel_version, min_version) < 0:
        fail(msg="Version is below minimum")
    if version_compare(bazel_version, max_version) > 0:
        fail(msg="Version is above maximum")

    # Method 4: Using version_at_least
    if not version_at_least(bazel_version, min_version):
        fail(msg="Version check failed with version_at_least")

    # Parse and use components
    v = parse_version(bazel_version)
    if v.major < 7:
        fail(msg="Major version too low")

    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_version_compatibility),
])
`

	interp := NewInterpreter()
	_, err := interp.LoadString("test.star", script)
	if err != nil {
		t.Errorf("integration test failed: %v", err)
	}
}
