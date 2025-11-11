-- Git sync audit log
-- Tracks all git repository synchronization operations

-- Drop old version of table if it exists (from incomplete migration)
DROP TABLE IF EXISTS git_sync_log CASCADE;

CREATE TABLE git_sync_log (
    id BIGSERIAL PRIMARY KEY,
    sync_started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sync_completed_at TIMESTAMPTZ,
    status TEXT NOT NULL, -- 'success', 'failed', 'in_progress'
    commit_hash TEXT,
    commit_message TEXT,
    commit_author TEXT,
    commit_timestamp TIMESTAMPTZ,
    error_message TEXT,
    changes_applied JSONB, -- Summary of changes applied (subnets added/updated/deleted, reservations changed, etc.)
    triggered_by TEXT NOT NULL, -- 'poll', 'manual', 'startup'
    triggered_by_user TEXT, -- Username if triggered manually via API
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_git_sync_log_started ON git_sync_log(sync_started_at DESC);
CREATE INDEX IF NOT EXISTS idx_git_sync_log_status ON git_sync_log(status);
CREATE INDEX IF NOT EXISTS idx_git_sync_log_commit ON git_sync_log(commit_hash);

COMMENT ON TABLE git_sync_log IS 'Audit log of all git repository synchronization operations';
COMMENT ON COLUMN git_sync_log.status IS 'Sync status: success, failed, or in_progress';
COMMENT ON COLUMN git_sync_log.triggered_by IS 'Source of sync trigger: poll, manual, or startup';
COMMENT ON COLUMN git_sync_log.changes_applied IS 'JSON summary of configuration changes applied during sync';
