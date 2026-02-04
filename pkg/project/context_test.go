// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     map[string]string
		wantUseBzlmod  bool
		wantWorkspace  bool
		wantModuleName string
		wantErr        bool
	}{
		{
			name: "bzlmod only",
			setupFiles: map[string]string{
				"MODULE.bazel": `module(name = "test_module", version = "1.0.0")`,
			},
			wantUseBzlmod:  true,
			wantWorkspace:  false,
			wantModuleName: "test_module",
		},
		{
			name: "workspace only",
			setupFiles: map[string]string{
				"WORKSPACE": "# empty workspace",
			},
			wantUseBzlmod: false,
			wantWorkspace: true,
		},
		{
			name: "workspace.bazel",
			setupFiles: map[string]string{
				"WORKSPACE.bazel": "# empty workspace",
			},
			wantUseBzlmod: false,
			wantWorkspace: true,
		},
		{
			name: "both bzlmod and workspace",
			setupFiles: map[string]string{
				"MODULE.bazel": `module(name = "hybrid_project")`,
				"WORKSPACE":    "# workspace",
			},
			wantUseBzlmod:  true,
			wantWorkspace:  true,
			wantModuleName: "hybrid_project",
		},
		{
			name:       "no project files",
			setupFiles: map[string]string{},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()

			// Setup files
			for name, content := range tt.setupFiles {
				path := filepath.Join(tempDir, name)
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("failed to create %s: %v", name, err)
				}
			}

			// Run detection
			ctx, err := Detect(tempDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Detect() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Detect() unexpected error: %v", err)
			}

			if ctx.UseBzlmod != tt.wantUseBzlmod {
				t.Errorf("UseBzlmod = %v, want %v", ctx.UseBzlmod, tt.wantUseBzlmod)
			}

			if ctx.UseWorkspace != tt.wantWorkspace {
				t.Errorf("UseWorkspace = %v, want %v", ctx.UseWorkspace, tt.wantWorkspace)
			}

			if ctx.ModuleName != tt.wantModuleName {
				t.Errorf("ModuleName = %q, want %q", ctx.ModuleName, tt.wantModuleName)
			}
		})
	}
}

func TestDetectBazelVersion(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "simple version",
			content:     "7.0.0\n",
			wantVersion: "7.0.0",
		},
		{
			name:        "version without newline",
			content:     "8.1.0",
			wantVersion: "8.1.0",
		},
		{
			name:        "version with whitespace",
			content:     "  7.5.0  \n",
			wantVersion: "7.5.0",
		},
		{
			name:        "prerelease version",
			content:     "9.0.0-rc1\n",
			wantVersion: "9.0.0-rc1",
		},
		{
			name:    "latest (bazelisk format)",
			content: "latest\n",
			wantErr: true,
		},
		{
			name:    "x version (bazelisk format)",
			content: "7.x\n",
			wantErr: true,
		},
		{
			name:    "empty file",
			content: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with .bazelversion
			tempDir := t.TempDir()
			bazelVersionPath := filepath.Join(tempDir, ".bazelversion")
			if err := os.WriteFile(bazelVersionPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to create .bazelversion: %v", err)
			}

			// Run detection
			v, err := DetectBazelVersion(tempDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("DetectBazelVersion() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("DetectBazelVersion() unexpected error: %v", err)
			}

			if v.String() != tt.wantVersion {
				t.Errorf("BazelVersion = %q, want %q", v.String(), tt.wantVersion)
			}
		})
	}
}

func TestDetectBazelVersionNotFound(t *testing.T) {
	tempDir := t.TempDir()

	_, err := DetectBazelVersion(tempDir)
	if err == nil {
		t.Errorf("DetectBazelVersion() expected error for missing file, got nil")
	}
}

func TestFindProjectRoot(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		searchFrom string // relative to temp dir
		wantRoot   string // relative to temp dir (empty = root)
		wantErr    bool
	}{
		{
			name: "root is current directory",
			setupFiles: map[string]string{
				"WORKSPACE": "",
			},
			searchFrom: ".",
			wantRoot:   "",
		},
		{
			name: "find workspace in parent",
			setupFiles: map[string]string{
				"WORKSPACE": "",
				"src/main.go": "",
			},
			searchFrom: "src",
			wantRoot:   "",
		},
		{
			name: "find MODULE.bazel in grandparent",
			setupFiles: map[string]string{
				"MODULE.bazel":    `module(name = "test")`,
				"src/pkg/main.go": "",
			},
			searchFrom: "src/pkg",
			wantRoot:   "",
		},
		{
			name:       "no project root",
			setupFiles: map[string]string{},
			searchFrom: ".",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()

			// Setup files
			for name, content := range tt.setupFiles {
				path := filepath.Join(tempDir, name)
				dir := filepath.Dir(path)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("failed to create directory %s: %v", dir, err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("failed to create %s: %v", name, err)
				}
			}

			// Run search
			searchPath := filepath.Join(tempDir, tt.searchFrom)
			root, err := FindProjectRoot(searchPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FindProjectRoot() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("FindProjectRoot() unexpected error: %v", err)
			}

			wantRoot := filepath.Join(tempDir, tt.wantRoot)
			if root != wantRoot {
				t.Errorf("FindProjectRoot() = %q, want %q", root, wantRoot)
			}
		})
	}
}

func TestDetectWithBazelDeps(t *testing.T) {
	tempDir := t.TempDir()

	moduleContent := `
module(
    name = "my_project",
    version = "2.0.0",
    bazel_compatibility = [">=6.4.0", "<=9.0.0"],
)

bazel_dep(name = "rules_cc", version = "0.0.17")
bazel_dep(name = "rules_go", version = "0.50.0", repo_name = "io_bazel_rules_go")
bazel_dep(name = "gazelle", version = "0.39.0", dev_dependency = True)
`
	if err := os.WriteFile(filepath.Join(tempDir, "MODULE.bazel"), []byte(moduleContent), 0o644); err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	ctx, err := Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	// Check module info
	if ctx.ModuleName != "my_project" {
		t.Errorf("ModuleName = %q, want %q", ctx.ModuleName, "my_project")
	}
	if ctx.ModuleVersion != "2.0.0" {
		t.Errorf("ModuleVersion = %q, want %q", ctx.ModuleVersion, "2.0.0")
	}

	// Check compatibility
	if len(ctx.Compatibility) != 2 {
		t.Errorf("Compatibility length = %d, want 2", len(ctx.Compatibility))
	} else {
		if ctx.Compatibility[0] != ">=6.4.0" {
			t.Errorf("Compatibility[0] = %q, want %q", ctx.Compatibility[0], ">=6.4.0")
		}
		if ctx.Compatibility[1] != "<=9.0.0" {
			t.Errorf("Compatibility[1] = %q, want %q", ctx.Compatibility[1], "<=9.0.0")
		}
	}

	// Check bazel_deps
	if len(ctx.BazelDeps) != 3 {
		t.Errorf("BazelDeps length = %d, want 3", len(ctx.BazelDeps))
	}

	// Check rules_cc
	rulesCc := findDep(ctx.BazelDeps, "rules_cc")
	if rulesCc == nil {
		t.Error("rules_cc dependency not found")
	} else {
		if rulesCc.Version != "0.0.17" {
			t.Errorf("rules_cc version = %q, want %q", rulesCc.Version, "0.0.17")
		}
		if rulesCc.DevDependency {
			t.Error("rules_cc should not be a dev dependency")
		}
	}

	// Check rules_go with repo_name
	rulesGo := findDep(ctx.BazelDeps, "rules_go")
	if rulesGo == nil {
		t.Error("rules_go dependency not found")
	} else {
		if rulesGo.RepoName != "io_bazel_rules_go" {
			t.Errorf("rules_go repo_name = %q, want %q", rulesGo.RepoName, "io_bazel_rules_go")
		}
	}

	// Check gazelle dev dependency
	gazelle := findDep(ctx.BazelDeps, "gazelle")
	if gazelle == nil {
		t.Error("gazelle dependency not found")
	} else {
		if !gazelle.DevDependency {
			t.Error("gazelle should be a dev dependency")
		}
	}
}

func findDep(deps []BazelDep, name string) *BazelDep {
	for i := range deps {
		if deps[i].Name == name {
			return &deps[i]
		}
	}
	return nil
}
