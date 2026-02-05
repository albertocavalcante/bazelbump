// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package flagdb provides access to Bazel flag data across versions.
//
// The flag data is synced from registry.build using tools/sync-flags
// and stored in data/flags/{version}/all.json.
//
// Example usage:
//
//	db, err := flagdb.Load("data/flags")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get flags for a specific version
//	flags, ok := db.Version("9.0.0")
//
//	// Find a specific flag
//	flag, ok := db.Flag("9.0.0", "experimental_enable_bzlmod")
//
//	// Compute diff between versions
//	diff := db.Diff("8.0.0", "9.0.0")
package flagdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/albertocavalcante/bazelbump/internal/version"
)

// Sentinel errors for flagdb operations.
var (
	// ErrFlagNotFound is returned when a flag does not exist in the database.
	ErrFlagNotFound = errors.New("flag not found")
	// ErrVersionNotFound is returned when a version does not exist in the database.
	ErrVersionNotFound = errors.New("version not found")
)

// Flag represents a Bazel command-line flag.
type Flag struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Type        string   `json:"type,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Sources     []string `json:"sources,omitempty"` // Commands where flag is available
}

// IsDeprecated returns true if the flag has the "deprecated" tag.
func (f Flag) IsDeprecated() bool {
	return slices.Contains(f.Tags, "deprecated")
}

// IsExperimental returns true if the flag has the "experimental" tag.
func (f Flag) IsExperimental() bool {
	return slices.Contains(f.Tags, "experimental")
}

// IsIncompatibleChange returns true if this is an --incompatible_* flag.
func (f Flag) IsIncompatibleChange() bool {
	return strings.HasPrefix(f.Name, "incompatible_")
}

// DB is the flag database containing flags for multiple Bazel versions.
type DB struct {
	basePath string
	versions []version.Version
	flags    map[string][]Flag // version string -> flags
	index    map[string]map[string]*Flag // version -> flag name -> flag
}

// Load loads the flag database from the given path.
func Load(basePath string) (*DB, error) {
	db := &DB{
		basePath: basePath,
		flags:    make(map[string][]Flag),
		index:    make(map[string]map[string]*Flag),
	}

	// Read versions.json
	versionsFile := filepath.Join(basePath, "versions.json")
	data, err := os.ReadFile(versionsFile)
	if err != nil {
		return nil, fmt.Errorf("read versions.json: %w", err)
	}

	var versionStrings []string
	if err := json.Unmarshal(data, &versionStrings); err != nil {
		return nil, fmt.Errorf("parse versions.json: %w", err)
	}

	// Parse and sort versions
	for _, vs := range versionStrings {
		v, err := version.Parse(vs)
		if err != nil {
			return nil, fmt.Errorf("parse version %q: %w", vs, err)
		}
		db.versions = append(db.versions, v)
	}
	slices.SortFunc(db.versions, version.Version.Compare)

	return db, nil
}

// Versions returns all available versions in sorted order.
func (db *DB) Versions() []version.Version {
	return db.versions
}

// HasVersion returns true if the given version is available.
func (db *DB) HasVersion(v string) bool {
	for _, ver := range db.versions {
		if ver.String() == v {
			return true
		}
	}
	return false
}

// Flags returns all flags for the given version.
// Returns nil if version not found or not yet loaded.
func (db *DB) Flags(v string) ([]Flag, error) {
	// Check cache
	if flags, ok := db.flags[v]; ok {
		return flags, nil
	}

	// Load from disk
	flags, err := db.loadVersion(v)
	if err != nil {
		return nil, err
	}

	db.flags[v] = flags
	db.buildIndex(v, flags)

	return flags, nil
}

// Flag returns a specific flag for the given version.
// Returns ErrFlagNotFound if the flag does not exist.
func (db *DB) Flag(v, name string) (*Flag, error) {
	// Ensure version is loaded
	if _, err := db.Flags(v); err != nil {
		return nil, err
	}

	if idx, ok := db.index[v]; ok {
		if f, ok := idx[name]; ok {
			return f, nil
		}
	}
	return nil, fmt.Errorf("%w: %s in version %s", ErrFlagNotFound, name, v)
}

// loadVersion loads flags for a specific version from disk.
func (db *DB) loadVersion(v string) ([]Flag, error) {
	if !db.HasVersion(v) {
		return nil, fmt.Errorf("%w: %s", ErrVersionNotFound, v)
	}

	flagsFile := filepath.Join(db.basePath, v, "all.json")
	f, err := os.Open(flagsFile)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", flagsFile, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", flagsFile, err)
	}

	var flags []Flag
	if err := json.Unmarshal(data, &flags); err != nil {
		return nil, fmt.Errorf("parse %s: %w", flagsFile, err)
	}

	return flags, nil
}

// buildIndex builds the name lookup index for a version.
func (db *DB) buildIndex(v string, flags []Flag) {
	idx := make(map[string]*Flag, len(flags))
	for i := range flags {
		idx[flags[i].Name] = &flags[i]
	}
	db.index[v] = idx
}

// Diff represents the differences between two Bazel versions.
type Diff struct {
	FromVersion string
	ToVersion   string
	Added       []Flag
	Removed     []Flag
	Changed     []FlagChange
}

// FlagChange represents a change in a flag between versions.
type FlagChange struct {
	Name   string
	From   *Flag
	To     *Flag
	Change ChangeType
}

// ChangeType categorizes the type of flag change.
type ChangeType int

const (
	ChangeUnknown ChangeType = iota
	ChangeDefaultValue
	ChangeDeprecated
	ChangeTypeChanged
	ChangeDescription
)

func (c ChangeType) String() string {
	switch c {
	case ChangeDefaultValue:
		return "default_changed"
	case ChangeDeprecated:
		return "deprecated"
	case ChangeTypeChanged:
		return "type_changed"
	case ChangeDescription:
		return "description_changed"
	default:
		return "unknown"
	}
}

// Diff computes the differences between two versions.
func (db *DB) Diff(fromVersion, toVersion string) (*Diff, error) {
	fromFlags, err := db.Flags(fromVersion)
	if err != nil {
		return nil, fmt.Errorf("load from version: %w", err)
	}

	toFlags, err := db.Flags(toVersion)
	if err != nil {
		return nil, fmt.Errorf("load to version: %w", err)
	}

	// Build maps for comparison
	fromMap := make(map[string]*Flag, len(fromFlags))
	for i := range fromFlags {
		fromMap[fromFlags[i].Name] = &fromFlags[i]
	}

	toMap := make(map[string]*Flag, len(toFlags))
	for i := range toFlags {
		toMap[toFlags[i].Name] = &toFlags[i]
	}

	diff := &Diff{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}

	// Find removed flags
	for name, fromFlag := range fromMap {
		if _, exists := toMap[name]; !exists {
			diff.Removed = append(diff.Removed, *fromFlag)
		}
	}

	// Find added flags and changes
	for name, toFlag := range toMap {
		fromFlag, exists := fromMap[name]
		if !exists {
			diff.Added = append(diff.Added, *toFlag)
			continue
		}

		// Check for changes
		if changes := detectChanges(fromFlag, toFlag); len(changes) > 0 {
			for _, ct := range changes {
				diff.Changed = append(diff.Changed, FlagChange{
					Name:   name,
					From:   fromFlag,
					To:     toFlag,
					Change: ct,
				})
			}
		}
	}

	return diff, nil
}

// detectChanges returns the types of changes between two flags.
func detectChanges(from, to *Flag) []ChangeType {
	var changes []ChangeType

	if from.Default != to.Default {
		changes = append(changes, ChangeDefaultValue)
	}

	if !from.IsDeprecated() && to.IsDeprecated() {
		changes = append(changes, ChangeDeprecated)
	}

	if from.Type != to.Type {
		changes = append(changes, ChangeTypeChanged)
	}

	return changes
}

// IncompatibleFlags returns all --incompatible_* flags for a version.
func (db *DB) IncompatibleFlags(v string) ([]Flag, error) {
	flags, err := db.Flags(v)
	if err != nil {
		return nil, err
	}

	var result []Flag
	for _, f := range flags {
		if f.IsIncompatibleChange() {
			result = append(result, f)
		}
	}
	return result, nil
}

// DeprecatedFlags returns all deprecated flags for a version.
func (db *DB) DeprecatedFlags(v string) ([]Flag, error) {
	flags, err := db.Flags(v)
	if err != nil {
		return nil, err
	}

	var result []Flag
	for _, f := range flags {
		if f.IsDeprecated() {
			result = append(result, f)
		}
	}
	return result, nil
}

// ExperimentalFlags returns all experimental flags for a version.
func (db *DB) ExperimentalFlags(v string) ([]Flag, error) {
	flags, err := db.Flags(v)
	if err != nil {
		return nil, err
	}

	var result []Flag
	for _, f := range flags {
		if f.IsExperimental() {
			result = append(result, f)
		}
	}
	return result, nil
}
