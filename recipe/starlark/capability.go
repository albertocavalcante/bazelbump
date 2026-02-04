// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"
	"sort"
	"strings"
)

// Capability represents a capability that a recipe may require.
// Capabilities control access to sensitive operations like shell execution
// or network access.
type Capability string

const (
	// CapabilityFS provides read/write access within the project directory.
	// This is enabled by default as recipes need to modify files.
	CapabilityFS Capability = "fs"

	// CapabilityBCR provides access to query the Bazel Central Registry.
	// This requires network access but is read-only.
	CapabilityBCR Capability = "bcr"

	// CapabilityExec provides the ability to execute shell commands.
	// This is a high-risk capability and requires explicit approval.
	CapabilityExec Capability = "exec"
)

// AllCapabilities returns all known capabilities in display order.
func AllCapabilities() []Capability {
	return []Capability{CapabilityFS, CapabilityBCR, CapabilityExec}
}

// CapabilityInfo provides metadata about a capability.
type CapabilityInfo struct {
	// Name is the capability identifier (e.g., "exec").
	Name Capability

	// Description is a human-readable description of what the capability provides.
	Description string

	// Risk indicates the risk level: "low", "medium", or "high".
	Risk string

	// DefaultEnabled indicates whether this capability is enabled by default.
	DefaultEnabled bool
}

// GetCapabilityInfo returns information about a capability.
func GetCapabilityInfo(cap Capability) *CapabilityInfo {
	switch cap {
	case CapabilityFS:
		return &CapabilityInfo{
			Name:           CapabilityFS,
			Description:    "Read/write files in project directory",
			Risk:           "low",
			DefaultEnabled: true,
		}
	case CapabilityBCR:
		return &CapabilityInfo{
			Name:           CapabilityBCR,
			Description:    "Query Bazel Central Registry (network access)",
			Risk:           "low",
			DefaultEnabled: false,
		}
	case CapabilityExec:
		return &CapabilityInfo{
			Name:           CapabilityExec,
			Description:    "Execute shell commands",
			Risk:           "high",
			DefaultEnabled: false,
		}
	default:
		return nil
	}
}

// IsValidCapability checks if a string is a valid capability name.
func IsValidCapability(s string) bool {
	switch Capability(s) {
	case CapabilityFS, CapabilityBCR, CapabilityExec:
		return true
	default:
		return false
	}
}

// ParseCapabilities parses a comma-separated list of capability names.
// Returns an error if any capability is unknown.
func ParseCapabilities(s string) ([]Capability, error) {
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	result := make([]Capability, 0, len(parts))

	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !IsValidCapability(name) {
			return nil, fmt.Errorf("unknown capability: %q (valid: fs, bcr, exec)", name)
		}
		result = append(result, Capability(name))
	}

	return result, nil
}

// CapabilityConfig holds the capability configuration for a recipe execution.
type CapabilityConfig struct {
	// Granted contains the capabilities that have been granted.
	// If a capability is in this map with value true, it is granted.
	Granted map[Capability]bool

	// Denied contains capabilities that have been explicitly denied.
	// Denied takes precedence over Granted.
	Denied map[Capability]bool

	// TrustAll, if true, grants all capabilities without prompting.
	// Used for CI/automation scenarios.
	TrustAll bool
}

// NewCapabilityConfig creates a new capability configuration with defaults.
// By default, only CapabilityFS is enabled.
func NewCapabilityConfig() *CapabilityConfig {
	return &CapabilityConfig{
		Granted: map[Capability]bool{
			CapabilityFS: true, // fs is enabled by default
		},
		Denied: make(map[Capability]bool),
	}
}

// IsGranted returns true if the capability is granted.
// A capability is granted if:
// 1. TrustAll is true, OR
// 2. It is in Granted with value true AND NOT in Denied
func (c *CapabilityConfig) IsGranted(cap Capability) bool {
	if c == nil {
		// If no config, only default capabilities are granted
		info := GetCapabilityInfo(cap)
		return info != nil && info.DefaultEnabled
	}

	// Denied always takes precedence
	if c.Denied[cap] {
		return false
	}

	// Trust all grants everything
	if c.TrustAll {
		return true
	}

	// Check explicit grant
	return c.Granted[cap]
}

// Grant adds a capability to the granted set.
func (c *CapabilityConfig) Grant(caps ...Capability) {
	if c.Granted == nil {
		c.Granted = make(map[Capability]bool)
	}
	for _, cap := range caps {
		c.Granted[cap] = true
	}
}

// Deny adds a capability to the denied set.
// Denied capabilities take precedence over granted capabilities.
func (c *CapabilityConfig) Deny(caps ...Capability) {
	if c.Denied == nil {
		c.Denied = make(map[Capability]bool)
	}
	for _, cap := range caps {
		c.Denied[cap] = true
	}
}

// RequiredButNotGranted returns the capabilities that are required but not granted.
func (c *CapabilityConfig) RequiredButNotGranted(required []Capability) []Capability {
	var missing []Capability
	for _, cap := range required {
		if !c.IsGranted(cap) {
			missing = append(missing, cap)
		}
	}
	return missing
}

// FormatCapabilityList formats a list of capabilities for display.
func FormatCapabilityList(caps []Capability) string {
	if len(caps) == 0 {
		return "(none)"
	}

	// Sort for consistent display
	sorted := make([]string, len(caps))
	for i, cap := range caps {
		sorted[i] = string(cap)
	}
	sort.Strings(sorted)

	return strings.Join(sorted, ", ")
}

// CapabilityPromptInfo provides information for prompting the user about capabilities.
type CapabilityPromptInfo struct {
	// Capability is the capability being requested.
	Capability Capability

	// Info contains metadata about the capability.
	Info *CapabilityInfo

	// UsedBy describes what feature or module uses this capability.
	UsedBy string
}

// GetCapabilityPromptInfos returns prompt information for the given capabilities.
func GetCapabilityPromptInfos(caps []Capability) []CapabilityPromptInfo {
	result := make([]CapabilityPromptInfo, 0, len(caps))
	for _, cap := range caps {
		info := GetCapabilityInfo(cap)
		if info == nil {
			continue
		}

		usedBy := ""
		switch cap {
		case CapabilityExec:
			usedBy = "exec module (exec.run, exec.which)"
		case CapabilityBCR:
			usedBy = "bcr module (bcr.latest, bcr.metadata, etc.)"
		case CapabilityFS:
			usedBy = "file system operations"
		}

		result = append(result, CapabilityPromptInfo{
			Capability: cap,
			Info:       info,
			UsedBy:     usedBy,
		})
	}
	return result
}
