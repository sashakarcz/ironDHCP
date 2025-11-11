package storage

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreateGitSyncLog creates a new git sync log entry
func (s *Store) CreateGitSyncLog(ctx context.Context, log *GitSyncLog) error {
	query := `
		INSERT INTO git_sync_log (
			sync_started_at, status, commit_hash, commit_message, commit_author,
			commit_timestamp, triggered_by, triggered_by_user
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`

	err := s.pool.QueryRow(ctx, query,
		log.SyncStartedAt,
		log.Status,
		log.CommitHash,
		log.CommitMessage,
		log.CommitAuthor,
		log.CommitTimestamp,
		log.TriggeredBy,
		log.TriggeredByUser,
	).Scan(&log.ID, &log.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create git sync log: %w", err)
	}

	return nil
}

// UpdateGitSyncLog updates an existing git sync log entry
func (s *Store) UpdateGitSyncLog(ctx context.Context, log *GitSyncLog) error {
	query := `
		UPDATE git_sync_log
		SET sync_completed_at = $1,
		    status = $2,
		    error_message = $3,
		    changes_applied = $4
		WHERE id = $5
	`

	// Marshal changes to JSON
	var changesJSON []byte
	var err error
	if log.ChangesApplied != nil {
		changesJSON, err = json.Marshal(log.ChangesApplied)
		if err != nil {
			return fmt.Errorf("failed to marshal changes: %w", err)
		}
	}

	_, err = s.pool.Exec(ctx, query,
		log.SyncCompletedAt,
		log.Status,
		log.ErrorMessage,
		changesJSON,
		log.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update git sync log: %w", err)
	}

	return nil
}

// GetGitSyncLog retrieves a git sync log by ID
func (s *Store) GetGitSyncLog(ctx context.Context, id int64) (*GitSyncLog, error) {
	query := `
		SELECT id, sync_started_at, sync_completed_at, status, commit_hash,
		       commit_message, commit_author, commit_timestamp, error_message,
		       changes_applied, triggered_by, triggered_by_user, created_at
		FROM git_sync_log
		WHERE id = $1
	`

	var log GitSyncLog
	var changesJSON []byte

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&log.ID,
		&log.SyncStartedAt,
		&log.SyncCompletedAt,
		&log.Status,
		&log.CommitHash,
		&log.CommitMessage,
		&log.CommitAuthor,
		&log.CommitTimestamp,
		&log.ErrorMessage,
		&changesJSON,
		&log.TriggeredBy,
		&log.TriggeredByUser,
		&log.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get git sync log: %w", err)
	}

	// Unmarshal changes from JSON
	if len(changesJSON) > 0 {
		if err := json.Unmarshal(changesJSON, &log.ChangesApplied); err != nil {
			return nil, fmt.Errorf("failed to unmarshal changes: %w", err)
		}
	}

	return &log, nil
}

// GetRecentGitSyncLogs retrieves the most recent git sync logs
func (s *Store) GetRecentGitSyncLogs(ctx context.Context, limit int) ([]*GitSyncLog, error) {
	query := `
		SELECT id, sync_started_at, sync_completed_at, status, commit_hash,
		       commit_message, commit_author, commit_timestamp, error_message,
		       changes_applied, triggered_by, triggered_by_user, created_at
		FROM git_sync_log
		ORDER BY sync_started_at DESC
		LIMIT $1
	`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent git sync logs: %w", err)
	}
	defer rows.Close()

	var logs []*GitSyncLog
	for rows.Next() {
		var log GitSyncLog
		var changesJSON []byte

		err := rows.Scan(
			&log.ID,
			&log.SyncStartedAt,
			&log.SyncCompletedAt,
			&log.Status,
			&log.CommitHash,
			&log.CommitMessage,
			&log.CommitAuthor,
			&log.CommitTimestamp,
			&log.ErrorMessage,
			&changesJSON,
			&log.TriggeredBy,
			&log.TriggeredByUser,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan git sync log: %w", err)
		}

		// Unmarshal changes from JSON
		if len(changesJSON) > 0 {
			if err := json.Unmarshal(changesJSON, &log.ChangesApplied); err != nil {
				return nil, fmt.Errorf("failed to unmarshal changes: %w", err)
			}
		}

		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// GetLastSuccessfulSync retrieves the most recent successful git sync
func (s *Store) GetLastSuccessfulSync(ctx context.Context) (*GitSyncLog, error) {
	query := `
		SELECT id, sync_started_at, sync_completed_at, status, commit_hash,
		       commit_message, commit_author, commit_timestamp, error_message,
		       changes_applied, triggered_by, triggered_by_user, created_at
		FROM git_sync_log
		WHERE status = $1
		ORDER BY sync_completed_at DESC
		LIMIT 1
	`

	var log GitSyncLog
	var changesJSON []byte

	err := s.pool.QueryRow(ctx, query, GitSyncStatusSuccess).Scan(
		&log.ID,
		&log.SyncStartedAt,
		&log.SyncCompletedAt,
		&log.Status,
		&log.CommitHash,
		&log.CommitMessage,
		&log.CommitAuthor,
		&log.CommitTimestamp,
		&log.ErrorMessage,
		&changesJSON,
		&log.TriggeredBy,
		&log.TriggeredByUser,
		&log.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get last successful sync: %w", err)
	}

	// Unmarshal changes from JSON
	if len(changesJSON) > 0 {
		if err := json.Unmarshal(changesJSON, &log.ChangesApplied); err != nil {
			return nil, fmt.Errorf("failed to unmarshal changes: %w", err)
		}
	}

	return &log, nil
}
