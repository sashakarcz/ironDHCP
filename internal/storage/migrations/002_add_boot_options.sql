-- Add PXE/iPXE boot options support
-- This migration adds TFTP server and boot filename fields to reservations

-- Add boot options columns to reservations table
ALTER TABLE reservations
ADD COLUMN IF NOT EXISTS tftp_server TEXT,
ADD COLUMN IF NOT EXISTS boot_filename TEXT;

-- Add indexes for boot options queries
CREATE INDEX IF NOT EXISTS idx_reservations_tftp ON reservations(tftp_server) WHERE tftp_server IS NOT NULL;

-- Add comments
COMMENT ON COLUMN reservations.tftp_server IS 'TFTP server for PXE boot (DHCP option 66)';
COMMENT ON COLUMN reservations.boot_filename IS 'Boot filename for PXE/iPXE (DHCP option 67)';
