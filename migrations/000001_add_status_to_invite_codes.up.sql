-- Add status column and update invite codes structure
-- Remove old boolean columns
ALTER TABLE invite_codes DROP COLUMN IF EXISTS is_used;
ALTER TABLE invite_codes DROP COLUMN IF EXISTS is_active;

-- Rename existing columns
ALTER TABLE invite_codes RENAME COLUMN used_by TO registered_by;
ALTER TABLE invite_codes RENAME COLUMN used_at TO registered_at;

-- Add new columns
ALTER TABLE invite_codes ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'new';
ALTER TABLE invite_codes ADD COLUMN viewed_at TIMESTAMPTZ;

-- Create index on status for better performance
CREATE INDEX IF NOT EXISTS idx_invite_codes_status ON invite_codes(status);