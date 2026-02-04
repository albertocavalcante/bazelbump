// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"testing"
)

func TestCapabilityConfig(t *testing.T) {
	t.Run("NewCapabilityConfig has fs enabled by default", func(t *testing.T) {
		config := NewCapabilityConfig()
		if !config.IsGranted(CapabilityFS) {
			t.Error("Expected fs capability to be granted by default")
		}
		if config.IsGranted(CapabilityExec) {
			t.Error("Expected exec capability to NOT be granted by default")
		}
		if config.IsGranted(CapabilityBCR) {
			t.Error("Expected bcr capability to NOT be granted by default")
		}
	})

	t.Run("Grant adds capabilities", func(t *testing.T) {
		config := NewCapabilityConfig()
		config.Grant(CapabilityExec, CapabilityBCR)

		if !config.IsGranted(CapabilityExec) {
			t.Error("Expected exec capability to be granted")
		}
		if !config.IsGranted(CapabilityBCR) {
			t.Error("Expected bcr capability to be granted")
		}
	})

	t.Run("Deny overrides Grant", func(t *testing.T) {
		config := NewCapabilityConfig()
		config.Grant(CapabilityExec)
		config.Deny(CapabilityExec)

		if config.IsGranted(CapabilityExec) {
			t.Error("Expected exec capability to be denied (deny should override grant)")
		}
	})

	t.Run("TrustAll grants everything", func(t *testing.T) {
		config := NewCapabilityConfig()
		config.TrustAll = true

		if !config.IsGranted(CapabilityExec) {
			t.Error("Expected exec capability to be granted with TrustAll")
		}
		if !config.IsGranted(CapabilityBCR) {
			t.Error("Expected bcr capability to be granted with TrustAll")
		}
	})

	t.Run("TrustAll is still overridden by Deny", func(t *testing.T) {
		config := NewCapabilityConfig()
		config.TrustAll = true
		config.Deny(CapabilityExec)

		if config.IsGranted(CapabilityExec) {
			t.Error("Expected exec capability to be denied even with TrustAll")
		}
	})

	t.Run("RequiredButNotGranted returns missing capabilities", func(t *testing.T) {
		config := NewCapabilityConfig()
		required := []Capability{CapabilityFS, CapabilityExec, CapabilityBCR}

		missing := config.RequiredButNotGranted(required)

		// fs is granted by default, exec and bcr are not
		if len(missing) != 2 {
			t.Errorf("Expected 2 missing capabilities, got %d", len(missing))
		}

		hasExec := false
		hasBCR := false
		for _, cap := range missing {
			if cap == CapabilityExec {
				hasExec = true
			}
			if cap == CapabilityBCR {
				hasBCR = true
			}
		}
		if !hasExec || !hasBCR {
			t.Errorf("Expected exec and bcr to be missing, got %v", missing)
		}
	})
}

func TestParseCapabilities(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		caps, err := ParseCapabilities("")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if caps != nil {
			t.Errorf("Expected nil, got %v", caps)
		}
	})

	t.Run("single capability", func(t *testing.T) {
		caps, err := ParseCapabilities("exec")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(caps) != 1 || caps[0] != CapabilityExec {
			t.Errorf("Expected [exec], got %v", caps)
		}
	})

	t.Run("multiple capabilities", func(t *testing.T) {
		caps, err := ParseCapabilities("exec,bcr")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(caps) != 2 {
			t.Errorf("Expected 2 capabilities, got %d", len(caps))
		}
	})

	t.Run("with spaces", func(t *testing.T) {
		caps, err := ParseCapabilities("exec, bcr, fs")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(caps) != 3 {
			t.Errorf("Expected 3 capabilities, got %d", len(caps))
		}
	})

	t.Run("unknown capability returns error", func(t *testing.T) {
		_, err := ParseCapabilities("unknown")
		if err == nil {
			t.Error("Expected error for unknown capability")
		}
	})
}

func TestIsValidCapability(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"fs", true},
		{"bcr", true},
		{"exec", true},
		{"unknown", false},
		{"", false},
		{"FS", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsValidCapability(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidCapability(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetCapabilityInfo(t *testing.T) {
	t.Run("known capabilities", func(t *testing.T) {
		for _, cap := range AllCapabilities() {
			info := GetCapabilityInfo(cap)
			if info == nil {
				t.Errorf("Expected info for capability %q", cap)
			}
			if info.Name != cap {
				t.Errorf("Info name mismatch: got %q, want %q", info.Name, cap)
			}
			if info.Description == "" {
				t.Errorf("Expected description for capability %q", cap)
			}
			if info.Risk == "" {
				t.Errorf("Expected risk level for capability %q", cap)
			}
		}
	})

	t.Run("unknown capability returns nil", func(t *testing.T) {
		info := GetCapabilityInfo(Capability("unknown"))
		if info != nil {
			t.Error("Expected nil for unknown capability")
		}
	})

	t.Run("fs is default enabled", func(t *testing.T) {
		info := GetCapabilityInfo(CapabilityFS)
		if !info.DefaultEnabled {
			t.Error("Expected fs to be default enabled")
		}
	})

	t.Run("exec is not default enabled", func(t *testing.T) {
		info := GetCapabilityInfo(CapabilityExec)
		if info.DefaultEnabled {
			t.Error("Expected exec to NOT be default enabled")
		}
	})
}

func TestFormatCapabilityList(t *testing.T) {
	tests := []struct {
		input    []Capability
		expected string
	}{
		{nil, "(none)"},
		{[]Capability{}, "(none)"},
		{[]Capability{CapabilityExec}, "exec"},
		{[]Capability{CapabilityExec, CapabilityBCR}, "bcr, exec"}, // sorted
		{[]Capability{CapabilityFS, CapabilityBCR, CapabilityExec}, "bcr, exec, fs"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatCapabilityList(tt.input)
			if result != tt.expected {
				t.Errorf("FormatCapabilityList(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
