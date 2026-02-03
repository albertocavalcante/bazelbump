// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"strings"
)

// ComputeHunks computes the hunks (diff regions) between old and new content.
// Returns a slice of Hunk structs representing the changes.
func ComputeHunks(oldContent, newContent []byte) []Hunk {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// Use a simple LCS-based diff algorithm
	return computeDiff(oldLines, newLines)
}

// splitLines splits content into lines, preserving empty lines.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return []string{}
	}
	return strings.Split(string(content), "\n")
}

// computeDiff computes hunks using Myers' diff algorithm (simplified).
func computeDiff(oldLines, newLines []string) []Hunk {
	// Compute the edit script using LCS
	edits := computeEditScript(oldLines, newLines)

	// Group edits into hunks (contiguous changes with context)
	return groupIntoHunks(edits, oldLines, newLines, 0) // 0 context lines for compact output
}

// editOp represents a single edit operation
type editOp struct {
	opType  string // "equal", "delete", "insert"
	oldIdx  int    // index in old (for equal/delete)
	newIdx  int    // index in new (for equal/insert)
	content string
}

// computeEditScript computes the sequence of edits to transform old to new.
func computeEditScript(oldLines, newLines []string) []editOp {
	m, n := len(oldLines), len(newLines)

	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else if lcs[i-1][j] >= lcs[i][j-1] {
				lcs[i][j] = lcs[i-1][j]
			} else {
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}

	// Backtrack to get edit script
	var edits []editOp
	i, j := m, n

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			edits = append(edits, editOp{
				opType:  "equal",
				oldIdx:  i - 1,
				newIdx:  j - 1,
				content: oldLines[i-1],
			})
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			edits = append(edits, editOp{
				opType:  "insert",
				newIdx:  j - 1,
				content: newLines[j-1],
			})
			j--
		} else if i > 0 {
			edits = append(edits, editOp{
				opType:  "delete",
				oldIdx:  i - 1,
				content: oldLines[i-1],
			})
			i--
		}
	}

	// Reverse to get forward order
	for left, right := 0, len(edits)-1; left < right; left, right = left+1, right-1 {
		edits[left], edits[right] = edits[right], edits[left]
	}

	return edits
}

// groupIntoHunks groups edit operations into hunks with context lines.
func groupIntoHunks(edits []editOp, oldLines, newLines []string, contextLines int) []Hunk {
	var hunks []Hunk
	var currentLines []LineDiff

	oldLineNo := 1
	newLineNo := 1
	hunkOldStart := 0
	hunkNewStart := 0
	inHunk := false

	for _, edit := range edits {
		switch edit.opType {
		case "equal":
			if inHunk {
				// End current hunk if we've seen enough context
				hunks = append(hunks, Hunk{
					OldStart: hunkOldStart,
					OldCount: oldLineNo - hunkOldStart,
					NewStart: hunkNewStart,
					NewCount: newLineNo - hunkNewStart,
					Lines:    currentLines,
				})
				currentLines = nil
				inHunk = false
			}
			oldLineNo++
			newLineNo++

		case "delete":
			if !inHunk {
				inHunk = true
				hunkOldStart = oldLineNo
				hunkNewStart = newLineNo
			}
			currentLines = append(currentLines, LineDiff{
				Type:    "remove",
				LineNo:  oldLineNo,
				Content: edit.content,
			})
			oldLineNo++

		case "insert":
			if !inHunk {
				inHunk = true
				hunkOldStart = oldLineNo
				hunkNewStart = newLineNo
			}
			currentLines = append(currentLines, LineDiff{
				Type:    "add",
				LineNo:  newLineNo,
				Content: edit.content,
			})
			newLineNo++
		}
	}

	// Close any remaining hunk
	if inHunk && len(currentLines) > 0 {
		hunks = append(hunks, Hunk{
			OldStart: hunkOldStart,
			OldCount: oldLineNo - hunkOldStart,
			NewStart: hunkNewStart,
			NewCount: newLineNo - hunkNewStart,
			Lines:    currentLines,
		})
	}

	return hunks
}

// ChangeSummary summarizes the changes across multiple files.
type ChangeSummary struct {
	FilesModified int `json:"files"`
	LinesRemoved  int `json:"removed"`
	LinesAdded    int `json:"added"`
}

// ComputeSummary computes summary statistics from a list of file changes.
func ComputeSummary(changes []*FileChange) ChangeSummary {
	summary := ChangeSummary{}
	filesSet := make(map[string]bool)

	for _, change := range changes {
		if change == nil {
			continue
		}
		filesSet[change.Path] = true

		for _, hunk := range change.Hunks {
			for _, line := range hunk.Lines {
				switch line.Type {
				case "add":
					summary.LinesAdded++
				case "remove":
					summary.LinesRemoved++
				}
			}
		}
	}

	summary.FilesModified = len(filesSet)
	return summary
}

// PopulateHunks computes and populates the Hunks field of a FileChange.
// This should be called after setting OldContent and NewContent.
func (fc *FileChange) PopulateHunks() {
	if fc.OldContent == nil && fc.NewContent == nil {
		return
	}

	// Handle file creation
	if fc.OldContent == nil {
		newLines := splitLines(fc.NewContent)
		if len(newLines) > 0 {
			var lines []LineDiff
			for i, line := range newLines {
				lines = append(lines, LineDiff{
					Type:    "add",
					LineNo:  i + 1,
					Content: line,
				})
			}
			fc.Hunks = []Hunk{{
				OldStart: 1,
				OldCount: 0,
				NewStart: 1,
				NewCount: len(newLines),
				Lines:    lines,
			}}
		}
		return
	}

	// Handle file deletion
	if fc.NewContent == nil {
		oldLines := splitLines(fc.OldContent)
		if len(oldLines) > 0 {
			var lines []LineDiff
			for i, line := range oldLines {
				lines = append(lines, LineDiff{
					Type:    "remove",
					LineNo:  i + 1,
					Content: line,
				})
			}
			fc.Hunks = []Hunk{{
				OldStart: 1,
				OldCount: len(oldLines),
				NewStart: 1,
				NewCount: 0,
				Lines:    lines,
			}}
		}
		return
	}

	// Normal modification
	fc.Hunks = ComputeHunks(fc.OldContent, fc.NewContent)
}
