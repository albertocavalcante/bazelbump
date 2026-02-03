// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package bazelrc

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := `
# This is a comment
build --keep_going
build --jobs=8

# Config section
build:debug --compilation_mode=dbg
build:debug --copt=-g

test --test_output=errors
test:debug --test_arg=--verbose

common --color=yes

import /path/to/other.bazelrc
try-import %workspace%/user.bazelrc
`

	file, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Check we got some lines (exact count depends on input)
	if len(file.Lines) == 0 {
		t.Error("expected some lines")
	}

	// Check imports
	if len(file.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(file.Imports))
	}
}

func TestFlagsFor(t *testing.T) {
	input := `
common --color=yes
build --keep_going
build:opt --compilation_mode=opt
test --test_output=errors
test:opt --test_timeout=300
`

	file, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		command string
		config  string
		want    []string
	}{
		{
			command: "build",
			config:  "",
			want:    []string{"--color=yes", "--keep_going"},
		},
		{
			command: "build",
			config:  "opt",
			want:    []string{"--color=yes", "--compilation_mode=opt"},
		},
		{
			command: "test",
			config:  "",
			// test inherits from build
			want: []string{"--color=yes", "--keep_going", "--test_output=errors"},
		},
		{
			command: "test",
			config:  "opt",
			want:    []string{"--color=yes", "--compilation_mode=opt", "--test_timeout=300"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.command+":"+tt.config, func(t *testing.T) {
			got := file.FlagsFor(tt.command, tt.config)
			if len(got) != len(tt.want) {
				t.Errorf("FlagsFor(%q, %q) = %v, want %v", tt.command, tt.config, got, tt.want)
				return
			}
			for i, flag := range got {
				if flag != tt.want[i] {
					t.Errorf("FlagsFor(%q, %q)[%d] = %q, want %q", tt.command, tt.config, i, flag, tt.want[i])
				}
			}
		})
	}
}

func TestAllConfigs(t *testing.T) {
	input := `
build --keep_going
build:opt --compilation_mode=opt
build:debug --compilation_mode=dbg
test:asan --test_env=ASAN_OPTIONS=detect_leaks=1
`

	file, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	configs := file.AllConfigs()
	expected := []string{"opt", "debug", "asan"}

	if len(configs) != len(expected) {
		t.Errorf("AllConfigs() = %v, want %v", configs, expected)
	}
}

func TestAllFlags(t *testing.T) {
	input := `
build --keep_going
build --jobs=8
build --noremote_accept_cached
test --test_output=errors
`

	file, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	flags := file.AllFlags()
	// Should have: keep_going, jobs, remote_accept_cached, test_output
	if len(flags) != 4 {
		t.Errorf("AllFlags() = %v, want 4 flags", flags)
	}
}

func TestFindFlag(t *testing.T) {
	input := `
build --keep_going
build:opt --keep_going
test --nokeep_going
`

	file, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	occurrences := file.FindFlag("keep_going")
	if len(occurrences) != 3 {
		t.Errorf("FindFlag(keep_going) found %d occurrences, want 3", len(occurrences))
	}
}

func TestExtractFlagName(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"--keep_going", "keep_going"},
		{"--jobs=8", "jobs"},
		{"--noremote_accept_cached", "remote_accept_cached"},
		{"-k", "k"},
		{"--compilation_mode=opt", "compilation_mode"},
		{"--test_arg=--verbose", "test_arg"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got := extractFlagName(tt.flag)
			if got != tt.want {
				t.Errorf("extractFlagName(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

func TestVersionCondition(t *testing.T) {
	input := `
try-import-if-bazel-version >=7.0.0 %workspace%/v7.bazelrc
try-import-if-bazel-version ~8.0 %workspace%/v8.bazelrc
`

	file, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(file.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(file.Imports))
	}

	// First import: >=7.0.0
	imp := file.Imports[0]
	if imp.Version == nil {
		t.Error("expected version condition for first import")
	} else {
		if imp.Version.Operator != ">=" {
			t.Errorf("expected operator >=, got %s", imp.Version.Operator)
		}
		if imp.Version.Version != "7.0.0" {
			t.Errorf("expected version 7.0.0, got %s", imp.Version.Version)
		}
	}

	// Second import: ~8.0
	imp = file.Imports[1]
	if imp.Version == nil {
		t.Error("expected version condition for second import")
	} else {
		if imp.Version.Operator != "~" {
			t.Errorf("expected operator ~, got %s", imp.Version.Operator)
		}
	}
}
