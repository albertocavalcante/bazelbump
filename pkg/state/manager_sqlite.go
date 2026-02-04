//go:build !nostate

// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// Constants for checkpoint management.
const (
	// MaxCheckpointSize is the warning threshold for checkpoint state size (1MB).
	MaxCheckpointSize = 1 * 1024 * 1024

	// MaxCheckpointsPerRun is the default number of checkpoints to keep per run.
	MaxCheckpointsPerRun = 10
)

// sqliteManager implements Manager using SQLite for persistence.
type sqliteManager struct {
	db   *sql.DB
	mu   sync.RWMutex
	path string
}

// New creates a new state Manager backed by SQLite.
// The database file is created if it doesn't exist.
// Uses modernc.org/sqlite - a pure Go implementation (no CGO required).
func New(dbPath string) (Manager, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." && dir != ":memory:" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create state directory: %w", err)
		}
	}

	// Open database with WAL mode for better concurrency
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // Single writer for SQLite
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Configure SQLite for durability
	if err := configureSQLite(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to configure SQLite: %w", err)
	}

	// Initialize schema
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	mgr := &sqliteManager{
		db:   db,
		path: dbPath,
	}

	// Validate on startup - mark orphaned runs as interrupted
	if err := mgr.ValidateOnStartup(); err != nil {
		// Log warning but don't fail
		log.Printf("Warning: failed to validate state on startup: %v", err)
	}

	return mgr, nil
}

// configureSQLite sets up SQLite for durability and proper behavior.
// CRITICAL: Default SQLite settings can lose data on power failure.
func configureSQLite(db *sql.DB) error {
	pragmas := []string{
		// CRITICAL: Full sync for durability (default NORMAL can lose data)
		"PRAGMA synchronous=FULL",

		// Limit WAL file size to prevent unbounded growth
		"PRAGMA wal_autocheckpoint=100",       // Checkpoint every 100 pages
		"PRAGMA journal_size_limit=5242880",   // Limit WAL to 5MB

		// Foreign key enforcement
		"PRAGMA foreign_keys=ON",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to set %s: %w", pragma, err)
		}
	}

	return nil
}

// Available returns true because this build includes SQLite support.
func Available() bool { return true }

// initSchema creates the database tables if they don't exist.
func initSchema(db *sql.DB) error {
	// Execute schema creation
	if _, err := db.Exec(Schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Initialize schema version
	if _, err := db.Exec(SchemaInitVersion); err != nil {
		return fmt.Errorf("failed to initialize schema version: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (m *sqliteManager) Close() error {
	return m.db.Close()
}

// =============================================================================
// Maintenance
// =============================================================================

// ValidateOnStartup performs startup validation and cleanup.
// This includes marking orphaned runs (stuck in "running" status) as interrupted.
func (m *sqliteManager) ValidateOnStartup() error {
	affected, err := m.MarkOrphanedRunsInterrupted(time.Hour)
	if err != nil {
		return err
	}

	if affected > 0 {
		log.Printf("Marked %d orphaned runs as interrupted", affected)
	}

	return nil
}

// MarkOrphanedRunsInterrupted marks runs stuck in "running" status as "interrupted".
// This handles cases where the process crashed without graceful shutdown.
func (m *sqliteManager) MarkOrphanedRunsInterrupted(timeout time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.db.Exec(`
		UPDATE runs
		SET status = ?,
			error_message = 'Process terminated unexpectedly',
			completed_at = CURRENT_TIMESTAMP
		WHERE status = ?
		AND started_at < datetime('now', ?)
	`, RunStatusInterrupted, RunStatusRunning, fmt.Sprintf("-%d seconds", int(timeout.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("failed to mark orphaned runs: %w", err)
	}

	affected, _ := result.RowsAffected()
	return affected, nil
}

// =============================================================================
// Run Management
// =============================================================================

// CreateRun creates a new recipe run.
func (m *sqliteManager) CreateRun(recipe string, opts RunOptions) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	now := time.Now()

	var configJSON []byte
	if opts.Config != nil {
		var err error
		configJSON, err = json.Marshal(opts.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
	}

	_, err := m.db.Exec(`
		INSERT INTO runs (
			id, recipe_name, recipe_version, recipe_hash, project_path,
			started_at, status,
			git_commit, git_branch, bazel_version, dry_run,
			config_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, recipe, opts.RecipeVersion, opts.RecipeHash, opts.ProjectPath,
		now, RunStatusPending,
		opts.GitCommit, opts.GitBranch, opts.BazelVersion, opts.DryRun,
		string(configJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	return &Run{
		ID:            id,
		RecipeName:    recipe,
		RecipeVersion: opts.RecipeVersion,
		RecipeHash:    opts.RecipeHash,
		ProjectPath:   opts.ProjectPath,
		StartedAt:     now,
		Status:        RunStatusPending,
		GitCommit:     opts.GitCommit,
		GitBranch:     opts.GitBranch,
		BazelVersion:  opts.BazelVersion,
		DryRun:        opts.DryRun,
		Config:        configJSON,
	}, nil
}

// GetRun retrieves a run by ID.
// Returns ErrRunNotFound if the run does not exist.
func (m *sqliteManager) GetRun(id string) (*Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	run, err := m.scanRun(m.db.QueryRow(`
		SELECT id, recipe_name, recipe_version, recipe_hash, project_path,
			started_at, completed_at, status,
			total_files, processed_files, current_step, current_file,
			retry_count,
			git_commit, git_branch, bazel_version, dry_run,
			config_json, context_outputs, error_message, changes_count
		FROM runs WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, ErrRunNotFound
	}
	return run, nil
}

// GetLatestRun retrieves the most recent run for a recipe.
// Returns ErrRunNotFound if no run exists for the recipe.
func (m *sqliteManager) GetLatestRun(recipe string) (*Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	run, err := m.scanRun(m.db.QueryRow(`
		SELECT id, recipe_name, recipe_version, recipe_hash, project_path,
			started_at, completed_at, status,
			total_files, processed_files, current_step, current_file,
			retry_count,
			git_commit, git_branch, bazel_version, dry_run,
			config_json, context_outputs, error_message, changes_count
		FROM runs WHERE recipe_name = ?
		ORDER BY started_at DESC LIMIT 1`, recipe))
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, ErrRunNotFound
	}
	return run, nil
}

// scanRun scans a row into a Run struct.
func (m *sqliteManager) scanRun(row *sql.Row) (*Run, error) {
	var run Run
	var completedAt sql.NullTime
	var currentStep, currentFile, gitCommit, gitBranch, bazelVersion sql.NullString
	var configJSON, contextOutputs, errorMessage sql.NullString
	var recipeVersion, recipeHash sql.NullString

	err := row.Scan(
		&run.ID, &run.RecipeName, &recipeVersion, &recipeHash, &run.ProjectPath,
		&run.StartedAt, &completedAt, &run.Status,
		&run.TotalFiles, &run.ProcessedFiles, &currentStep, &currentFile,
		&run.RetryCount,
		&gitCommit, &gitBranch, &bazelVersion, &run.DryRun,
		&configJSON, &contextOutputs, &errorMessage, &run.ChangesCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan run: %w", err)
	}

	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	if recipeVersion.Valid {
		run.RecipeVersion = recipeVersion.String
	}
	if recipeHash.Valid {
		run.RecipeHash = recipeHash.String
	}
	if currentStep.Valid {
		run.CurrentStep = currentStep.String
	}
	if currentFile.Valid {
		run.CurrentFile = currentFile.String
	}
	if gitCommit.Valid {
		run.GitCommit = gitCommit.String
	}
	if gitBranch.Valid {
		run.GitBranch = gitBranch.String
	}
	if bazelVersion.Valid {
		run.BazelVersion = bazelVersion.String
	}
	if configJSON.Valid {
		run.Config = json.RawMessage(configJSON.String)
	}
	if contextOutputs.Valid {
		run.ContextOutputs = json.RawMessage(contextOutputs.String)
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}

	return &run, nil
}

// UpdateRunStatus updates the status of a run.
func (m *sqliteManager) UpdateRunStatus(id string, status RunStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var completedAt interface{}
	if status == RunStatusCompleted || status == RunStatusFailed ||
		status == RunStatusInterrupted || status == RunStatusCancelled {
		completedAt = time.Now()
	}

	result, err := m.db.Exec(`
		UPDATE runs SET status = ?, completed_at = ?
		WHERE id = ?`, status, completedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrRunNotFound
	}

	return nil
}

// UpdateRunProgress updates the progress of a run.
func (m *sqliteManager) UpdateRunProgress(id string, processed, total int, currentFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.db.Exec(`
		UPDATE runs SET processed_files = ?, total_files = ?, current_file = ?
		WHERE id = ?`, processed, total, currentFile, id)
	if err != nil {
		return fmt.Errorf("failed to update run progress: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrRunNotFound
	}

	return nil
}

// ListRuns lists runs matching the given criteria.
func (m *sqliteManager) ListRuns(opts ListRunsOptions) ([]*Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT id, recipe_name, recipe_version, recipe_hash, project_path,
			started_at, completed_at, status,
			total_files, processed_files, current_step, current_file,
			retry_count,
			git_commit, git_branch, bazel_version, dry_run,
			config_json, context_outputs, error_message, changes_count
		FROM runs WHERE 1=1`
	args := []interface{}{}

	if opts.Recipe != "" {
		query += " AND recipe_name = ?"
		args = append(args, opts.Recipe)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	if opts.Since != nil {
		query += " AND started_at >= ?"
		args = append(args, *opts.Since)
	}

	query += " ORDER BY started_at DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		var run Run
		var completedAt sql.NullTime
		var currentStep, currentFile, gitCommit, gitBranch, bazelVersion sql.NullString
		var configJSON, contextOutputs, errorMessage sql.NullString
		var recipeVersion, recipeHash sql.NullString

		err := rows.Scan(
			&run.ID, &run.RecipeName, &recipeVersion, &recipeHash, &run.ProjectPath,
			&run.StartedAt, &completedAt, &run.Status,
			&run.TotalFiles, &run.ProcessedFiles, &currentStep, &currentFile,
			&run.RetryCount,
			&gitCommit, &gitBranch, &bazelVersion, &run.DryRun,
			&configJSON, &contextOutputs, &errorMessage, &run.ChangesCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}

		if completedAt.Valid {
			run.CompletedAt = &completedAt.Time
		}
		if recipeVersion.Valid {
			run.RecipeVersion = recipeVersion.String
		}
		if recipeHash.Valid {
			run.RecipeHash = recipeHash.String
		}
		if currentStep.Valid {
			run.CurrentStep = currentStep.String
		}
		if currentFile.Valid {
			run.CurrentFile = currentFile.String
		}
		if gitCommit.Valid {
			run.GitCommit = gitCommit.String
		}
		if gitBranch.Valid {
			run.GitBranch = gitBranch.String
		}
		if bazelVersion.Valid {
			run.BazelVersion = bazelVersion.String
		}
		if configJSON.Valid {
			run.Config = json.RawMessage(configJSON.String)
		}
		if contextOutputs.Valid {
			run.ContextOutputs = json.RawMessage(contextOutputs.String)
		}
		if errorMessage.Valid {
			run.ErrorMessage = errorMessage.String
		}

		runs = append(runs, &run)
	}

	return runs, rows.Err()
}

// =============================================================================
// Checkpoint Management
// =============================================================================

// CreateCheckpoint creates a new checkpoint for a run.
func (m *sqliteManager) CreateCheckpoint(runID string, opts CheckpointOptions) (*Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	now := time.Now()

	var stateJSON []byte
	var stateSize int
	if opts.State != nil {
		var err error
		stateJSON, err = json.Marshal(opts.State)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal state: %w", err)
		}
		stateSize = len(stateJSON)

		// Warn if state is too large
		if stateSize > MaxCheckpointSize {
			log.Printf("Warning: Checkpoint state is large (%d bytes), consider reducing scan scope", stateSize)
		}
	}

	cpType := opts.Type
	if cpType == "" {
		cpType = CheckpointAuto
	}

	_, err := m.db.Exec(`
		INSERT INTO checkpoints (
			id, run_id, step_id, step_index, file_index,
			created_at, checkpoint_type, state_json, state_size, description
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, runID, opts.StepID, opts.StepIndex, opts.FileIndex,
		now, cpType, string(stateJSON), stateSize, opts.Description,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint: %w", err)
	}

	// Cleanup old checkpoints (keep last N)
	if err := m.cleanupOldCheckpointsLocked(runID, MaxCheckpointsPerRun); err != nil {
		log.Printf("Warning: Failed to cleanup old checkpoints: %v", err)
	}

	return &Checkpoint{
		ID:          id,
		RunID:       runID,
		StepID:      opts.StepID,
		StepIndex:   opts.StepIndex,
		FileIndex:   opts.FileIndex,
		CreatedAt:   now,
		Type:        cpType,
		State:       stateJSON,
		StateSize:   stateSize,
		Description: opts.Description,
	}, nil
}

// cleanupOldCheckpointsLocked removes old checkpoints, keeping the last N.
// Must be called with m.mu held.
func (m *sqliteManager) cleanupOldCheckpointsLocked(runID string, keepLast int) error {
	_, err := m.db.Exec(`
		DELETE FROM checkpoints
		WHERE run_id = ?
		AND id NOT IN (
			SELECT id FROM checkpoints
			WHERE run_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		)
	`, runID, runID, keepLast)
	return err
}

// GetCheckpoint retrieves a checkpoint by ID.
// Returns ErrCheckpointNotFound if the checkpoint does not exist.
func (m *sqliteManager) GetCheckpoint(id string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cp, err := m.scanCheckpoint(m.db.QueryRow(`
		SELECT id, run_id, step_id, step_index, file_index,
			created_at, checkpoint_type, state_json, state_size, description
		FROM checkpoints WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if cp == nil {
		return nil, ErrCheckpointNotFound
	}
	return cp, nil
}

// GetLatestCheckpoint retrieves the most recent checkpoint for a run.
// Returns ErrCheckpointNotFound if no checkpoint exists for the run.
func (m *sqliteManager) GetLatestCheckpoint(runID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cp, err := m.scanCheckpoint(m.db.QueryRow(`
		SELECT id, run_id, step_id, step_index, file_index,
			created_at, checkpoint_type, state_json, state_size, description
		FROM checkpoints WHERE run_id = ?
		ORDER BY created_at DESC LIMIT 1`, runID))
	if err != nil {
		return nil, err
	}
	if cp == nil {
		return nil, ErrCheckpointNotFound
	}
	return cp, nil
}

// scanCheckpoint scans a row into a Checkpoint struct.
func (m *sqliteManager) scanCheckpoint(row *sql.Row) (*Checkpoint, error) {
	var cp Checkpoint
	var stateJSON, description sql.NullString
	var stateSize sql.NullInt64

	err := row.Scan(
		&cp.ID, &cp.RunID, &cp.StepID, &cp.StepIndex, &cp.FileIndex,
		&cp.CreatedAt, &cp.Type, &stateJSON, &stateSize, &description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
	}

	if stateJSON.Valid {
		cp.State = json.RawMessage(stateJSON.String)
	}
	if stateSize.Valid {
		cp.StateSize = int(stateSize.Int64)
	}
	if description.Valid {
		cp.Description = description.String
	}

	return &cp, nil
}

// ListCheckpoints lists all checkpoints for a run.
func (m *sqliteManager) ListCheckpoints(runID string) ([]*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT id, run_id, step_id, step_index, file_index,
			created_at, checkpoint_type, state_json, state_size, description
		FROM checkpoints WHERE run_id = ?
		ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []*Checkpoint
	for rows.Next() {
		var cp Checkpoint
		var stateJSON, description sql.NullString
		var stateSize sql.NullInt64

		err := rows.Scan(
			&cp.ID, &cp.RunID, &cp.StepID, &cp.StepIndex, &cp.FileIndex,
			&cp.CreatedAt, &cp.Type, &stateJSON, &stateSize, &description,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
		}

		if stateJSON.Valid {
			cp.State = json.RawMessage(stateJSON.String)
		}
		if stateSize.Valid {
			cp.StateSize = int(stateSize.Int64)
		}
		if description.Valid {
			cp.Description = description.String
		}

		checkpoints = append(checkpoints, &cp)
	}

	return checkpoints, rows.Err()
}

// =============================================================================
// File State Management
// =============================================================================

// SetFileState creates or updates the state of a file.
func (m *sqliteManager) SetFileState(runID, path string, state FileState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Default file_exists to true if not explicitly set
	fileExists := state.FileExists
	if state.Status != FileStatusFailed && state.Status != FileStatusSkipped {
		fileExists = true
	}

	_, err := m.db.Exec(`
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
			error_message = excluded.error_message`,
		runID, path, state.Status, state.ContentHash, state.AfterHash, state.Mtime, fileExists,
		state.StartedAt, state.CompletedAt, state.Changed,
		state.LinesAdded, state.LinesRemoved, state.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to set file state: %w", err)
	}

	return nil
}

// GetFileState retrieves the state of a file.
// Returns nil, nil if the file state does not exist.
func (m *sqliteManager) GetFileState(runID, path string) (*FileState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var fs FileState
	var contentHash, afterHash, errorMessage sql.NullString
	var mtime sql.NullInt64
	var startedAt, completedAt sql.NullTime
	var fileExists sql.NullBool

	err := m.db.QueryRow(`
		SELECT run_id, file_path, status, content_hash, after_hash, mtime, file_exists,
			started_at, completed_at, changed, lines_added, lines_removed, error_message
		FROM file_states WHERE run_id = ? AND file_path = ?`, runID, path).Scan(
		&fs.RunID, &fs.Path, &fs.Status, &contentHash, &afterHash, &mtime, &fileExists,
		&startedAt, &completedAt, &fs.Changed, &fs.LinesAdded, &fs.LinesRemoved, &errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file state: %w", err)
	}

	if contentHash.Valid {
		fs.ContentHash = contentHash.String
	}
	if afterHash.Valid {
		fs.AfterHash = afterHash.String
	}
	if mtime.Valid {
		fs.Mtime = mtime.Int64
	}
	if fileExists.Valid {
		fs.FileExists = fileExists.Bool
	} else {
		fs.FileExists = true // Default to true for backwards compatibility
	}
	if startedAt.Valid {
		fs.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		fs.CompletedAt = &completedAt.Time
	}
	if errorMessage.Valid {
		fs.ErrorMessage = errorMessage.String
	}

	return &fs, nil
}

// GetPendingFiles returns the paths of files that haven't been processed yet.
func (m *sqliteManager) GetPendingFiles(runID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT file_path FROM file_states
		WHERE run_id = ? AND status = ?
		ORDER BY file_path`, runID, FileStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending files: %w", err)
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("failed to scan file path: %w", err)
		}
		files = append(files, path)
	}

	return files, rows.Err()
}

// GetFilesByStatus returns file paths with the specified status.
// This is useful for finding staged files that need recovery.
func (m *sqliteManager) GetFilesByStatus(runID string, status FileStatus) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT file_path FROM file_states
		WHERE run_id = ? AND status = ?
		ORDER BY file_path`, runID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get files by status: %w", err)
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("failed to scan file path: %w", err)
		}
		files = append(files, path)
	}

	return files, rows.Err()
}

// =============================================================================
// Audit Log
// =============================================================================

// LogAction records an action in the audit log.
func (m *sqliteManager) LogAction(runID string, action AuditAction, opts AuditLogOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO audit_log (
			run_id, action, file_path, step_id, description,
			diff_summary, old_value, new_value
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, action, opts.FilePath, opts.StepID, opts.Description,
		opts.DiffSummary, opts.OldValue, opts.NewValue,
	)
	if err != nil {
		return fmt.Errorf("failed to log action: %w", err)
	}

	return nil
}
