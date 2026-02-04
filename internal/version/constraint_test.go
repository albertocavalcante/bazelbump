// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"
)

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Comparison operators
		{name: "greater than or equal", input: ">=6.4.0", wantErr: false},
		{name: "less than or equal", input: "<=9.0.0", wantErr: false},
		{name: "greater than", input: ">7.0.0", wantErr: false},
		{name: "less than", input: "<8.0.0", wantErr: false},
		{name: "equal", input: "==7.5.0", wantErr: false},
		{name: "not equal", input: "!=7.1.0", wantErr: false},
		{name: "single equal", input: "=7.0.0", wantErr: false},

		// Exclude operator (BCR style)
		{name: "exclude", input: "-7.1.0", wantErr: false},
		{name: "exclude with prerelease", input: "-8.0.0-rc1", wantErr: false},

		// Bare version (implicit ==)
		{name: "bare version", input: "7.0.0", wantErr: false},
		{name: "bare version with prerelease", input: "8.0.0-rc1", wantErr: false},

		// With spaces
		{name: "with leading space", input: " >=7.0.0", wantErr: false},
		{name: "with trailing space", input: ">=7.0.0 ", wantErr: false},
		{name: "with space after op", input: ">= 7.0.0", wantErr: false},

		// Errors
		{name: "empty", input: "", wantErr: true},
		{name: "invalid version", input: ">=abc", wantErr: true},
		{name: "invalid exclude", input: "-abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseConstraint(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseConstraint(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseConstraint(%q) unexpected error: %v", tt.input, err)
				return
			}
			if c == nil {
				t.Errorf("ParseConstraint(%q) returned nil constraint", tt.input)
			}
		})
	}
}

func TestConstraintMatches(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		want       bool
	}{
		// Greater than or equal
		{name: ">= match equal", constraint: ">=7.0.0", version: "7.0.0", want: true},
		{name: ">= match greater", constraint: ">=7.0.0", version: "8.0.0", want: true},
		{name: ">= no match lesser", constraint: ">=7.0.0", version: "6.0.0", want: false},
		{name: ">= match patch greater", constraint: ">=7.0.0", version: "7.0.1", want: true},
		{name: ">= match minor greater", constraint: ">=7.0.0", version: "7.1.0", want: true},

		// Less than or equal
		{name: "<= match equal", constraint: "<=9.0.0", version: "9.0.0", want: true},
		{name: "<= match lesser", constraint: "<=9.0.0", version: "8.0.0", want: true},
		{name: "<= no match greater", constraint: "<=9.0.0", version: "10.0.0", want: false},

		// Greater than
		{name: "> no match equal", constraint: ">7.0.0", version: "7.0.0", want: false},
		{name: "> match greater", constraint: ">7.0.0", version: "7.0.1", want: true},

		// Less than
		{name: "< no match equal", constraint: "<8.0.0", version: "8.0.0", want: false},
		{name: "< match lesser", constraint: "<8.0.0", version: "7.9.9", want: true},

		// Equal
		{name: "== match", constraint: "==7.5.0", version: "7.5.0", want: true},
		{name: "== no match", constraint: "==7.5.0", version: "7.5.1", want: false},

		// Not equal
		{name: "!= match different", constraint: "!=7.1.0", version: "7.2.0", want: true},
		{name: "!= no match same", constraint: "!=7.1.0", version: "7.1.0", want: false},

		// Exclude (BCR style)
		{name: "exclude match different", constraint: "-7.1.0", version: "7.2.0", want: true},
		{name: "exclude no match same", constraint: "-7.1.0", version: "7.1.0", want: false},
		{name: "exclude match earlier", constraint: "-7.1.0", version: "7.0.0", want: true},

		// Prerelease handling
		{name: "prerelease less than stable", constraint: ">=7.0.0", version: "7.0.0-rc1", want: false},
		{name: "stable greater than prerelease", constraint: ">7.0.0-rc1", version: "7.0.0", want: true},
		{name: "prerelease exact match", constraint: "==7.0.0-rc1", version: "7.0.0-rc1", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error: %v", tt.constraint, err)
			}

			v, err := Parse(tt.version)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.version, err)
			}

			got := c.Matches(v)
			if got != tt.want {
				t.Errorf("Constraint(%q).Matches(%q) = %v, want %v", tt.constraint, tt.version, got, tt.want)
			}
		})
	}
}

func TestConstraintSetMatches(t *testing.T) {
	tests := []struct {
		name        string
		constraints []string
		version     string
		want        bool
	}{
		// BCR-style compatibility list
		{
			name:        "BCR simple range",
			constraints: []string{">=6.4.0", "<=9.0.0"},
			version:     "7.0.0",
			want:        true,
		},
		{
			name:        "BCR range with exclude - included",
			constraints: []string{">=6.4.0", "<=9.0.0", "-7.1.0"},
			version:     "7.0.0",
			want:        true,
		},
		{
			name:        "BCR range with exclude - excluded version",
			constraints: []string{">=6.4.0", "<=9.0.0", "-7.1.0"},
			version:     "7.1.0",
			want:        false,
		},
		{
			name:        "BCR range below minimum",
			constraints: []string{">=6.4.0", "<=9.0.0"},
			version:     "6.0.0",
			want:        false,
		},
		{
			name:        "BCR range above maximum",
			constraints: []string{">=6.4.0", "<=9.0.0"},
			version:     "10.0.0",
			want:        false,
		},
		{
			name:        "BCR range at minimum boundary",
			constraints: []string{">=6.4.0", "<=9.0.0"},
			version:     "6.4.0",
			want:        true,
		},
		{
			name:        "BCR range at maximum boundary",
			constraints: []string{">=6.4.0", "<=9.0.0"},
			version:     "9.0.0",
			want:        true,
		},
		{
			name:        "multiple excludes",
			constraints: []string{">=6.0.0", "-6.1.0", "-6.2.0", "-6.3.0"},
			version:     "6.4.0",
			want:        true,
		},
		{
			name:        "multiple excludes - hit first",
			constraints: []string{">=6.0.0", "-6.1.0", "-6.2.0", "-6.3.0"},
			version:     "6.1.0",
			want:        false,
		},
		{
			name:        "multiple excludes - hit last",
			constraints: []string{">=6.0.0", "-6.1.0", "-6.2.0", "-6.3.0"},
			version:     "6.3.0",
			want:        false,
		},
		{
			name:        "empty constraint set matches all",
			constraints: []string{},
			version:     "7.0.0",
			want:        true,
		},
		{
			name:        "single constraint",
			constraints: []string{">=7.0.0"},
			version:     "8.0.0",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, err := ParseConstraints(tt.constraints)
			if err != nil {
				t.Fatalf("ParseConstraints(%v) error: %v", tt.constraints, err)
			}

			v, err := Parse(tt.version)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.version, err)
			}

			got := cs.Matches(v)
			if got != tt.want {
				t.Errorf("ConstraintSet(%v).Matches(%q) = %v, want %v", tt.constraints, tt.version, got, tt.want)
			}
		})
	}
}

func TestConstraintString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{">=7.0.0", ">=7.0.0"},
		{"<=9.0.0", "<=9.0.0"},
		{">7.0.0", ">7.0.0"},
		{"<8.0.0", "<8.0.0"},
		{"==7.5.0", "==7.5.0"},
		{"!=7.1.0", "!=7.1.0"},
		{"-7.1.0", "-7.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c, err := ParseConstraint(tt.input)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error: %v", tt.input, err)
			}
			got := c.String()
			if got != tt.want {
				t.Errorf("Constraint.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConstraintSetString(t *testing.T) {
	constraints := []string{">=6.4.0", "<=9.0.0", "-7.1.0"}
	cs, err := ParseConstraints(constraints)
	if err != nil {
		t.Fatalf("ParseConstraints error: %v", err)
	}

	want := ">=6.4.0, <=9.0.0, -7.1.0"
	got := cs.String()
	if got != want {
		t.Errorf("ConstraintSet.String() = %q, want %q", got, want)
	}
}

func TestParseConstraintsError(t *testing.T) {
	tests := []struct {
		name        string
		constraints []string
	}{
		{name: "invalid version", constraints: []string{">=abc"}},
		{name: "empty in middle", constraints: []string{">=7.0.0", "", "<=9.0.0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConstraints(tt.constraints)
			if err == nil {
				t.Errorf("ParseConstraints(%v) expected error, got nil", tt.constraints)
			}
		})
	}
}
