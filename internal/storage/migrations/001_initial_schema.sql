-- ironDHCP Initial Schema
-- PostgreSQL 12+ required for generated columns and advanced features

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Leases table (active and expired)
CREATE TABLE IF NOT EXISTS leases (
    id BIGSERIAL PRIMARY KEY,
    ip INET NOT NULL,
    mac MACADDR NOT NULL,
    hostname TEXT,
    subnet CIDR NOT NULL,

    -- Lease timing
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,

    -- State
    state TEXT NOT NULL DEFAULT 'active', -- active, expired, released, declined

    -- Metadata
    client_id TEXT,
    vendor_class TEXT,
    user_class TEXT,

    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_ip_per_subnet UNIQUE(ip, subnet),
    CONSTRAINT valid_state CHECK (state IN ('active', 'expired', 'released', 'declined'))
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_leases_mac ON leases(mac);
CREATE INDEX IF NOT EXISTS idx_leases_ip ON leases(ip);
CREATE INDEX IF NOT EXISTS idx_leases_expires ON leases(expires_at) WHERE state = 'active';
CREATE INDEX IF NOT EXISTS idx_leases_subnet ON leases(subnet);
CREATE INDEX IF NOT EXISTS idx_leases_state ON leases(state);
CREATE INDEX IF NOT EXISTS idx_leases_last_seen ON leases(last_seen);

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_leases_updated_at ON leases;
CREATE TRIGGER update_leases_updated_at
    BEFORE UPDATE ON leases
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Reservations (from config, cached for performance)
CREATE TABLE IF NOT EXISTS reservations (
    id BIGSERIAL PRIMARY KEY,
    mac MACADDR NOT NULL UNIQUE,
    ip INET NOT NULL,
    hostname TEXT NOT NULL,
    subnet CIDR NOT NULL,
    description TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_reservation_ip UNIQUE(ip, subnet)
);

CREATE INDEX IF NOT EXISTS idx_reservations_mac ON reservations(mac);
CREATE INDEX IF NOT EXISTS idx_reservations_ip ON reservations(ip);
CREATE INDEX IF NOT EXISTS idx_reservations_subnet ON reservations(subnet);

DROP TRIGGER IF EXISTS update_reservations_updated_at ON reservations;
CREATE TRIGGER update_reservations_updated_at
    BEFORE UPDATE ON reservations
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Git sync state (audit and rollback capability)
CREATE TABLE IF NOT EXISTS git_sync_log (
    id BIGSERIAL PRIMARY KEY,
    commit_hash TEXT NOT NULL,
    sync_started_at TIMESTAMPTZ NOT NULL,
    sync_completed_at TIMESTAMPTZ,
    status TEXT NOT NULL,  -- syncing, applied, failed
    error_message TEXT,
    config_diff TEXT,  -- Store the diff for audit

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_sync_status CHECK (status IN ('syncing', 'applied', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_git_sync_log_commit ON git_sync_log(commit_hash);
CREATE INDEX IF NOT EXISTS idx_git_sync_log_status ON git_sync_log(status);
CREATE INDEX IF NOT EXISTS idx_git_sync_log_created ON git_sync_log(created_at DESC);

-- Current active config (singleton table)
CREATE TABLE IF NOT EXISTS active_config (
    id INTEGER PRIMARY KEY DEFAULT 1,
    commit_hash TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL,
    config_yaml TEXT NOT NULL,

    CONSTRAINT single_row CHECK (id = 1)
);

-- Insert initial empty config (only if table is empty)
INSERT INTO active_config (commit_hash, applied_at, config_yaml)
SELECT 'initial', NOW(), ''
WHERE NOT EXISTS (SELECT 1 FROM active_config);

-- Function to get next available IP in a pool (LRU)
CREATE OR REPLACE FUNCTION get_next_available_ip(
    p_subnet CIDR,
    p_range_start INET,
    p_range_end INET
)
RETURNS INET AS $$
DECLARE
    v_ip INET;
BEGIN
    -- Try to find an expired lease first (LRU)
    SELECT ip INTO v_ip
    FROM leases
    WHERE subnet = p_subnet
      AND ip >= p_range_start
      AND ip <= p_range_end
      AND state IN ('expired', 'released')
    ORDER BY expires_at ASC
    LIMIT 1;

    IF FOUND THEN
        RETURN v_ip;
    END IF;

    -- If no expired leases, find first never-used IP
    -- This requires generating IPs and checking against leases table
    -- For now, return NULL to indicate pool exhaustion
    -- The application will handle IP generation and checking

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Statistics view for monitoring
CREATE OR REPLACE VIEW lease_statistics AS
SELECT
    subnet,
    COUNT(*) FILTER (WHERE state = 'active') AS active_leases,
    COUNT(*) FILTER (WHERE state = 'expired') AS expired_leases,
    COUNT(*) FILTER (WHERE state = 'released') AS released_leases,
    COUNT(*) FILTER (WHERE state = 'declined') AS declined_leases,
    MIN(expires_at) FILTER (WHERE state = 'active') AS next_expiry,
    MAX(last_seen) AS last_activity
FROM leases
GROUP BY subnet;

-- Comments for documentation
COMMENT ON TABLE leases IS 'DHCP lease records including active, expired, and historical leases';
COMMENT ON TABLE reservations IS 'Static IP reservations synced from Git configuration';
COMMENT ON TABLE git_sync_log IS 'Audit log of Git configuration synchronization events';
COMMENT ON TABLE active_config IS 'Current active configuration (singleton)';
COMMENT ON COLUMN leases.state IS 'Lease state: active (in use), expired (timed out), released (client released), declined (IP conflict)';
