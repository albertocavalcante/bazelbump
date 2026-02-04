// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// envoy-ast-test validates AST-based transformations on the Envoy codebase.
// This tests that our EnsureLoad implementation correctly handles multi-line
// load statements that broke 96% of Envoy BUILD files with text-based approach.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/albertocavalcante/bazelbump/ast"
)

type Result struct {
	Path         string
	Success      bool
	Error        string
	HasMultiLine bool
	WasModified  bool
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: envoy-ast-test <envoy-path>")
		os.Exit(1)
	}

	envoyPath := os.Args[1]

	// Find all BUILD files
	var buildFiles []string
	err := filepath.Walk(envoyPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "BUILD" || name == "BUILD.bazel" {
			buildFiles = append(buildFiles, path)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d BUILD files\n", len(buildFiles))
	fmt.Println("Testing AST-based EnsureLoad on each file...")
	fmt.Println()

	// Process files concurrently
	results := make(chan Result, len(buildFiles))
	var wg sync.WaitGroup

	// Use worker pool
	workers := 8
	fileChan := make(chan string, len(buildFiles))

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				result := testFile(path)
				results <- result
			}
		}()
	}

	// Send files to workers
	for _, path := range buildFiles {
		fileChan <- path
	}
	close(fileChan)

	// Wait for completion
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var (
		totalCount      int64
		successCount    int64
		failCount       int64
		multiLineCount  int64
		modifiedCount   int64
		failures        []Result
	)

	for result := range results {
		atomic.AddInt64(&totalCount, 1)
		if result.Success {
			atomic.AddInt64(&successCount, 1)
		} else {
			atomic.AddInt64(&failCount, 1)
			failures = append(failures, result)
		}
		if result.HasMultiLine {
			atomic.AddInt64(&multiLineCount, 1)
		}
		if result.WasModified {
			atomic.AddInt64(&modifiedCount, 1)
		}
	}

	// Print results
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("RESULTS")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Printf("Total files tested:     %d\n", totalCount)
	fmt.Printf("Successful:             %d (%.1f%%)\n", successCount, float64(successCount)/float64(totalCount)*100)
	fmt.Printf("Failed:                 %d (%.1f%%)\n", failCount, float64(failCount)/float64(totalCount)*100)
	fmt.Printf("Had multi-line loads:   %d (%.1f%%)\n", multiLineCount, float64(multiLineCount)/float64(totalCount)*100)
	fmt.Printf("Were modified:          %d\n", modifiedCount)
	fmt.Println()

	if failCount > 0 {
		fmt.Println("FAILURES:")
		fmt.Println("-" + strings.Repeat("-", 69))
		maxShow := 20
		for i, f := range failures {
			if i >= maxShow {
				fmt.Printf("... and %d more failures\n", len(failures)-maxShow)
				break
			}
			fmt.Printf("  %s\n    Error: %s\n", f.Path, f.Error)
		}
		fmt.Println()
	}

	// Success criteria
	successRate := float64(successCount) / float64(totalCount) * 100
	if successRate == 100 {
		fmt.Println("✓ SUCCESS: All files passed! AST-based transformations work correctly.")
		fmt.Println("  The multi-line load bug that broke 96% of files is FIXED.")
	} else if successRate >= 99 {
		fmt.Printf("~ MOSTLY SUCCESS: %.1f%% passed. Some edge cases to investigate.\n", successRate)
	} else {
		fmt.Printf("✗ FAILURE: Only %.1f%% passed. Investigation needed.\n", successRate)
		os.Exit(1)
	}
}

func testFile(path string) Result {
	result := Result{Path: path}

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		result.Error = fmt.Sprintf("read error: %v", err)
		return result
	}

	// Check if file has multi-line load (the problematic pattern)
	result.HasMultiLine = hasMultiLineLoad(string(content))

	// Parse with AST
	f, err := ast.ParseFileContent(path, content)
	if err != nil {
		result.Error = fmt.Sprintf("parse error: %v", err)
		return result
	}

	// Apply EnsureLoad (the operation that broke files before)
	modified, err := ast.EnsureLoad(f, ast.EnsureLoadOptions{
		Module:   "@rules_cc//cc:defs.bzl",
		Symbols:  []string{"cc_test_marker_for_ast_validation"},
		Position: ast.LoadPositionTop,
	})
	if err != nil {
		result.Error = fmt.Sprintf("ensure_load error: %v", err)
		return result
	}
	result.WasModified = modified

	// Format output
	output := f.Format()

	// CRITICAL: Validate output is still valid Starlark
	_, err = ast.ParseFileContent(path, output)
	if err != nil {
		result.Error = fmt.Sprintf("OUTPUT INVALID: %v", err)
		return result
	}

	// Additional check: new load should NOT be inside any existing load
	outputStr := string(output)
	if strings.Contains(outputStr, "load(\nload(") || strings.Contains(outputStr, "load(\n    load(") {
		result.Error = "BUG: new load appears inside existing load statement"
		return result
	}

	result.Success = true
	return result
}

func hasMultiLineLoad(content string) bool {
	// Simple heuristic: load( followed by newline before )
	lines := strings.Split(content, "\n")
	inLoad := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "load(") && !strings.Contains(trimmed, ")") {
			inLoad = true
		}
		if inLoad && strings.HasPrefix(trimmed, ")") {
			return true
		}
	}
	return false
}
