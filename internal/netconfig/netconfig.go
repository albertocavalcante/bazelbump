// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package netconfig provides network configuration for bazelbump.
// It supports air-gap environments by defaulting to offline mode
// and requiring explicit opt-in for network access.
package netconfig

import (
	"os"
	"strings"
)

// Environment variable names.
const (
	// EnvOffline forces offline mode (no network access).
	// Takes precedence over EnvOnline.
	EnvOffline = "BAZELBUMP_OFFLINE"

	// EnvOnline enables network access for features like
	// GitHub API fallback for version validation.
	EnvOnline = "BAZELBUMP_ONLINE"

	// EnvGitHubToken is the GitHub token for API requests.
	// Also checks GITHUB_TOKEN and BAZELISK_GITHUB_TOKEN.
	EnvGitHubToken = "BAZELBUMP_GITHUB_TOKEN"
)

// Mode defines how bazelbump handles network access.
type Mode int

const (
	// ModeOffline is the default: no network access.
	// Uses only local/embedded data.
	ModeOffline Mode = iota

	// ModeOnline allows network access for fallback features
	// like GitHub API queries for version validation.
	ModeOnline
)

func (m Mode) String() string {
	switch m {
	case ModeOnline:
		return "online"
	default:
		return "offline"
	}
}

// Config holds network configuration.
type Config struct {
	Mode        Mode
	GitHubToken string
}

// Default returns the default configuration.
// Default is offline (air-gap safe).
func Default() *Config {
	cfg := &Config{
		Mode: ModeOffline,
	}
	cfg.applyEnvVars()
	return cfg
}

// WithOnline returns a copy with online mode enabled.
func (c *Config) WithOnline() *Config {
	copy := *c
	copy.Mode = ModeOnline
	return &copy
}

// WithOffline returns a copy with offline mode forced.
func (c *Config) WithOffline() *Config {
	copy := *c
	copy.Mode = ModeOffline
	return &copy
}

// IsOffline returns true if network access is disabled.
func (c *Config) IsOffline() bool {
	return c.Mode == ModeOffline
}

// IsOnline returns true if network access is allowed.
func (c *Config) IsOnline() bool {
	return c.Mode == ModeOnline
}

// applyEnvVars applies environment variable overrides.
// BAZELBUMP_OFFLINE takes precedence over BAZELBUMP_ONLINE.
func (c *Config) applyEnvVars() {
	// Offline takes precedence (security: admin can force offline)
	if isEnvTrue(EnvOffline) {
		c.Mode = ModeOffline
		return
	}

	// Online is opt-in
	if isEnvTrue(EnvOnline) {
		c.Mode = ModeOnline
	}

	// GitHub token (check multiple sources)
	c.GitHubToken = getGitHubToken()
}

// isEnvTrue returns true if the environment variable is set to a truthy value.
func isEnvTrue(name string) bool {
	val := strings.ToLower(os.Getenv(name))
	return val == "1" || val == "true" || val == "yes"
}

// getGitHubToken returns the GitHub token from environment variables.
// Checks BAZELBUMP_GITHUB_TOKEN, GITHUB_TOKEN, BAZELISK_GITHUB_TOKEN in order.
func getGitHubToken() string {
	if token := os.Getenv(EnvGitHubToken); token != "" {
		return token
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("BAZELISK_GITHUB_TOKEN"); token != "" {
		return token
	}
	return ""
}
