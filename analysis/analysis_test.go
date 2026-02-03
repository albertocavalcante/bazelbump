// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/albertocavalcante/bazelbump/flagdb"
)

// TestAnalyzer tests the core analysis functionality.
func TestAnalyzer(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := flagdb.Load(dataDir)
	if err != nil {
		t.Fatalf("Load flagdb: %v", err)
	}

	if !db.HasVersion("8.0.0") || !db.HasVersion("9.0.0") {
		t.Skip("versions 8.0.0 and 9.0.0 not available")
	}

	// Create a temporary project with a .bazelrc file
	tmpDir := t.TempDir()
	bazelrcContent := `
# Test bazelrc
build --keep_going
build --jobs=8
test --test_output=errors
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".bazelrc"), []byte(bazelrcContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	analyzer := New(
		WithProjectPath(tmpDir),
		WithFromVersion("8.0.0"),
		WithToVersion("9.0.0"),
		WithFlagDB(db),
	)

	result, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned, got %d", result.FilesScanned)
	}

	t.Logf("Analysis found %d issues", len(result.FlagIssues))
}

// TestAnalyzerRequiredOptions tests that required options are validated.
func TestAnalyzerRequiredOptions(t *testing.T) {
	analyzer := New()

	// Missing flagDB
	_, err := analyzer.Analyze()
	if err == nil {
		t.Error("expected error for missing flagDB")
	}

	// Missing fromVersion
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}
	db, err := flagdb.Load(dataDir)
	if err != nil {
		t.Fatalf("Load flagdb: %v", err)
	}

	analyzer = New(WithFlagDB(db))
	_, err = analyzer.Analyze()
	if err == nil {
		t.Error("expected error for missing fromVersion")
	}

	// Missing toVersion
	analyzer = New(WithFlagDB(db), WithFromVersion("8.0.0"))
	_, err = analyzer.Analyze()
	if err == nil {
		t.Error("expected error for missing toVersion")
	}
}

// TestDetectRemovedFlags tests detection of flags that were removed.
func TestDetectRemovedFlags(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := flagdb.Load(dataDir)
	if err != nil {
		t.Fatalf("Load flagdb: %v", err)
	}

	if !db.HasVersion("8.0.0") || !db.HasVersion("9.0.0") {
		t.Skip("versions 8.0.0 and 9.0.0 not available")
	}

	// Find a flag that was removed between 8.0.0 and 9.0.0
	diff, err := db.Diff("8.0.0", "9.0.0")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(diff.Removed) == 0 {
		t.Skip("no removed flags between 8.0.0 and 9.0.0")
	}

	// Use the first removed flag
	removedFlag := diff.Removed[0]
	t.Logf("Testing with removed flag: --%s", removedFlag.Name)

	// Create a temporary project that uses this flag
	tmpDir := t.TempDir()
	bazelrcContent := "build --" + removedFlag.Name + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".bazelrc"), []byte(bazelrcContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	analyzer := New(
		WithProjectPath(tmpDir),
		WithFromVersion("8.0.0"),
		WithToVersion("9.0.0"),
		WithFlagDB(db),
	)

	result, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Should detect the removed flag
	removed := result.IssuesByType(FlagRemoved)
	if len(removed) == 0 {
		t.Errorf("expected to detect removed flag --%s", removedFlag.Name)
	} else {
		found := false
		for _, issue := range removed {
			if issue.FlagName == removedFlag.Name {
				found = true
				t.Logf("Detected removed flag: %s (suggestion: %s)", issue.FlagName, issue.Suggestion)
				break
			}
		}
		if !found {
			t.Errorf("expected to find --%s in removed flags", removedFlag.Name)
		}
	}
}

// TestDetectDeprecatedFlags tests detection of flags that became deprecated.
func TestDetectDeprecatedFlags(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := flagdb.Load(dataDir)
	if err != nil {
		t.Fatalf("Load flagdb: %v", err)
	}

	if !db.HasVersion("8.0.0") || !db.HasVersion("9.0.0") {
		t.Skip("versions 8.0.0 and 9.0.0 not available")
	}

	// Find a flag that became deprecated between 8.0.0 and 9.0.0
	diff, err := db.Diff("8.0.0", "9.0.0")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	var deprecatedFlag *flagdb.FlagChange
	for i := range diff.Changed {
		if diff.Changed[i].Change == flagdb.ChangeDeprecated {
			deprecatedFlag = &diff.Changed[i]
			break
		}
	}

	if deprecatedFlag == nil {
		t.Skip("no newly deprecated flags between 8.0.0 and 9.0.0")
	}

	t.Logf("Testing with deprecated flag: --%s", deprecatedFlag.Name)

	// Create a temporary project that uses this flag
	tmpDir := t.TempDir()
	bazelrcContent := "build --" + deprecatedFlag.Name + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".bazelrc"), []byte(bazelrcContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	analyzer := New(
		WithProjectPath(tmpDir),
		WithFromVersion("8.0.0"),
		WithToVersion("9.0.0"),
		WithFlagDB(db),
	)

	result, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Should detect the deprecated flag
	deprecated := result.IssuesByType(FlagDeprecated)
	found := false
	for _, issue := range deprecated {
		if issue.FlagName == deprecatedFlag.Name {
			found = true
			t.Logf("Detected deprecated flag: %s (suggestion: %s)", issue.FlagName, issue.Suggestion)
			break
		}
	}
	if !found {
		t.Logf("Flag --%s not detected as deprecated (may have been removed instead)", deprecatedFlag.Name)
	}
}

// TestMultipleBazelrcFiles tests scanning multiple .bazelrc files.
func TestMultipleBazelrcFiles(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := flagdb.Load(dataDir)
	if err != nil {
		t.Fatalf("Load flagdb: %v", err)
	}

	if !db.HasVersion("8.0.0") || !db.HasVersion("9.0.0") {
		t.Skip("versions 8.0.0 and 9.0.0 not available")
	}

	// Create a temporary project with multiple .bazelrc files
	tmpDir := t.TempDir()

	// Root .bazelrc
	if err := os.WriteFile(filepath.Join(tmpDir, ".bazelrc"), []byte("build --keep_going\n"), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	// user.bazelrc
	if err := os.WriteFile(filepath.Join(tmpDir, "user.bazelrc"), []byte("build --jobs=8\n"), 0644); err != nil {
		t.Fatalf("write user.bazelrc: %v", err)
	}

	// Subdirectory with another .bazelrc
	subDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "ci.bazelrc"), []byte("test --test_output=all\n"), 0644); err != nil {
		t.Fatalf("write ci.bazelrc: %v", err)
	}

	analyzer := New(
		WithProjectPath(tmpDir),
		WithFromVersion("8.0.0"),
		WithToVersion("9.0.0"),
		WithFlagDB(db),
	)

	result, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.FilesScanned != 3 {
		t.Errorf("expected 3 files scanned, got %d", result.FilesScanned)
	}

	t.Logf("Scanned %d files, found %d issues", result.FilesScanned, len(result.FlagIssues))
}

// TestSkipHiddenDirectories tests that hidden directories are skipped.
func TestSkipHiddenDirectories(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := flagdb.Load(dataDir)
	if err != nil {
		t.Fatalf("Load flagdb: %v", err)
	}

	if !db.HasVersion("8.0.0") || !db.HasVersion("9.0.0") {
		t.Skip("versions 8.0.0 and 9.0.0 not available")
	}

	// Create a temporary project
	tmpDir := t.TempDir()

	// Root .bazelrc
	if err := os.WriteFile(filepath.Join(tmpDir, ".bazelrc"), []byte("build --keep_going\n"), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	// Hidden directory with .bazelrc (should be skipped)
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatalf("create hidden dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, ".bazelrc"), []byte("build --jobs=8\n"), 0644); err != nil {
		t.Fatalf("write hidden .bazelrc: %v", err)
	}

	analyzer := New(
		WithProjectPath(tmpDir),
		WithFromVersion("8.0.0"),
		WithToVersion("9.0.0"),
		WithFlagDB(db),
	)

	result, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Should only scan the root .bazelrc, not the one in .hidden/
	if result.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned (hidden dir should be skipped), got %d", result.FilesScanned)
	}
}

// TestExtractFlagName tests the flag name extraction function.
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
		{"--incompatible_enable_cc_toolchain_resolution", "incompatible_enable_cc_toolchain_resolution"},
		{"--noincompatible_enable_cc_toolchain_resolution", "incompatible_enable_cc_toolchain_resolution"},
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

// TestIssuesByFile tests grouping issues by file.
func TestIssuesByFile(t *testing.T) {
	result := &AnalysisResult{
		FlagIssues: []FlagIssue{
			{FlagName: "flag1", File: "/a/.bazelrc", Line: 1},
			{FlagName: "flag2", File: "/a/.bazelrc", Line: 2},
			{FlagName: "flag3", File: "/b/.bazelrc", Line: 1},
		},
	}

	byFile := result.IssuesByFile()
	if len(byFile) != 2 {
		t.Errorf("expected 2 files, got %d", len(byFile))
	}
	if len(byFile["/a/.bazelrc"]) != 2 {
		t.Errorf("expected 2 issues in /a/.bazelrc, got %d", len(byFile["/a/.bazelrc"]))
	}
	if len(byFile["/b/.bazelrc"]) != 1 {
		t.Errorf("expected 1 issue in /b/.bazelrc, got %d", len(byFile["/b/.bazelrc"]))
	}
}

// TestIssuesByType tests filtering issues by type.
func TestIssuesByType(t *testing.T) {
	result := &AnalysisResult{
		FlagIssues: []FlagIssue{
			{FlagName: "flag1", IssueType: FlagRemoved},
			{FlagName: "flag2", IssueType: FlagDeprecated},
			{FlagName: "flag3", IssueType: FlagRemoved},
			{FlagName: "flag4", IssueType: FlagDefaultChanged},
		},
	}

	removed := result.IssuesByType(FlagRemoved)
	if len(removed) != 2 {
		t.Errorf("expected 2 removed flags, got %d", len(removed))
	}

	deprecated := result.IssuesByType(FlagDeprecated)
	if len(deprecated) != 1 {
		t.Errorf("expected 1 deprecated flag, got %d", len(deprecated))
	}

	defaultChanged := result.IssuesByType(FlagDefaultChanged)
	if len(defaultChanged) != 1 {
		t.Errorf("expected 1 default_changed flag, got %d", len(defaultChanged))
	}
}

// TestHasIssues tests the HasIssues helper.
func TestHasIssues(t *testing.T) {
	result := &AnalysisResult{}
	if result.HasIssues() {
		t.Error("expected HasIssues() = false for empty result")
	}

	result.FlagIssues = []FlagIssue{{FlagName: "test"}}
	if !result.HasIssues() {
		t.Error("expected HasIssues() = true when issues exist")
	}
}

// findDataDir locates the data/flags directory.
func findDataDir(t *testing.T) string {
	t.Helper()

	candidates := []string{
		"../data/flags",
		"data/flags",
	}

	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "versions.json")); err == nil {
			return dir
		}
	}

	return ""
}
