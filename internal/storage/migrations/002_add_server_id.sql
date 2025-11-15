-- Add server_id column to track which server allocated each lease
-- This enables debugging and monitoring in active/active deployments

-- Add allocated_by column (nullable for backward compatibility)
ALTER TABLE leases ADD COLUMN allocated_by TEXT;

-- Create index for querying by server
CREATE INDEX idx_leases_allocated_by ON leases(allocated_by);

-- Add comment
COMMENT ON COLUMN leases.allocated_by IS 'Server ID that allocated this lease (for HA deployments)';
