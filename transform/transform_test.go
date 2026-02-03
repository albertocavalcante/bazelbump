// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/albertocavalcante/bazelbump/analysis"
)

// TestRemoveFlagTransform tests the RemoveFlagTransform.
func TestRemoveFlagTransform(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		input    string
		want     string
	}{
		{
			name:     "remove_single_flag_on_line",
			flagName: "keep_going",
			input:    "build --keep_going\n",
			want:     "",
		},
		{
			name:     "remove_flag_with_value",
			flagName: "jobs",
			input:    "build --jobs=8\n",
			want:     "",
		},
		{
			name:     "remove_one_of_multiple_flags",
			flagName: "keep_going",
			input:    "build --keep_going --jobs=8\n",
			want:     "build --jobs=8\n",
		},
		{
			name:     "remove_negated_flag",
			flagName: "remote_accept_cached",
			input:    "build --noremote_accept_cached\n",
			want:     "",
		},
		{
			name:     "preserve_unrelated_lines",
			flagName: "keep_going",
			input:    "# Comment\nbuild --keep_going\ntest --test_output=errors\n",
			want:     "# Comment\ntest --test_output=errors\n",
		},
		{
			name:     "remove_flag_with_config",
			flagName: "keep_going",
			input:    "build:ci --keep_going\n",
			want:     "",
		},
		{
			name:     "no_match_leaves_unchanged",
			flagName: "nonexistent",
			input:    "build --keep_going\n",
			want:     "build --keep_going\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transform := &RemoveFlagTransform{FlagName: tt.flagName}
			got, err := transform.Apply([]byte(tt.input))
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Apply() =\n%q\nwant\n%q", string(got), tt.want)
			}
		})
	}
}

// TestRemoveFlagTransformCanApply tests CanApply for RemoveFlagTransform.
func TestRemoveFlagTransformCanApply(t *testing.T) {
	transform := &RemoveFlagTransform{FlagName: "keep_going"}

	tests := []struct {
		name  string
		issue *analysis.FlagIssue
		want  bool
	}{
		{
			name: "matching_removed_flag",
			issue: &analysis.FlagIssue{
				FlagName:  "keep_going",
				IssueType: analysis.FlagRemoved,
			},
			want: true,
		},
		{
			name: "different_flag_name",
			issue: &analysis.FlagIssue{
				FlagName:  "other_flag",
				IssueType: analysis.FlagRemoved,
			},
			want: false,
		},
		{
			name: "deprecated_not_removed",
			issue: &analysis.FlagIssue{
				FlagName:  "keep_going",
				IssueType: analysis.FlagDeprecated,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transform.CanApply(tt.issue)
			if got != tt.want {
				t.Errorf("CanApply() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCommentFlagTransform tests the CommentFlagTransform.
func TestCommentFlagTransform(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		reason   string
		input    string
		want     string
	}{
		{
			name:     "comment_single_flag",
			flagName: "experimental_foo",
			reason:   "This flag will be removed in the next version",
			input:    "build --experimental_foo\n",
			want:     "# DEPRECATED: This flag will be removed in the next version\n# build --experimental_foo\n",
		},
		{
			name:     "comment_flag_default_reason",
			flagName: "experimental_bar",
			reason:   "",
			input:    "build --experimental_bar\n",
			want:     "# DEPRECATED: deprecated\n# build --experimental_bar\n",
		},
		{
			name:     "preserve_other_lines",
			flagName: "experimental_foo",
			reason:   "deprecated",
			input:    "# Header\nbuild --experimental_foo\ntest --test_output=errors\n",
			want:     "# Header\n# DEPRECATED: deprecated\n# build --experimental_foo\ntest --test_output=errors\n",
		},
		{
			name:     "no_match_unchanged",
			flagName: "nonexistent",
			reason:   "deprecated",
			input:    "build --keep_going\n",
			want:     "build --keep_going\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transform := &CommentFlagTransform{
				FlagName: tt.flagName,
				Reason:   tt.reason,
			}
			got, err := transform.Apply([]byte(tt.input))
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Apply() =\n%q\nwant\n%q", string(got), tt.want)
			}
		})
	}
}

// TestCommentFlagTransformCanApply tests CanApply for CommentFlagTransform.
func TestCommentFlagTransformCanApply(t *testing.T) {
	transform := &CommentFlagTransform{FlagName: "experimental_foo"}

	tests := []struct {
		name  string
		issue *analysis.FlagIssue
		want  bool
	}{
		{
			name: "matching_deprecated_flag",
			issue: &analysis.FlagIssue{
				FlagName:  "experimental_foo",
				IssueType: analysis.FlagDeprecated,
			},
			want: true,
		},
		{
			name: "removed_not_deprecated",
			issue: &analysis.FlagIssue{
				FlagName:  "experimental_foo",
				IssueType: analysis.FlagRemoved,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transform.CanApply(tt.issue)
			if got != tt.want {
				t.Errorf("CanApply() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReplaceFlagTransform tests the ReplaceFlagTransform.
func TestReplaceFlagTransform(t *testing.T) {
	tests := []struct {
		name    string
		oldFlag string
		newFlag string
		input   string
		want    string
	}{
		{
			name:    "replace_simple_flag",
			oldFlag: "experimental_foo",
			newFlag: "foo",
			input:   "build --experimental_foo\n",
			want:    "build --foo\n",
		},
		{
			name:    "replace_flag_with_value",
			oldFlag: "experimental_bar",
			newFlag: "bar",
			input:   "build --experimental_bar=true\n",
			want:    "build --bar=true\n",
		},
		{
			name:    "replace_negated_flag",
			oldFlag: "experimental_baz",
			newFlag: "baz",
			input:   "build --noexperimental_baz\n",
			want:    "build --nobaz\n",
		},
		{
			name:    "replace_preserves_other_flags",
			oldFlag: "experimental_foo",
			newFlag: "foo",
			input:   "build --keep_going --experimental_foo --jobs=8\n",
			want:    "build --keep_going --foo --jobs=8\n",
		},
		{
			name:    "no_match_unchanged",
			oldFlag: "nonexistent",
			newFlag: "new_flag",
			input:   "build --keep_going\n",
			want:    "build --keep_going\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transform := &ReplaceFlagTransform{
				OldFlag: tt.oldFlag,
				NewFlag: tt.newFlag,
			}
			got, err := transform.Apply([]byte(tt.input))
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Apply() =\n%q\nwant\n%q", string(got), tt.want)
			}
		})
	}
}

// TestGeneratePatches tests patch generation.
func TestGeneratePatches(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	bazelrcPath := filepath.Join(tmpDir, ".bazelrc")
	content := "build --keep_going\nbuild --removed_flag\ntest --test_output=errors\n"
	if err := os.WriteFile(bazelrcPath, []byte(content), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	issues := []*analysis.FlagIssue{
		{
			FlagName:   "removed_flag",
			IssueType:  analysis.FlagRemoved,
			File:       bazelrcPath,
			Line:       2,
			Suggestion: "Remove --removed_flag",
		},
	}

	patches, err := GeneratePatches(issues)
	if err != nil {
		t.Fatalf("GeneratePatches() error = %v", err)
	}

	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}

	patch := patches[0]
	if patch.File != bazelrcPath {
		t.Errorf("patch.File = %q, want %q", patch.File, bazelrcPath)
	}
	if patch.OldLine != 2 {
		t.Errorf("patch.OldLine = %d, want 2", patch.OldLine)
	}
	if patch.OldText != "build --removed_flag" {
		t.Errorf("patch.OldText = %q, want %q", patch.OldText, "build --removed_flag")
	}
	if patch.Transform != "remove" {
		t.Errorf("patch.Transform = %q, want %q", patch.Transform, "remove")
	}
}

// TestGeneratePatchesEmpty tests that empty issues returns nil.
func TestGeneratePatchesEmpty(t *testing.T) {
	patches, err := GeneratePatches(nil)
	if err != nil {
		t.Fatalf("GeneratePatches() error = %v", err)
	}
	if patches != nil {
		t.Errorf("expected nil patches for empty issues, got %v", patches)
	}
}

// TestApplyPatchesDryRun tests dry-run mode.
func TestApplyPatchesDryRun(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	bazelrcPath := filepath.Join(tmpDir, ".bazelrc")
	originalContent := "build --keep_going\nbuild --removed_flag\ntest --test_output=errors\n"
	if err := os.WriteFile(bazelrcPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	patches := []*Patch{
		{
			File:      bazelrcPath,
			OldLine:   2,
			OldText:   "build --removed_flag",
			NewText:   "",
			Transform: "remove",
		},
	}

	// Run in dry-run mode
	if err := ApplyPatches(patches, true); err != nil {
		t.Fatalf("ApplyPatches(dryRun=true) error = %v", err)
	}

	// Verify file was NOT modified
	content, err := os.ReadFile(bazelrcPath)
	if err != nil {
		t.Fatalf("read .bazelrc: %v", err)
	}
	if string(content) != originalContent {
		t.Errorf("file was modified during dry-run:\ngot:\n%s\nwant:\n%s", string(content), originalContent)
	}
}

// TestApplyPatches tests actual patch application.
func TestApplyPatches(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	bazelrcPath := filepath.Join(tmpDir, ".bazelrc")
	originalContent := "build --keep_going\nbuild --removed_flag\ntest --test_output=errors\n"
	if err := os.WriteFile(bazelrcPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	patches := []*Patch{
		{
			File:      bazelrcPath,
			OldLine:   2,
			OldText:   "build --removed_flag",
			NewText:   "",
			Transform: "remove",
		},
	}

	// Apply patches
	if err := ApplyPatches(patches, false); err != nil {
		t.Fatalf("ApplyPatches() error = %v", err)
	}

	// Verify file was modified
	content, err := os.ReadFile(bazelrcPath)
	if err != nil {
		t.Fatalf("read .bazelrc: %v", err)
	}

	expectedContent := "build --keep_going\ntest --test_output=errors\n"
	if string(content) != expectedContent {
		t.Errorf("ApplyPatches() result:\ngot:\n%s\nwant:\n%s", string(content), expectedContent)
	}
}

// TestApplyPatchesReplace tests replacement patches.
func TestApplyPatchesReplace(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	bazelrcPath := filepath.Join(tmpDir, ".bazelrc")
	originalContent := "build --experimental_foo\n"
	if err := os.WriteFile(bazelrcPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	patches := []*Patch{
		{
			File:      bazelrcPath,
			OldLine:   1,
			OldText:   "build --experimental_foo",
			NewText:   "build --foo",
			Transform: "replace",
		},
	}

	// Apply patches
	if err := ApplyPatches(patches, false); err != nil {
		t.Fatalf("ApplyPatches() error = %v", err)
	}

	// Verify file was modified
	content, err := os.ReadFile(bazelrcPath)
	if err != nil {
		t.Fatalf("read .bazelrc: %v", err)
	}

	expectedContent := "build --foo\n"
	if string(content) != expectedContent {
		t.Errorf("ApplyPatches() result:\ngot:\n%s\nwant:\n%s", string(content), expectedContent)
	}
}

// TestApplyPatchesMultiLine tests comment transform with multiple lines.
func TestApplyPatchesMultiLine(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	bazelrcPath := filepath.Join(tmpDir, ".bazelrc")
	originalContent := "build --deprecated_flag\nbuild --other\n"
	if err := os.WriteFile(bazelrcPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("write .bazelrc: %v", err)
	}

	patches := []*Patch{
		{
			File:      bazelrcPath,
			OldLine:   1,
			OldText:   "build --deprecated_flag",
			NewText:   "# DEPRECATED: reason\n# build --deprecated_flag",
			Transform: "comment",
		},
	}

	// Apply patches
	if err := ApplyPatches(patches, false); err != nil {
		t.Fatalf("ApplyPatches() error = %v", err)
	}

	// Verify file was modified
	content, err := os.ReadFile(bazelrcPath)
	if err != nil {
		t.Fatalf("read .bazelrc: %v", err)
	}

	expectedContent := "# DEPRECATED: reason\n# build --deprecated_flag\nbuild --other\n"
	if string(content) != expectedContent {
		t.Errorf("ApplyPatches() result:\ngot:\n%s\nwant:\n%s", string(content), expectedContent)
	}
}

// TestApplyPatchesEmpty tests that empty patches returns nil.
func TestApplyPatchesEmpty(t *testing.T) {
	if err := ApplyPatches(nil, false); err != nil {
		t.Errorf("ApplyPatches(nil) error = %v", err)
	}
}

// TestExtractReplacement tests the replacement extraction from suggestions.
func TestExtractReplacement(t *testing.T) {
	tests := []struct {
		suggestion string
		want       string
	}{
		{"use --new_flag instead", "new_flag"},
		{"Use --new_flag instead of this", "new_flag"},
		{"replaced by --other_flag", "other_flag"},
		{"migrate to --better_flag", "better_flag"},
		{"--old_flag -> --new_flag", "new_flag"},
		{"just remove it", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.suggestion, func(t *testing.T) {
			got := extractReplacement(tt.suggestion)
			if got != tt.want {
				t.Errorf("extractReplacement(%q) = %q, want %q", tt.suggestion, got, tt.want)
			}
		})
	}
}

// TestIsCommand tests the command detection helper.
func TestIsCommand(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"build", true},
		{"test", true},
		{"run", true},
		{"common", true},
		{"startup", true},
		{"build:ci", true},
		{"test:opt", true},
		{"--flag", false},
		{"something", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := isCommand(tt.s)
			if got != tt.want {
				t.Errorf("isCommand(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// TestSelectTransform tests transform selection based on issue type.
func TestSelectTransform(t *testing.T) {
	tests := []struct {
		name     string
		issue    *analysis.FlagIssue
		wantType string
	}{
		{
			name: "removed_flag_gets_remove_transform",
			issue: &analysis.FlagIssue{
				FlagName:   "old_flag",
				IssueType:  analysis.FlagRemoved,
				Suggestion: "Remove this flag",
			},
			wantType: "remove",
		},
		{
			name: "deprecated_flag_gets_comment_transform",
			issue: &analysis.FlagIssue{
				FlagName:   "deprecated_flag",
				IssueType:  analysis.FlagDeprecated,
				Suggestion: "This flag is deprecated",
			},
			wantType: "comment",
		},
		{
			name: "removed_with_replacement_gets_replace_transform",
			issue: &analysis.FlagIssue{
				FlagName:   "experimental_foo",
				IssueType:  analysis.FlagRemoved,
				Suggestion: "use --foo instead",
			},
			wantType: "replace",
		},
		{
			name: "deprecated_with_replacement_gets_replace_transform",
			issue: &analysis.FlagIssue{
				FlagName:   "old_flag",
				IssueType:  analysis.FlagDeprecated,
				Suggestion: "replaced by --new_flag",
			},
			wantType: "replace",
		},
		{
			name: "default_changed_gets_no_transform",
			issue: &analysis.FlagIssue{
				FlagName:   "some_flag",
				IssueType:  analysis.FlagDefaultChanged,
				Suggestion: "Review this flag",
			},
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transform := selectTransform(tt.issue)
			if tt.wantType == "" {
				if transform != nil {
					t.Errorf("selectTransform() = %v, want nil", transform.Name())
				}
			} else {
				if transform == nil {
					t.Fatalf("selectTransform() = nil, want %s", tt.wantType)
				}
				if transform.Name() != tt.wantType {
					t.Errorf("selectTransform().Name() = %q, want %q", transform.Name(), tt.wantType)
				}
			}
		})
	}
}

// TestTransformInterface ensures all transforms implement the interface.
func TestTransformInterface(t *testing.T) {
	transforms := []Transform{
		&RemoveFlagTransform{FlagName: "test"},
		&CommentFlagTransform{FlagName: "test", Reason: "test"},
		&ReplaceFlagTransform{OldFlag: "old", NewFlag: "new"},
	}

	for _, tr := range transforms {
		t.Run(tr.Name(), func(t *testing.T) {
			// Verify Name() returns non-empty
			if tr.Name() == "" {
				t.Error("Name() returned empty string")
			}

			// Verify Description() returns non-empty
			if tr.Description() == "" {
				t.Error("Description() returned empty string")
			}

			// Verify Apply() works
			_, err := tr.Apply([]byte("build --test\n"))
			if err != nil {
				t.Errorf("Apply() error = %v", err)
			}
		})
	}
}

// TestReadFileLines tests the file line reader utility.
func TestReadFileLines(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(testPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	lines, err := ReadFileLines(testPath)
	if err != nil {
		t.Fatalf("ReadFileLines() error = %v", err)
	}

	expected := []string{"line1", "line2", "line3"}
	if len(lines) != len(expected) {
		t.Errorf("ReadFileLines() returned %d lines, want %d", len(lines), len(expected))
	}

	for i, line := range lines {
		if i < len(expected) && line != expected[i] {
			t.Errorf("line %d = %q, want %q", i, line, expected[i])
		}
	}
}

// TestReadFileLinesNotFound tests reading a non-existent file.
func TestReadFileLinesNotFound(t *testing.T) {
	_, err := ReadFileLines("/nonexistent/file")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestApplyPatchesToLinesReverseOrder tests that patches are applied correctly
// when multiple patches target different lines.
func TestApplyPatchesToLinesReverseOrder(t *testing.T) {
	lines := []string{
		"line1",
		"line2",
		"line3",
		"line4",
	}

	patches := []*Patch{
		{OldLine: 2, OldText: "line2", NewText: ""},  // delete
		{OldLine: 4, OldText: "line4", NewText: ""},  // delete
	}

	result, err := applyPatchesToLines(lines, patches)
	if err != nil {
		t.Fatalf("applyPatchesToLines() error = %v", err)
	}

	expected := []string{"line1", "line3"}
	if len(result) != len(expected) {
		t.Fatalf("got %d lines, want %d", len(result), len(expected))
	}

	for i, line := range result {
		if line != expected[i] {
			t.Errorf("line %d = %q, want %q", i, line, expected[i])
		}
	}
}

// TestRemoveFlagFromLineWithMultipleFlags tests removing one flag when multiple exist.
func TestRemoveFlagFromLineWithMultipleFlags(t *testing.T) {
	transform := &RemoveFlagTransform{FlagName: "keep_going"}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "first_flag",
			input: "build --keep_going --jobs=8 --verbose",
			want:  "build --jobs=8 --verbose",
		},
		{
			name:  "middle_flag",
			input: "build --jobs=8 --keep_going --verbose",
			want:  "build --jobs=8 --verbose",
		},
		{
			name:  "last_flag",
			input: "build --jobs=8 --verbose --keep_going",
			want:  "build --jobs=8 --verbose",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transform.removeFlagFromLine(tt.input)
			// Normalize whitespace for comparison
			got = strings.Join(strings.Fields(got), " ")
			want := strings.Join(strings.Fields(tt.want), " ")
			if got != want {
				t.Errorf("removeFlagFromLine(%q) = %q, want %q", tt.input, got, want)
			}
		})
	}
}
