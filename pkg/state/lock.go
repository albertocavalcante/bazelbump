//go:build !nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ProjectLock provides cross-platform file locking to prevent concurrent
// bazelbump instances from running on the same project.
//
// It uses github.com/gofrs/flock which works on:
// - Linux (flock)
// - macOS (flock)
// - Windows (LockFileEx)
// - NFS (advisory locking)
type ProjectLock struct {
	flock *flock.Flock
	path  string
}

// NewProjectLock creates a new ProjectLock for the given project path.
// The lock file is created at .bazelbump/locks/state.lock within the project.
func NewProjectLock(projectPath string) (*ProjectLock, error) {
	lockDir := filepath.Join(projectPath, ".bazelbump", "locks")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	lockPath := filepath.Join(lockDir, "state.lock")

	return &ProjectLock{
		flock: flock.New(lockPath),
		path:  lockPath,
	}, nil
}

// Acquire attempts to acquire the lock.
// Returns ErrLockHeld if another instance already holds the lock.
func (l *ProjectLock) Acquire() error {
	locked, err := l.flock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return ErrLockHeld
	}
	return nil
}

// Release releases the lock.
func (l *ProjectLock) Release() error {
	return l.flock.Unlock()
}

// Path returns the path to the lock file.
func (l *ProjectLock) Path() string {
	return l.path
}

// Locked returns whether the lock is currently held by this instance.
func (l *ProjectLock) Locked() bool {
	return l.flock.Locked()
}
