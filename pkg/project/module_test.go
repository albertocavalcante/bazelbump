// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseModuleBazel(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantName       string
		wantVersion    string
		wantCompat     []string
		wantDepsCount  int
		wantErr        bool
	}{
		{
			name: "simple module",
			content: `module(name = "test_module")`,
			wantName:      "test_module",
			wantDepsCount: 0,
		},
		{
			name: "module with version",
			content: `module(name = "versioned", version = "1.2.3")`,
			wantName:      "versioned",
			wantVersion:   "1.2.3",
			wantDepsCount: 0,
		},
		{
			name: "module with compatibility",
			content: `
module(
    name = "compat_test",
    version = "2.0.0",
    bazel_compatibility = [">=6.4.0", "<=9.0.0", "-7.1.0"],
)
`,
			wantName:    "compat_test",
			wantVersion: "2.0.0",
			wantCompat:  []string{">=6.4.0", "<=9.0.0", "-7.1.0"},
		},
		{
			name: "module with bazel_deps",
			content: `
module(name = "with_deps")

bazel_dep(name = "rules_cc", version = "0.0.17")
bazel_dep(name = "rules_go", version = "0.50.0")
`,
			wantName:      "with_deps",
			wantDepsCount: 2,
		},
		{
			name: "empty module",
			content: `module()`,
			wantName:      "",
			wantDepsCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			path := filepath.Join(tempDir, "MODULE.bazel")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			mf, err := ParseModuleBazel(path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseModuleBazel() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseModuleBazel() unexpected error: %v", err)
			}

			if mf.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", mf.Name, tt.wantName)
			}

			if mf.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", mf.Version, tt.wantVersion)
			}

			if len(mf.BazelDeps) != tt.wantDepsCount {
				t.Errorf("BazelDeps count = %d, want %d", len(mf.BazelDeps), tt.wantDepsCount)
			}

			if tt.wantCompat != nil {
				if len(mf.Compatibility) != len(tt.wantCompat) {
					t.Errorf("Compatibility length = %d, want %d", len(mf.Compatibility), len(tt.wantCompat))
				} else {
					for i, c := range tt.wantCompat {
						if mf.Compatibility[i] != c {
							t.Errorf("Compatibility[%d] = %q, want %q", i, mf.Compatibility[i], c)
						}
					}
				}
			}
		})
	}
}

func TestParseBazelDepDetails(t *testing.T) {
	content := `
module(name = "test")

bazel_dep(name = "simple_dep", version = "1.0.0")
bazel_dep(name = "with_repo", version = "2.0.0", repo_name = "custom_repo")
bazel_dep(name = "dev_dep", version = "3.0.0", dev_dependency = True)
bazel_dep(name = "full_dep", version = "4.0.0", repo_name = "full", dev_dependency = True)
`

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "MODULE.bazel")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	mf, err := ParseModuleBazel(path)
	if err != nil {
		t.Fatalf("ParseModuleBazel() error: %v", err)
	}

	tests := []struct {
		name          string
		wantVersion   string
		wantRepoName  string
		wantDevDep    bool
	}{
		{name: "simple_dep", wantVersion: "1.0.0", wantRepoName: "", wantDevDep: false},
		{name: "with_repo", wantVersion: "2.0.0", wantRepoName: "custom_repo", wantDevDep: false},
		{name: "dev_dep", wantVersion: "3.0.0", wantRepoName: "", wantDevDep: true},
		{name: "full_dep", wantVersion: "4.0.0", wantRepoName: "full", wantDevDep: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := mf.GetBazelDep(tt.name)
			if dep == nil {
				t.Fatalf("GetBazelDep(%q) returned nil", tt.name)
			}

			if dep.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", dep.Version, tt.wantVersion)
			}

			if dep.RepoName != tt.wantRepoName {
				t.Errorf("RepoName = %q, want %q", dep.RepoName, tt.wantRepoName)
			}

			if dep.DevDependency != tt.wantDevDep {
				t.Errorf("DevDependency = %v, want %v", dep.DevDependency, tt.wantDevDep)
			}
		})
	}
}

func TestModuleFileHasBazelDep(t *testing.T) {
	content := `
module(name = "test")
bazel_dep(name = "rules_cc", version = "0.0.17")
`

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "MODULE.bazel")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	mf, err := ParseModuleBazel(path)
	if err != nil {
		t.Fatalf("ParseModuleBazel() error: %v", err)
	}

	if !mf.HasBazelDep("rules_cc") {
		t.Error("HasBazelDep(rules_cc) = false, want true")
	}

	if mf.HasBazelDep("rules_go") {
		t.Error("HasBazelDep(rules_go) = true, want false")
	}
}

func TestParseModuleBazelContent(t *testing.T) {
	content := []byte(`
module(
    name = "content_test",
    version = "1.0.0",
)

bazel_dep(name = "dep1", version = "1.0.0")
`)

	mf, err := ParseModuleBazelContent("test.bazel", content)
	if err != nil {
		t.Fatalf("ParseModuleBazelContent() error: %v", err)
	}

	if mf.Name != "content_test" {
		t.Errorf("Name = %q, want %q", mf.Name, "content_test")
	}

	if len(mf.BazelDeps) != 1 {
		t.Errorf("BazelDeps count = %d, want 1", len(mf.BazelDeps))
	}
}

func TestParseModuleBazelInvalidSyntax(t *testing.T) {
	content := []byte(`
module(name = "test"
bazel_dep(name = "broken"
`)

	_, err := ParseModuleBazelContent("broken.bazel", content)
	if err == nil {
		t.Error("ParseModuleBazelContent() expected error for invalid syntax, got nil")
	}
}

func TestParseModuleBazelFileNotFound(t *testing.T) {
	_, err := ParseModuleBazel("/nonexistent/path/MODULE.bazel")
	if err == nil {
		t.Error("ParseModuleBazel() expected error for missing file, got nil")
	}
}

func TestRealWorldModuleBazel(t *testing.T) {
	// Test with a more realistic MODULE.bazel file
	content := `
"""Module for rules_go, the Bazel Go rules."""

module(
    name = "rules_go",
    version = "0.50.0",
    bazel_compatibility = [">=6.4.0"],
    compatibility_level = 0,
)

bazel_dep(name = "bazel_features", version = "1.9.1")
bazel_dep(name = "bazel_skylib", version = "1.7.1")
bazel_dep(name = "platforms", version = "0.0.10")
bazel_dep(name = "rules_proto", version = "6.0.2")
bazel_dep(name = "protobuf", version = "27.1", repo_name = "com_google_protobuf")

bazel_dep(name = "gazelle", version = "0.37.0")
bazel_dep(name = "rules_cc", version = "0.0.10", dev_dependency = True)
`

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "MODULE.bazel")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	mf, err := ParseModuleBazel(path)
	if err != nil {
		t.Fatalf("ParseModuleBazel() error: %v", err)
	}

	if mf.Name != "rules_go" {
		t.Errorf("Name = %q, want %q", mf.Name, "rules_go")
	}

	if mf.Version != "0.50.0" {
		t.Errorf("Version = %q, want %q", mf.Version, "0.50.0")
	}

	if len(mf.Compatibility) != 1 || mf.Compatibility[0] != ">=6.4.0" {
		t.Errorf("Compatibility = %v, want [>=6.4.0]", mf.Compatibility)
	}

	if len(mf.BazelDeps) != 7 {
		t.Errorf("BazelDeps count = %d, want 7", len(mf.BazelDeps))
	}

	// Check specific deps
	protobuf := mf.GetBazelDep("protobuf")
	if protobuf == nil {
		t.Fatal("protobuf dep not found")
	}
	if protobuf.RepoName != "com_google_protobuf" {
		t.Errorf("protobuf repo_name = %q, want %q", protobuf.RepoName, "com_google_protobuf")
	}

	rulesCc := mf.GetBazelDep("rules_cc")
	if rulesCc == nil {
		t.Fatal("rules_cc dep not found")
	}
	if !rulesCc.DevDependency {
		t.Error("rules_cc should be a dev dependency")
	}
}
