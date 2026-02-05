// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package netconfig

import (
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	// Clear env vars for clean test
	os.Unsetenv(EnvOffline)
	os.Unsetenv(EnvOnline)

	cfg := Default()

	// Default should be offline
	if !cfg.IsOffline() {
		t.Error("Default config should be offline")
	}
	if cfg.IsOnline() {
		t.Error("Default config should not be online")
	}
}

func TestWithOnline(t *testing.T) {
	cfg := Default().WithOnline()

	if cfg.IsOffline() {
		t.Error("WithOnline config should not be offline")
	}
	if !cfg.IsOnline() {
		t.Error("WithOnline config should be online")
	}
}

func TestWithOffline(t *testing.T) {
	cfg := Default().WithOnline().WithOffline()

	if !cfg.IsOffline() {
		t.Error("WithOffline config should be offline")
	}
	if cfg.IsOnline() {
		t.Error("WithOffline config should not be online")
	}
}

func TestEnvOfflineTakesPrecedence(t *testing.T) {
	// Set both env vars - offline should take precedence
	os.Setenv(EnvOnline, "1")
	os.Setenv(EnvOffline, "1")
	defer os.Unsetenv(EnvOnline)
	defer os.Unsetenv(EnvOffline)

	cfg := Default()

	if !cfg.IsOffline() {
		t.Error("BAZELBUMP_OFFLINE=1 should force offline mode")
	}
}

func TestEnvOnline(t *testing.T) {
	os.Setenv(EnvOnline, "1")
	os.Unsetenv(EnvOffline)
	defer os.Unsetenv(EnvOnline)

	cfg := Default()

	if !cfg.IsOnline() {
		t.Error("BAZELBUMP_ONLINE=1 should enable online mode")
	}
}

func TestEnvTruthyValues(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"yes", true},
		{"YES", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv(EnvOnline, tt.value)
			os.Unsetenv(EnvOffline)
			defer os.Unsetenv(EnvOnline)

			cfg := Default()
			got := cfg.IsOnline()
			if got != tt.want {
				t.Errorf("EnvOnline=%q: IsOnline() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeOffline, "offline"},
		{ModeOnline, "online"},
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestGitHubTokenSources(t *testing.T) {
	// Clear all token env vars
	os.Unsetenv(EnvGitHubToken)
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("BAZELISK_GITHUB_TOKEN")

	// Test priority: BAZELBUMP_GITHUB_TOKEN > GITHUB_TOKEN > BAZELISK_GITHUB_TOKEN
	os.Setenv("BAZELISK_GITHUB_TOKEN", "bazelisk-token")
	cfg := Default()
	if cfg.GitHubToken != "bazelisk-token" {
		t.Errorf("expected bazelisk-token, got %q", cfg.GitHubToken)
	}

	os.Setenv("GITHUB_TOKEN", "github-token")
	cfg = Default()
	if cfg.GitHubToken != "github-token" {
		t.Errorf("expected github-token, got %q", cfg.GitHubToken)
	}

	os.Setenv(EnvGitHubToken, "bazelbump-token")
	cfg = Default()
	if cfg.GitHubToken != "bazelbump-token" {
		t.Errorf("expected bazelbump-token, got %q", cfg.GitHubToken)
	}

	// Cleanup
	os.Unsetenv(EnvGitHubToken)
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("BAZELISK_GITHUB_TOKEN")
}
