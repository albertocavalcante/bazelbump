//go:build nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"time"
)

// BatchedManager is a no-op wrapper for builds without state management.
type BatchedManager struct {
	Manager
}

// BatchedManagerOption configures a BatchedManager.
type BatchedManagerOption func(*BatchedManager)

// WithFlushThreshold is a no-op option.
func WithFlushThreshold(n int) BatchedManagerOption {
	return func(b *BatchedManager) {}
}

// WithFlushInterval is a no-op option.
func WithFlushInterval(d time.Duration) BatchedManagerOption {
	return func(b *BatchedManager) {}
}

// NewBatchedManager creates a no-op BatchedManager.
func NewBatchedManager(mgr Manager, opts ...BatchedManagerOption) *BatchedManager {
	return &BatchedManager{Manager: mgr}
}

// SetFileState delegates to the underlying manager (which is also no-op).
func (b *BatchedManager) SetFileState(runID, path string, state FileState) error {
	return b.Manager.SetFileState(runID, path, state)
}

// Flush is a no-op.
func (b *BatchedManager) Flush() error {
	return nil
}

// PendingCount always returns 0.
func (b *BatchedManager) PendingCount() int {
	return 0
}

// Close delegates to the underlying manager.
func (b *BatchedManager) Close() error {
	return b.Manager.Close()
}
