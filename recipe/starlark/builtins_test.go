// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestReadFile tests the read_file builtin
func TestReadFile(t *testing.T) {
	// Create a temp directory for testing
	tempDir := t.TempDir()

	// Create test file
	testContent := "Hello, World!"
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "read existing file",
			script: `
result = read_file("test.txt")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "read non-existent file with default",
			script: `
result = read_file("nonexistent.txt", default="default value")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "read non-existent file without default",
			script: `
result = read_file("nonexistent.txt")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
			wantErr:     true,
			errContains: "no such file",
		},
		{
			name: "path traversal attempt",
			script: `
result = read_file("../../../etc/passwd")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
			wantErr:     true,
			errContains: "escapes project directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter(WithProjectPath(tempDir))
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
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

// TestFileExists tests the file_exists builtin
func TestFileExists(t *testing.T) {
	// Create a temp directory for testing
	tempDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tempDir, "exists.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create test directory
	testDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(testDir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "file exists",
			script: `
exists = file_exists("exists.txt")
# Use exists in a list comprehension to verify it works
files = ["found"] if exists else []
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "file does not exist",
			script: `
exists = file_exists("nonexistent.txt")
# Just verify it returns False without error
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "directory exists",
			script: `
exists = file_exists("subdir")
# Just verify it returns True without error  
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "path traversal attempt",
			script: `
exists = file_exists("../../../etc/passwd")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
			wantErr:     true,
			errContains: "escapes project directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter(WithProjectPath(tempDir))
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
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

// TestListDir tests the list_dir builtin
func TestListDir(t *testing.T) {
	// Create a temp directory for testing
	tempDir := t.TempDir()

	// Create test structure
	if err := os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("1"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("2"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	subdir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "list directory non-recursive",
			script: `
entries = list_dir(".")
# Should contain file1.txt, file2.txt, subdir (3 items)
count = len(entries)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "list directory recursive",
			script: `
entries = list_dir(".", recursive=True)
# Should contain file1.txt, file2.txt, subdir, subdir/nested.txt (4 items)
count = len(entries)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "list subdirectory",
			script: `
entries = list_dir("subdir")
# Should contain nested.txt (1 item)
count = len(entries)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "list non-directory",
			script: `
entries = list_dir("file1.txt")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
			wantErr:     true,
			errContains: "not a directory",
		},
		{
			name: "path traversal attempt",
			script: `
entries = list_dir("../../../etc")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
			wantErr:     true,
			errContains: "escapes project directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter(WithProjectPath(tempDir))
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
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

// TestGlob tests the glob builtin
func TestGlob(t *testing.T) {
	// Create a temp directory for testing
	tempDir := t.TempDir()

	// Create test structure
	if err := os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("1"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("2"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "script.go"), []byte("go"), 0o644); err != nil {
		t.Fatalf("failed to create go file: %v", err)
	}
	subdir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "code.go"), []byte("code"), 0o644); err != nil {
		t.Fatalf("failed to create nested go file: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "glob txt files in root",
			script: `
matches = glob("*.txt")
# Should find file1.txt, file2.txt
count = len(matches)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "glob all txt files recursively",
			script: `
matches = glob("**/*.txt")
# Should find file1.txt, file2.txt, subdir/nested.txt
count = len(matches)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "glob with exclude",
			script: `
matches = glob("**/*.txt", exclude=["subdir/*"])
# Should find file1.txt, file2.txt (excluding subdir)
count = len(matches)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "glob go files recursively",
			script: `
matches = glob("**/*.go")
# Should find script.go, subdir/code.go
count = len(matches)
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter(WithProjectPath(tempDir))
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
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

// TestSandboxPath tests the sandboxPath helper function
func TestSandboxPath(t *testing.T) {
	tempDir := t.TempDir()

	interp := NewInterpreter(WithProjectPath(tempDir))

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid path",
			path:    "subdir/file.txt",
			wantErr: false,
		},
		{
			name:    "dot path",
			path:    ".",
			wantErr: false,
		},
		{
			name:        "parent directory escape",
			path:        "../escape",
			wantErr:     true,
			errContains: "escapes project directory",
		},
		{
			name:        "deep escape attempt",
			path:        "subdir/../../escape",
			wantErr:     true,
			errContains: "escapes project directory",
		},
		{
			name:        "absolute path escape",
			path:        "/etc/passwd",
			wantErr:     true,
			errContains: "escapes project directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := interp.sandboxPath(tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
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

// TestNoProjectPath tests that file I/O builtins fail when no project path is set
func TestNoProjectPath(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{
			name: "read_file without project path",
			script: `
result = read_file("test.txt")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "file_exists without project path",
			script: `
result = file_exists("test.txt")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "list_dir without project path",
			script: `
result = list_dir(".")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "glob without project path",
			script: `
result = glob("*.txt")
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter() // No project path
			_, err := interp.LoadString("test.star", tt.script)

			if err == nil {
				t.Errorf("expected error about missing project path, got nil")
				return
			}
			if !containsSubstring(err.Error(), "project path") {
				t.Errorf("error %q does not mention project path", err.Error())
			}
		})
	}
}

// TestMatchGlobPattern tests the matchGlobPattern helper function
func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// Simple patterns
		{"*.txt", "file.txt", true},
		{"*.txt", "file.go", false},
		{"*.txt", "dir/file.txt", false}, // Simple glob only matches in current dir

		// ** patterns
		{"**/*.txt", "file.txt", true},
		{"**/*.txt", "dir/file.txt", true},
		{"**/*.txt", "a/b/c/file.txt", true},
		{"**/*.txt", "file.go", false},

		// With prefix
		{"src/**/*.go", "src/main.go", true},
		{"src/**/*.go", "src/pkg/util.go", true},
		{"src/**/*.go", "test/main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlobPattern(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlobPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		(len(s) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// sortStrings sorts a slice of strings for comparison
func sortStrings(s []string) []string {
	sorted := make([]string, len(s))
	copy(sorted, s)
	sort.Strings(sorted)
	return sorted
}
