// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"

	"github.com/albertocavalcante/starlark-syntax-go/bzl"
)

// LoadPosition specifies where to insert new load statements.
type LoadPosition int

const (
	// LoadPositionTop inserts at the top, after any leading comments/docstring.
	LoadPositionTop LoadPosition = iota
	// LoadPositionWithExisting inserts after the last existing load statement.
	LoadPositionWithExisting
	// LoadPositionAuto uses smart placement based on load source type.
	LoadPositionAuto
)

// EnsureLoadOptions configures the ensure_load operation.
type EnsureLoadOptions struct {
	// Module is the load source (e.g., "@rules_cc//cc:defs.bzl").
	Module string

	// Symbols are the symbols to load.
	Symbols []string

	// Position specifies where to insert new loads.
	Position LoadPosition

	// GroupWith optionally specifies a module prefix to group near.
	GroupWith string
}

// EnsureLoad ensures symbols are loaded from a module.
// Returns true if the file was modified.
//
// This is AST-aware and correctly handles multi-line load statements,
// unlike text-based manipulation which fails on:
//
//	load(
//	    "//bazel:envoy_build_system.bzl",
//	    "envoy_cc_library",
//	)
func EnsureLoad(f *File, opts EnsureLoadOptions) (bool, error) {
	// 1. Find existing load for this module
	existingLoad := findLoadByModule(f.File, opts.Module)

	if existingLoad != nil {
		// 2a. Add missing symbols to existing load
		added, err := addSymbolsToLoad(existingLoad, opts.Symbols)
		if err != nil {
			return false, err
		}
		if added {
			f.modified = true
		}
		return added, nil
	}

	// 2b. Create new load statement
	newLoad := createLoadStmt(opts.Module, opts.Symbols)

	// 3. Find insertion position
	pos := findLoadInsertPosition(f.File, opts)

	// 4. Insert the new load
	insertStatement(f.File, pos, newLoad)
	f.modified = true

	return true, nil
}

// RemoveLoadOptions configures the remove_load operation.
type RemoveLoadOptions struct {
	// Module optionally limits removal to a specific module.
	Module string

	// Symbols are the symbols to remove.
	Symbols []string
}

// RemoveLoad removes symbols from load statements.
// Returns true if the file was modified.
func RemoveLoad(f *File, opts RemoveLoadOptions) (bool, error) {
	modified := false

	for i := 0; i < len(f.File.Stmt); i++ {
		load, ok := f.File.Stmt[i].(*bzl.LoadStmt)
		if !ok {
			continue
		}

		// Check module filter
		if opts.Module != "" && load.Module.Value != opts.Module {
			continue
		}

		// Remove specified symbols
		changed := removeSymbolsFromLoad(load, opts.Symbols)
		if changed {
			modified = true
		}

		// If load is now empty, remove it entirely
		if len(load.From) == 0 {
			// Preserve any comments by attaching to next statement
			if i+1 < len(f.File.Stmt) && len(load.Comment().After) > 0 {
				next := f.File.Stmt[i+1]
				next.Comment().Before = append(load.Comment().After, next.Comment().Before...)
			}
			f.File.Stmt = append(f.File.Stmt[:i], f.File.Stmt[i+1:]...)
			i-- // Adjust index after removal
		}
	}

	f.modified = f.modified || modified
	return modified, nil
}

// MergeLoads combines load statements from the same module.
// Returns true if the file was modified.
func MergeLoads(f *File) (bool, error) {
	// Group loads by module
	type loadInfo struct {
		stmt  *bzl.LoadStmt
		index int
	}
	loadsByModule := make(map[string][]*loadInfo)

	for i, stmt := range f.File.Stmt {
		if load, ok := stmt.(*bzl.LoadStmt); ok {
			module := load.Module.Value
			loadsByModule[module] = append(loadsByModule[module], &loadInfo{load, i})
		}
	}

	modified := false

	// Track indices to remove (in reverse order to avoid shifting issues)
	var toRemove []int

	for _, loads := range loadsByModule {
		if len(loads) <= 1 {
			continue
		}

		// Merge all into first load
		first := loads[0].stmt
		for _, info := range loads[1:] {
			// Merge symbols
			for i := range info.stmt.From {
				first.From = append(first.From, info.stmt.From[i])
				first.To = append(first.To, info.stmt.To[i])
			}

			// Merge comments
			first.Comment().Before = append(first.Comment().Before, info.stmt.Comment().Before...)

			// Mark for removal
			toRemove = append(toRemove, info.index)
		}

		// Sort and deduplicate
		SortLoadArgs(first)
		deduplicateLoadSymbols(first)
		modified = true
	}

	// Remove merged loads in reverse order
	for i := len(toRemove) - 1; i >= 0; i-- {
		idx := toRemove[i]
		f.File.Stmt = append(f.File.Stmt[:idx], f.File.Stmt[idx+1:]...)
	}

	f.modified = f.modified || modified
	return modified, nil
}

// SortLoads sorts all load statements in the file.
// External loads (@) come first, then workspace loads (//), then relative.
func SortLoads(f *File) (bool, error) {
	// Find the load block
	firstLoad := -1
	lastLoad := -1

	for i, stmt := range f.File.Stmt {
		if _, ok := stmt.(*bzl.LoadStmt); ok {
			if firstLoad == -1 {
				firstLoad = i
			}
			lastLoad = i
		} else if firstLoad >= 0 && lastLoad == i-1 {
			// Found a non-load after loads, stop looking
			break
		}
	}

	if firstLoad < 0 || firstLoad == lastLoad {
		return false, nil // 0 or 1 loads, nothing to sort
	}

	// Extract load statements
	loads := make([]*bzl.LoadStmt, 0, lastLoad-firstLoad+1)
	for i := firstLoad; i <= lastLoad; i++ {
		if load, ok := f.File.Stmt[i].(*bzl.LoadStmt); ok {
			loads = append(loads, load)
		}
	}

	// Sort by: external (@) < workspace (//) < relative
	sorted := make([]*bzl.LoadStmt, len(loads))
	copy(sorted, loads)

	// Simple bubble sort for stability
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if compareLoadOrder(sorted[i], sorted[j]) > 0 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Check if order changed
	changed := false
	for i := range loads {
		if loads[i] != sorted[i] {
			changed = true
			break
		}
	}

	if !changed {
		return false, nil
	}

	// Replace loads in the file
	loadIdx := 0
	for i := firstLoad; i <= lastLoad; i++ {
		if _, ok := f.File.Stmt[i].(*bzl.LoadStmt); ok {
			f.File.Stmt[i] = sorted[loadIdx]
			loadIdx++
		}
	}

	f.modified = true
	return true, nil
}

// compareLoadOrder returns negative if a should come before b.
func compareLoadOrder(a, b *bzl.LoadStmt) int {
	aModule := a.Module.Value
	bModule := b.Module.Value

	aType := loadType(aModule)
	bType := loadType(bModule)

	if aType != bType {
		return aType - bType
	}

	// Same type, sort alphabetically
	if aModule < bModule {
		return -1
	}
	if aModule > bModule {
		return 1
	}
	return 0
}

// loadType returns a sort key for the load type.
func loadType(module string) int {
	if strings.HasPrefix(module, "@") {
		return 0 // External
	}
	if strings.HasPrefix(module, "//") {
		return 1 // Workspace
	}
	return 2 // Relative
}

// findLoadByModule finds a LoadStmt importing from the given module.
func findLoadByModule(f *bzl.File, module string) *bzl.LoadStmt {
	for _, stmt := range f.Stmt {
		if load, ok := stmt.(*bzl.LoadStmt); ok {
			if load.Module.Value == module {
				return load
			}
		}
	}
	return nil
}

// addSymbolsToLoad adds symbols to an existing LoadStmt.
// Returns true if any symbols were added.
func addSymbolsToLoad(load *bzl.LoadStmt, symbols []string) (bool, error) {
	added := false
	existing := make(map[string]bool)

	// Build set of already-loaded symbols
	for _, to := range load.To {
		existing[to.Name] = true
	}

	// Add missing symbols
	for _, sym := range symbols {
		if !existing[sym] {
			ident := &bzl.Ident{Name: sym}
			load.From = append(load.From, ident)
			load.To = append(load.To, ident)
			added = true
		}
	}

	if added {
		// Sort symbols for consistency
		SortLoadArgs(load)
	}

	return added, nil
}

// createLoadStmt creates a new LoadStmt.
func createLoadStmt(module string, symbols []string) *bzl.LoadStmt {
	load := &bzl.LoadStmt{
		Module:       &bzl.StringExpr{Value: module},
		ForceCompact: len(symbols) <= 3, // Single line if few symbols
	}

	for _, sym := range symbols {
		ident := &bzl.Ident{Name: sym}
		load.From = append(load.From, ident)
		load.To = append(load.To, ident)
	}

	// Sort for consistency
	SortLoadArgs(load)

	return load
}

// findLoadInsertPosition determines where to insert a new load.
func findLoadInsertPosition(f *bzl.File, opts EnsureLoadOptions) int {
	switch opts.Position {
	case LoadPositionTop:
		// After any leading comments/docstring
		return findFirstStatementPosition(f)

	case LoadPositionWithExisting:
		// After last existing load
		lastLoad := findLastLoadPosition(f)
		if lastLoad >= 0 {
			return lastLoad + 1
		}
		return findFirstStatementPosition(f)

	case LoadPositionAuto:
		// Smart placement: group with similar loads
		return findSmartInsertPosition(f, opts.Module)
	}

	return 0
}

// findFirstStatementPosition returns the index for inserting at "top".
// Accounts for leading comments and docstrings.
func findFirstStatementPosition(f *bzl.File) int {
	for i, stmt := range f.Stmt {
		// Skip comment blocks
		if _, ok := stmt.(*bzl.CommentBlock); ok {
			continue
		}
		// Skip docstrings (string at top of file)
		if str, ok := stmt.(*bzl.StringExpr); ok && i == 0 && str.TripleQuote {
			continue
		}
		return i
	}
	return len(f.Stmt) // Empty file, insert at end
}

// findLastLoadPosition returns the index of the last load statement.
// Returns -1 if no loads exist.
func findLastLoadPosition(f *bzl.File) int {
	lastIdx := -1
	for i, stmt := range f.Stmt {
		if _, ok := stmt.(*bzl.LoadStmt); ok {
			lastIdx = i
		}
	}
	return lastIdx
}

// findSmartInsertPosition finds the best position for a new load.
// Groups loads by: external (@), workspace-relative (//), relative.
func findSmartInsertPosition(f *bzl.File, module string) int {
	isExternal := strings.HasPrefix(module, "@")
	isWorkspace := strings.HasPrefix(module, "//")

	// Find position among similar loads
	lastSimilar := -1
	lastLoad := -1

	for i, stmt := range f.Stmt {
		load, ok := stmt.(*bzl.LoadStmt)
		if !ok {
			continue
		}
		lastLoad = i

		modIsExternal := strings.HasPrefix(load.Module.Value, "@")
		modIsWorkspace := strings.HasPrefix(load.Module.Value, "//")

		if isExternal == modIsExternal && isWorkspace == modIsWorkspace {
			lastSimilar = i
		}
	}

	if lastSimilar >= 0 {
		return lastSimilar + 1
	}

	// No similar loads, insert at end of load block or at top
	if lastLoad >= 0 {
		return lastLoad + 1
	}

	return findFirstStatementPosition(f)
}

// insertStatement inserts a statement at the given position.
func insertStatement(f *bzl.File, pos int, stmt bzl.Expr) {
	if pos >= len(f.Stmt) {
		f.Stmt = append(f.Stmt, stmt)
		return
	}
	f.Stmt = append(f.Stmt, nil) // Make room
	copy(f.Stmt[pos+1:], f.Stmt[pos:])
	f.Stmt[pos] = stmt
}

// removeSymbolsFromLoad removes symbols from a load statement.
func removeSymbolsFromLoad(load *bzl.LoadStmt, symbols []string) bool {
	toRemove := make(map[string]bool)
	for _, s := range symbols {
		toRemove[s] = true
	}

	newFrom := make([]*bzl.Ident, 0, len(load.From))
	newTo := make([]*bzl.Ident, 0, len(load.To))

	for i := range load.From {
		if !toRemove[load.To[i].Name] {
			newFrom = append(newFrom, load.From[i])
			newTo = append(newTo, load.To[i])
		}
	}

	if len(newFrom) == len(load.From) {
		return false // Nothing removed
	}

	load.From = newFrom
	load.To = newTo
	return true
}

// deduplicateLoadSymbols removes duplicate symbols from a load.
func deduplicateLoadSymbols(load *bzl.LoadStmt) {
	seen := make(map[string]bool)
	newFrom := make([]*bzl.Ident, 0, len(load.From))
	newTo := make([]*bzl.Ident, 0, len(load.To))

	for i := range load.From {
		key := load.To[i].Name
		if !seen[key] {
			seen[key] = true
			newFrom = append(newFrom, load.From[i])
			newTo = append(newTo, load.To[i])
		}
	}

	load.From = newFrom
	load.To = newTo
}
