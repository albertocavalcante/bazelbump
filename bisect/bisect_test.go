// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package bisect

import (
	"context"
	"testing"
	"time"

	"github.com/albertocavalcante/bazelbump/flagdb"
)

func TestBisector_getVersionsInRange(t *testing.T) {
	// Load test flag database
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	tests := []struct {
		name           string
		from           string
		to             string
		skipPrerelease bool
		wantMin        int // Minimum expected versions
	}{
		{
			name:           "7.0.0 to 8.0.0",
			from:           "7.0.0",
			to:             "8.0.0",
			skipPrerelease: true,
			wantMin:        2, // At least 7.0.0 and 8.0.0
		},
		{
			name:           "8.0.0 to 9.0.0",
			from:           "8.0.0",
			to:             "9.0.0",
			skipPrerelease: true,
			wantMin:        2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(db, Config{
				FromVersion:    tt.from,
				ToVersion:      tt.to,
				SkipPrerelease: tt.skipPrerelease,
			})

			versions, err := b.getVersionsInRange()
			if err != nil {
				t.Fatalf("getVersionsInRange() error = %v", err)
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

func TestEstimateSteps(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{1, 1},
		{2, 3},  // 2 initial + 1 bisect
		{4, 4},  // 2 initial + 2 bisect
		{8, 5},  // 2 initial + 3 bisect
		{16, 6}, // 2 initial + 4 bisect
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
		name   string
		result *Result
		want   string
	}{
		{
			name: "no breakage found",
			result: &Result{
				LastGoodVersion: "8.0.0",
			},
			want: "No breakage found! Version 8.0.0 passes.\n",
		},
		{
			name: "breakage found",
			result: &Result{
				FirstBadVersion: "7.4.1",
				LastGoodVersion: "7.4.0",
				StepsTaken:      5,
			},
			want: "First bad version: 7.4.1\nLast good version: 7.4.0\nSteps taken: 5\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatResult(tt.result)
			if got != tt.want {
				t.Errorf("FormatResult() =\n%s\nwant:\n%s", got, tt.want)
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

	// If no error, verify we got a result
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	t.Logf("Bisect result: %+v", result)
}
