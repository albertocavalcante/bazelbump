// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package github

import (
	"context"
	"testing"

	"github.com/albertocavalcante/bazelbump/internal/netconfig"
)

func TestClient_VersionExists_OfflineMode(t *testing.T) {
	cfg := netconfig.Default() // Default is offline
	client := NewClient(cfg)

	_, err := client.VersionExists(context.Background(), "7.0.0")
	if err == nil {
		t.Error("expected error in offline mode")
	}
	if err.Error() != "network access disabled (offline mode)" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_GetReleases_OfflineMode(t *testing.T) {
	cfg := netconfig.Default() // Default is offline
	client := NewClient(cfg)

	_, err := client.GetReleases(context.Background())
	if err == nil {
		t.Error("expected error in offline mode")
	}
	if err.Error() != "network access disabled (offline mode)" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient(t *testing.T) {
	cfg := netconfig.Default()
	client := NewClient(cfg)

	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.config != cfg {
		t.Error("config not set correctly")
	}
	if client.client == nil {
		t.Error("http client not initialized")
	}
	if client.cacheTTL == 0 {
		t.Error("cacheTTL not set")
	}
}
