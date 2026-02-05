// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package bisect

import (
	"context"
	"strings"
	"testing"

	"github.com/albertocavalcante/bazelbump/flagdb"
)

func TestValidateVersion_Release(t *testing.T) {
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	tests := []struct {
		name        string
		version     string
		wantErr     bool
		wantSuggest bool // should have suggestions
	}{
		{
			name:    "valid version 7.0.0",
			version: "7.0.0",
			wantErr: false,
		},
		{
			name:    "valid version 8.0.0",
			version: "8.0.0",
			wantErr: false,
		},
		{
			name:        "invalid version 8.12.0",
			version:     "8.12.0",
			wantErr:     true,
			wantSuggest: true,
		},
		{
			name:        "invalid version 99.0.0",
			version:     "99.0.0",
			wantErr:     true,
			wantSuggest: true,
		},
		{
			name:    "invalid format",
			version: "not-a-version",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(context.Background(), tt.version, GranularityRelease, db)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}

			if err != nil && tt.wantSuggest {
				valErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("expected ValidationError, got %T", err)
					return
				}
				if len(valErr.Suggestions) == 0 {
					t.Errorf("expected suggestions, got none")
				}
			}
		})
	}
}

func TestValidateVersions_Order(t *testing.T) {
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid order 7.0.0 to 8.0.0",
			from:    "7.0.0",
			to:      "8.0.0",
			wantErr: false,
		},
		{
			name:    "invalid order 8.0.0 to 7.0.0",
			from:    "8.0.0",
			to:      "7.0.0",
			wantErr: true,
			errMsg:  "must be less than",
		},
		{
			name:    "same version",
			from:    "7.0.0",
			to:      "7.0.0",
			wantErr: true,
			errMsg:  "must be less than",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersions(context.Background(), tt.from, tt.to, GranularityRelease, db)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersions(%q, %q) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}

			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateVersion_Commit(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "valid short SHA",
			version: "abc1234",
			wantErr: false,
		},
		{
			name:    "valid full SHA",
			version: "abc1234def5678901234567890abcdef12345678",
			wantErr: false,
		},
		{
			name:    "too short SHA",
			version: "abc123",
			wantErr: true,
		},
		{
			name:    "version tag",
			version: "7.0.0",
			wantErr: false,
		},
		{
			name:    "invalid format",
			version: "xyz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(context.Background(), tt.version, GranularityCommit, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion(%q, Commit) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *ValidationError
		wantMsg string
	}{
		{
			name: "without suggestions",
			err: &ValidationError{
				Version: "8.12.0",
				Reason:  "version not found",
			},
			wantMsg: `invalid version "8.12.0": version not found`,
		},
		{
			name: "with suggestions",
			err: &ValidationError{
				Version:     "8.12.0",
				Reason:      "version not found",
				Suggestions: []string{"8.0.0", "8.1.0"},
			},
			wantMsg: "Did you mean one of these?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("Error() = %q, want to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestIsCommitSHA(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc1234", true},
		{"1234567", true},
		{"abc1234def5678901234567890abcdef12345678", true},
		{"abc123", false},  // too short
		{"ABC1234", false}, // uppercase
		{"7.0.0", false},   // version string
		{"abc123g", false}, // invalid hex char
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isCommitSHA(tt.input); got != tt.want {
				t.Errorf("isCommitSHA(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindSimilarVersions(t *testing.T) {
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	// Looking for 8.12.0 should suggest 8.x versions
	err = ValidateVersion(context.Background(), "8.12.0", GranularityRelease, db)
	if err == nil {
		t.Fatal("expected error for invalid version 8.12.0")
	}

	valErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	// Should have suggestions, preferably 8.x versions
	if len(valErr.Suggestions) == 0 {
		t.Fatal("expected suggestions")
	}

	// At least one suggestion should be an 8.x version
	has8x := false
	for _, s := range valErr.Suggestions {
		if strings.HasPrefix(s, "8.") {
			has8x = true
			break
		}
	}
	if !has8x {
		t.Errorf("expected at least one 8.x suggestion, got %v", valErr.Suggestions)
	}
}

func TestListAvailableVersions(t *testing.T) {
	db, err := flagdb.Load("../data/flags")
	if err != nil {
		t.Skipf("flag database not available: %v", err)
	}

	versions, err := ListAvailableVersions(context.Background(), GranularityRelease, db)
	if err != nil {
		t.Fatalf("ListAvailableVersions() error = %v", err)
	}

	if len(versions) == 0 {
		t.Error("expected versions, got none")
	}

	// Should contain known versions
	has7 := false
	has8 := false
	for _, v := range versions {
		if v == "7.0.0" {
			has7 = true
		}
		if v == "8.0.0" {
			has8 = true
		}
	}

	if !has7 {
		t.Error("expected 7.0.0 in versions")
	}
	if !has8 {
		t.Error("expected 8.0.0 in versions")
	}
}

func TestFormatAvailableVersions(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		wantMsg  string
	}{
		{
			name:     "empty",
			versions: []string{},
			wantMsg:  "No versions available",
		},
		{
			name:     "single version",
			versions: []string{"7.0.0"},
			wantMsg:  "7.x:",
		},
		{
			name:     "multiple majors",
			versions: []string{"7.0.0", "8.0.0"},
			wantMsg:  "Available versions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAvailableVersions(tt.versions)
			if !strings.Contains(got, tt.wantMsg) {
				t.Errorf("FormatAvailableVersions() = %q, want to contain %q", got, tt.wantMsg)
			}
		})
	}
}
