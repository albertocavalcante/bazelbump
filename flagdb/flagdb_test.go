// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package flagdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Find data directory relative to test
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := Load(dataDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	versions := db.Versions()
	if len(versions) == 0 {
		t.Error("expected at least one version")
	}

	t.Logf("Loaded %d versions", len(versions))
}

func TestFlags(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := Load(dataDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Test loading flags for a version
	if !db.HasVersion("9.0.0") {
		t.Skip("version 9.0.0 not available")
	}

	flags, err := db.Flags("9.0.0")
	if err != nil {
		t.Fatalf("Flags() error = %v", err)
	}

	if len(flags) == 0 {
		t.Error("expected flags for version 9.0.0")
	}

	t.Logf("Version 9.0.0 has %d flags", len(flags))

	// Test flag lookup
	flag, err := db.Flag("9.0.0", "keep_going")
	if err != nil {
		t.Fatalf("Flag() error = %v", err)
	}
	if flag == nil {
		t.Error("expected to find --keep_going flag")
	}
}

func TestDiff(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := Load(dataDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !db.HasVersion("8.0.0") || !db.HasVersion("9.0.0") {
		t.Skip("versions 8.0.0 and 9.0.0 not available")
	}

	diff, err := db.Diff("8.0.0", "9.0.0")
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	t.Logf("Diff 8.0.0 -> 9.0.0:")
	t.Logf("  Added: %d flags", len(diff.Added))
	t.Logf("  Removed: %d flags", len(diff.Removed))
	t.Logf("  Changed: %d flags", len(diff.Changed))

	// Log some removed flags for visibility
	if len(diff.Removed) > 0 {
		t.Log("  Sample removed flags:")
		for i, f := range diff.Removed {
			if i >= 5 {
				break
			}
			t.Logf("    - %s", f.Name)
		}
	}
}

func TestIncompatibleFlags(t *testing.T) {
	dataDir := findDataDir(t)
	if dataDir == "" {
		t.Skip("data/flags directory not found")
	}

	db, err := Load(dataDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !db.HasVersion("9.0.0") {
		t.Skip("version 9.0.0 not available")
	}

	flags, err := db.IncompatibleFlags("9.0.0")
	if err != nil {
		t.Fatalf("IncompatibleFlags() error = %v", err)
	}

	t.Logf("Version 9.0.0 has %d incompatible flags", len(flags))
}

func findDataDir(t *testing.T) string {
	t.Helper()

	// Try relative to current directory
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
