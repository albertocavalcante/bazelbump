// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"testing"
)

func TestComputeHunks_SimpleAdd(t *testing.T) {
	old := []byte("line1\nline2\nline3")
	new := []byte("line1\nnew line\nline2\nline3")

	hunks := ComputeHunks(old, new)

	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Count adds
	added := 0
	for _, h := range hunks {
		for _, line := range h.Lines {
			if line.Type == "add" {
				added++
			}
		}
	}

	if added != 1 {
		t.Errorf("expected 1 added line, got %d", added)
	}
}

func TestComputeHunks_SimpleRemove(t *testing.T) {
	old := []byte("line1\nremove me\nline2\nline3")
	new := []byte("line1\nline2\nline3")

	hunks := ComputeHunks(old, new)

	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Count removes
	removed := 0
	for _, h := range hunks {
		for _, line := range h.Lines {
			if line.Type == "remove" {
				removed++
			}
		}
	}

	if removed != 1 {
		t.Errorf("expected 1 removed line, got %d", removed)
	}
}

func TestComputeHunks_Replace(t *testing.T) {
	old := []byte("line1\nold content\nline3")
	new := []byte("line1\nnew content\nline3")

	hunks := ComputeHunks(old, new)

	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Should have 1 remove and 1 add
	removed := 0
	added := 0
	for _, h := range hunks {
		for _, line := range h.Lines {
			if line.Type == "remove" {
				removed++
			}
			if line.Type == "add" {
				added++
			}
		}
	}

	if removed != 1 {
		t.Errorf("expected 1 removed line, got %d", removed)
	}
	if added != 1 {
		t.Errorf("expected 1 added line, got %d", added)
	}
}

func TestComputeHunks_NoChange(t *testing.T) {
	content := []byte("line1\nline2\nline3")

	hunks := ComputeHunks(content, content)

	if len(hunks) != 0 {
		t.Errorf("expected no hunks for identical content, got %d", len(hunks))
	}
}

func TestFileChange_PopulateHunks_NewFile(t *testing.T) {
	fc := &FileChange{
		Path:       "test.txt",
		OldContent: nil,
		NewContent: []byte("line1\nline2"),
	}

	fc.PopulateHunks()

	if len(fc.Hunks) == 0 {
		t.Fatal("expected hunks for new file")
	}

	added := 0
	for _, h := range fc.Hunks {
		for _, line := range h.Lines {
			if line.Type == "add" {
				added++
			}
		}
	}

	if added != 2 {
		t.Errorf("expected 2 added lines for new file, got %d", added)
	}
}

func TestFileChange_PopulateHunks_DeletedFile(t *testing.T) {
	fc := &FileChange{
		Path:       "test.txt",
		OldContent: []byte("line1\nline2"),
		NewContent: nil,
	}

	fc.PopulateHunks()

	if len(fc.Hunks) == 0 {
		t.Fatal("expected hunks for deleted file")
	}

	removed := 0
	for _, h := range fc.Hunks {
		for _, line := range h.Lines {
			if line.Type == "remove" {
				removed++
			}
		}
	}

	if removed != 2 {
		t.Errorf("expected 2 removed lines for deleted file, got %d", removed)
	}
}

func TestComputeSummary(t *testing.T) {
	changes := []*FileChange{
		{
			Path: "file1.txt",
			Hunks: []Hunk{
				{
					Lines: []LineDiff{
						{Type: "remove", LineNo: 1, Content: "old"},
						{Type: "add", LineNo: 1, Content: "new"},
					},
				},
			},
		},
		{
			Path: "file2.txt",
			Hunks: []Hunk{
				{
					Lines: []LineDiff{
						{Type: "add", LineNo: 1, Content: "added"},
					},
				},
			},
		},
	}

	summary := ComputeSummary(changes)

	if summary.FilesModified != 2 {
		t.Errorf("expected 2 files modified, got %d", summary.FilesModified)
	}
	if summary.LinesRemoved != 1 {
		t.Errorf("expected 1 line removed, got %d", summary.LinesRemoved)
	}
	if summary.LinesAdded != 2 {
		t.Errorf("expected 2 lines added, got %d", summary.LinesAdded)
	}
}
