//go:build nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

// ProjectLock is a no-op implementation for builds without state management.
type ProjectLock struct {
	path string
}

// NewProjectLock creates a no-op ProjectLock.
func NewProjectLock(projectPath string) (*ProjectLock, error) {
	return &ProjectLock{
		path: projectPath + "/.bazelbump/locks/state.lock",
	}, nil
}

// Acquire is a no-op that always succeeds.
func (l *ProjectLock) Acquire() error {
	return nil
}

// Release is a no-op.
func (l *ProjectLock) Release() error {
	return nil
}

// Path returns the path to the lock file.
func (l *ProjectLock) Path() string {
	return l.path
}

// Locked always returns false in nostate builds.
func (l *ProjectLock) Locked() bool {
	return false
}
