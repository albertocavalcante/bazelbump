// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// sync-flags syncs Bazel flag data from registry.build to data/flags/.
//
// Usage:
//
//	go run ./tools/sync-flags                     # Sync all versions from default path
//	go run ./tools/sync-flags -source /path/to   # Sync from specific path
//	go run ./tools/sync-flags -version 9.0.0     # Sync specific version only
//	go run ./tools/sync-flags -list              # List available versions
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Default source path for registry.build data
const defaultSourcePath = "/Users/adsc/dev/temp/registry.build/flag/bazel"

// Flag represents a single Bazel command-line flag.
type Flag struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Type        string   `json:"type,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Sources     []string `json:"sources,omitempty"`
}

// VersionMetadata contains metadata about synced version data.
type VersionMetadata struct {
	Source    string    `json:"source"`
	SyncedAt  time.Time `json:"synced_at"`
	Tool      string    `json:"tool"`
	FlagCount int       `json:"flag_count"`
}

// SyncMetadata contains overall sync metadata.
type SyncMetadata struct {
	Source       string    `json:"source"`
	SyncedAt     time.Time `json:"synced_at"`
	Tool         string    `json:"tool"`
	VersionCount int       `json:"version_count"`
	Versions     []string  `json:"versions"`
}

func main() {
	sourcePath := flag.String("source", defaultSourcePath, "Path to registry.build flag data")
	version := flag.String("version", "", "Sync specific version only")
	listOnly := flag.Bool("list", false, "List available versions and exit")
	destPath := flag.String("dest", "data/flags", "Destination path for synced data")
	flag.Parse()

	// List available versions
	versions, err := listVersions(*sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing versions: %v\n", err)
		os.Exit(1)
	}

	if *listOnly {
		fmt.Printf("Available versions (%d):\n", len(versions))
		for _, v := range versions {
			fmt.Printf("  %s\n", v)
		}
		return
	}

	// Filter to specific version if requested
	if *version != "" {
		if !slices.Contains(versions, *version) {
			fmt.Fprintf(os.Stderr, "Error: version %q not found\n", *version)
			fmt.Fprintf(os.Stderr, "Available: %v\n", versions)
			os.Exit(1)
		}
		versions = []string{*version}
	}

	// Create destination directory
	if err := os.MkdirAll(*destPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dest directory: %v\n", err)
		os.Exit(1)
	}

	// Sync each version
	fmt.Printf("Syncing %d version(s) from %s to %s\n", len(versions), *sourcePath, *destPath)
	for _, v := range versions {
		if err := syncVersion(*sourcePath, *destPath, v); err != nil {
			fmt.Fprintf(os.Stderr, "Error syncing %s: %v\n", v, err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ %s\n", v)
	}

	// Write overall metadata
	meta := SyncMetadata{
		Source:       *sourcePath,
		SyncedAt:     time.Now().UTC(),
		Tool:         "tools/sync-flags",
		VersionCount: len(versions),
		Versions:     versions,
	}
	if err := writeJSON(filepath.Join(*destPath, "VERSION.json"), meta); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing VERSION.json: %v\n", err)
		os.Exit(1)
	}

	// Write versions index
	if err := writeJSON(filepath.Join(*destPath, "versions.json"), versions); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing versions.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSync complete: %d versions\n", len(versions))
}

// listVersions returns sorted list of available Bazel versions.
func listVersions(sourcePath string) ([]string, error) {
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source dir: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() && isVersion(e.Name()) {
			versions = append(versions, e.Name())
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})

	return versions, nil
}

// isVersion checks if a directory name looks like a version.
func isVersion(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Versions start with a digit
	return name[0] >= '0' && name[0] <= '9'
}

// compareVersions compares two version strings.
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := max(len(partsA), len(partsB))

	for i := range maxLen {
		var numA, numB int
		if i < len(partsA) {
			fmt.Sscanf(partsA[i], "%d", &numA)
		}
		if i < len(partsB) {
			fmt.Sscanf(partsB[i], "%d", &numB)
		}
		if numA != numB {
			return numA - numB
		}
	}
	return 0
}

// syncVersion syncs a single version's flag data.
func syncVersion(sourcePath, destPath, version string) error {
	srcFile := filepath.Join(sourcePath, version, "all.json")
	dstDir := filepath.Join(destPath, version)
	dstFile := filepath.Join(dstDir, "all.json")

	// Create version directory
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("create version dir: %w", err)
	}

	// Read source flags
	flags, err := readFlags(srcFile)
	if err != nil {
		return fmt.Errorf("read flags: %w", err)
	}

	// Write flags to destination
	if err := writeJSON(dstFile, flags); err != nil {
		return fmt.Errorf("write flags: %w", err)
	}

	// Write version metadata
	meta := VersionMetadata{
		Source:    srcFile,
		SyncedAt:  time.Now().UTC(),
		Tool:      "tools/sync-flags",
		FlagCount: len(flags),
	}
	if err := writeJSON(filepath.Join(dstDir, "VERSION.json"), meta); err != nil {
		return fmt.Errorf("write version metadata: %w", err)
	}

	return nil
}

// readFlags reads flags from a JSON file.
func readFlags(path string) ([]Flag, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var flags []Flag
	if err := json.Unmarshal(data, &flags); err != nil {
		return nil, err
	}

	return flags, nil
}

// writeJSON writes data as indented JSON.
func writeJSON(path string, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
