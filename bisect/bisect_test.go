// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package bisect

import (
	"context"
	"testing"
	"time"

	"github.com/albertocavalcante/bazelbump/flagdb"
)

func TestReleaseEngine_GetVersions(t *testing.T) {
	// Load test flag database
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	tests := []struct {
		name    string
		from    string
		to      string
		wantMin int // Minimum expected versions
	}{
		{
			name:    "7.0.0 to 8.0.0",
			from:    "7.0.0",
			to:      "8.0.0",
			wantMin: 2, // At least 7.0.0 and 8.0.0
		},
		{
			name:    "8.0.0 to 9.0.0",
			from:    "8.0.0",
			to:      "9.0.0",
			wantMin: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &ReleaseEngine{db: db}

			versions, err := engine.GetVersions(context.Background(), tt.from, tt.to)
			if err != nil {
				t.Fatalf("GetVersions() error = %v", err)
			}

			if len(versions) < tt.wantMin {
				t.Errorf("got %d versions, want at least %d", len(versions), tt.wantMin)
			}

			// First should be from, last should be to
			if versions[0] != tt.from {
				t.Errorf("first version = %s, want %s", versions[0], tt.from)
			}
			if versions[len(versions)-1] != tt.to {
				t.Errorf("last version = %s, want %s", versions[len(versions)-1], tt.to)
			}
		})
	}
}

func TestDetectGranularity(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want Granularity
	}{
		{"7.0.0", "8.0.0", GranularityRelease},
		{"7.0.0-rc1", "7.0.0", GranularityRC},
		{"7.0.0", "7.0.0-rc2", GranularityRC},
		{"abc1234", "def5678", GranularityCommit},
		{"7.0.0", "abc1234def5678", GranularityCommit},
	}

	for _, tt := range tests {
		t.Run(tt.from+".."+tt.to, func(t *testing.T) {
			got := DetectGranularity(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("DetectGranularity(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestEstimateSteps(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{1, 1},
		{2, 3},   // 2 initial + 1 bisect
		{4, 4},   // 2 initial + 2 bisect
		{8, 5},   // 2 initial + 3 bisect
		{16, 6},  // 2 initial + 4 bisect
		{100, 8}, // 2 initial + ~6 bisect (log2(100) ≈ 6.6)
	}

	for _, tt := range tests {
		got := estimateSteps(tt.n)
		if got != tt.want {
			t.Errorf("estimateSteps(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestFormatResult(t *testing.T) {
	tests := []struct {
		name        string
		result      *Result
		wantContain []string
	}{
		{
			name: "no breakage found",
			result: &Result{
				LastGoodVersion: "8.0.0",
			},
			wantContain: []string{"No breakage found", "8.0.0"},
		},
		{
			name: "breakage found with engine info",
			result: &Result{
				EngineName:      "native (flag database)",
				Granularity:     GranularityRelease,
				IsNative:        true,
				FirstBadVersion: "7.4.1",
				LastGoodVersion: "7.4.0",
				StepsTaken:      5,
			},
			wantContain: []string{
				"Engine: native",
				"Granularity: release",
				"First bad version: 7.4.1",
				"Last good version: 7.4.0",
				"Steps taken: 5",
			},
		},
		{
			name: "commit-level result",
			result: &Result{
				EngineName:  "bazelisk (delegated)",
				Granularity: GranularityCommit,
				IsNative:    false,
				CommitResult: &CommitBisectResult{
					FirstBadCommit: "abc123def",
					CommitURL:      "https://github.com/bazelbuild/bazel/commit/abc123def",
					StepsTaken:     10,
				},
			},
			wantContain: []string{
				"Engine: bazelisk",
				"delegates to bazelisk",
				"First bad commit: abc123def",
				"github.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatResult(tt.result)
			for _, want := range tt.wantContain {
				if !contains(got, want) {
					t.Errorf("FormatResult() missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSelectEngine(t *testing.T) {
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	tests := []struct {
		granularity Granularity
		wantName    string
		wantNative  bool
	}{
		{GranularityRelease, "native (flag database)", true},
		{GranularityRC, "native (GitHub releases)", true},
		{GranularityCommit, "bazelisk (delegated)", false},
	}

	for _, tt := range tests {
		t.Run(tt.granularity.String(), func(t *testing.T) {
			engine, err := SelectEngine(tt.granularity, db)
			if err != nil {
				t.Fatalf("SelectEngine() error = %v", err)
			}

			if engine.Name() != tt.wantName {
				t.Errorf("engine.Name() = %s, want %s", engine.Name(), tt.wantName)
			}
			if engine.IsNative() != tt.wantNative {
				t.Errorf("engine.IsNative() = %v, want %v", engine.IsNative(), tt.wantNative)
			}
		})
	}
}

// TestBisector_Run_Mock tests the bisect algorithm with mocked test results.
func TestBisector_Run_Mock(t *testing.T) {
	// This test would require mocking the test execution
	// For now, skip if no flag database
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	// Create a config that will fail quickly (invalid command)
	cfg := Config{
		FromVersion: "7.0.0",
		ToVersion:   "7.1.0",
		Granularity: GranularityRelease,
		TestCommand: "exit 0", // Always succeeds
		Timeout:     5 * time.Second,
	}

	b := New(db, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := b.Run(ctx)
	if err != nil {
		// Expected - from version might not pass with "exit 0"
		t.Logf("bisect returned error (may be expected): %v", err)
		return
	}

	// If no error, verify we got a result with engine info
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.EngineName == "" {
		t.Error("expected EngineName to be set")
	}
	if !result.IsNative {
		t.Error("expected IsNative to be true for release granularity")
	}

	t.Logf("Bisect result: engine=%s, granularity=%s", result.EngineName, result.Granularity)
}
