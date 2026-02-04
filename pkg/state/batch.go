//go:build !nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// BatchedManager wraps a Manager and batches file state writes for performance.
// Instead of writing each file state individually (which can cause thousands of
// writes for large projects), it buffers updates and flushes them periodically.
//
// Usage:
//
//	mgr, _ := state.New(dbPath)
//	batched := state.NewBatchedManager(mgr)
//	defer batched.Flush() // Ensure final flush
//
//	for _, file := range files {
//	    batched.SetFileState(runID, file, state)
//	}
type BatchedManager struct {
	Manager

	// db is the underlying database (extracted from sqliteManager)
	db *sql.DB

	// Pending file states to write
	pendingStates []pendingFileState

	// Configuration
	flushThreshold int           // Flush every N operations (default: 50)
	flushInterval  time.Duration // Flush every N seconds (default: 5s)
	lastFlush      time.Time

	mu sync.Mutex
}

// pendingFileState represents a file state waiting to be written.
type pendingFileState struct {
	RunID       string
	FilePath    string
	Status      FileStatus
	ContentHash string
	AfterHash   string
	Mtime       int64
	FileExists  bool
	StartedAt   *time.Time
	CompletedAt *time.Time
	Changed     bool
	LinesAdded  int
	LinesRemoved int
	ErrorMessage string
}

// BatchedManagerOption configures a BatchedManager.
type BatchedManagerOption func(*BatchedManager)

// WithFlushThreshold sets the number of operations before auto-flush.
func WithFlushThreshold(n int) BatchedManagerOption {
	return func(b *BatchedManager) {
		b.flushThreshold = n
	}
}

// WithFlushInterval sets the time interval between auto-flushes.
func WithFlushInterval(d time.Duration) BatchedManagerOption {
	return func(b *BatchedManager) {
		b.flushInterval = d
	}
}

// NewBatchedManager creates a new BatchedManager wrapping the given Manager.
// The Manager must be a *sqliteManager for batching to work; otherwise,
// operations fall back to the underlying Manager.
func NewBatchedManager(mgr Manager, opts ...BatchedManagerOption) *BatchedManager {
	b := &BatchedManager{
		Manager:        mgr,
		flushThreshold: 50,
		flushInterval:  5 * time.Second,
		lastFlush:      time.Now(),
	}

	// Try to extract the database connection from sqliteManager
	if sm, ok := mgr.(*sqliteManager); ok {
		b.db = sm.db
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// SetFileState buffers a file state update for batched writing.
// If the buffer reaches the threshold or enough time has passed, it flushes automatically.
func (b *BatchedManager) SetFileState(runID, path string, state FileState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If we don't have direct DB access, fall back to the underlying manager
	if b.db == nil {
		return b.Manager.SetFileState(runID, path, state)
	}

	// Add to pending states
	b.pendingStates = append(b.pendingStates, pendingFileState{
		RunID:        runID,
		FilePath:     path,
		Status:       state.Status,
		ContentHash:  state.ContentHash,
		AfterHash:    state.AfterHash,
		Mtime:        state.Mtime,
		FileExists:   state.FileExists,
		StartedAt:    state.StartedAt,
		CompletedAt:  state.CompletedAt,
		Changed:      state.Changed,
		LinesAdded:   state.LinesAdded,
		LinesRemoved: state.LinesRemoved,
		ErrorMessage: state.ErrorMessage,
	})

	// Check if we should flush
	if len(b.pendingStates) >= b.flushThreshold ||
		time.Since(b.lastFlush) >= b.flushInterval {
		return b.flushLocked()
	}

	return nil
}

// Flush writes all pending file states to the database.
// This should be called before closing the manager or when graceful shutdown is needed.
func (b *BatchedManager) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}

// flushLocked performs the actual flush. Must be called with b.mu held.
func (b *BatchedManager) flushLocked() error {
	if len(b.pendingStates) == 0 {
		return nil
	}

	// If we don't have direct DB access, flush one by one
	if b.db == nil {
		for _, ps := range b.pendingStates {
			err := b.Manager.SetFileState(ps.RunID, ps.FilePath, FileState{
				Status:       ps.Status,
				ContentHash:  ps.ContentHash,
				AfterHash:    ps.AfterHash,
				Mtime:        ps.Mtime,
				FileExists:   ps.FileExists,
				StartedAt:    ps.StartedAt,
				CompletedAt:  ps.CompletedAt,
				Changed:      ps.Changed,
				LinesAdded:   ps.LinesAdded,
				LinesRemoved: ps.LinesRemoved,
				ErrorMessage: ps.ErrorMessage,
			})
			if err != nil {
				return err
			}
		}
		b.pendingStates = b.pendingStates[:0]
		b.lastFlush = time.Now()
		return nil
	}

	// Batch all writes in a single transaction
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare the upsert statement
	stmt, err := tx.Prepare(`
		INSERT INTO file_states (
			run_id, file_path, status, content_hash, after_hash, mtime, file_exists,
			started_at, completed_at, changed, lines_added, lines_removed, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, file_path) DO UPDATE SET
			status = excluded.status,
			content_hash = COALESCE(excluded.content_hash, file_states.content_hash),
			after_hash = COALESCE(excluded.after_hash, file_states.after_hash),
			mtime = COALESCE(excluded.mtime, file_states.mtime),
			file_exists = excluded.file_exists,
			started_at = COALESCE(excluded.started_at, file_states.started_at),
			completed_at = excluded.completed_at,
			changed = excluded.changed,
			lines_added = excluded.lines_added,
			lines_removed = excluded.lines_removed,
			error_message = excluded.error_message
		WHERE file_states.status != 'completed' OR excluded.status = 'completed'
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute all pending states
	for _, ps := range b.pendingStates {
		// Default file_exists to true if not explicitly set
		fileExists := ps.FileExists
		if ps.Status != FileStatusFailed && ps.Status != FileStatusSkipped && !ps.FileExists {
			fileExists = true
		}

		_, err := stmt.Exec(
			ps.RunID, ps.FilePath, ps.Status, ps.ContentHash, ps.AfterHash, ps.Mtime, fileExists,
			ps.StartedAt, ps.CompletedAt, ps.Changed, ps.LinesAdded, ps.LinesRemoved, ps.ErrorMessage,
		)
		if err != nil {
			return fmt.Errorf("failed to insert file state for %s: %w", ps.FilePath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	b.pendingStates = b.pendingStates[:0]
	b.lastFlush = time.Now()

	return nil
}

// PendingCount returns the number of pending file states waiting to be flushed.
func (b *BatchedManager) PendingCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pendingStates)
}

// Close flushes pending states and closes the underlying manager.
func (b *BatchedManager) Close() error {
	if err := b.Flush(); err != nil {
		// Log but continue with close
		fmt.Printf("Warning: failed to flush pending states: %v\n", err)
	}
	return b.Manager.Close()
}
