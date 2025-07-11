-- Revert invite codes structure changes
-- Drop the index
DROP INDEX IF EXISTS idx_invite_codes_status;

-- Remove new columns
ALTER TABLE invite_codes DROP COLUMN IF EXISTS status;
ALTER TABLE invite_codes DROP COLUMN IF EXISTS viewed_at;

-- Rename columns back
ALTER TABLE invite_codes RENAME COLUMN registered_by TO used_by;
ALTER TABLE invite_codes RENAME COLUMN registered_at TO used_at;

-- Add back old boolean columns
ALTER TABLE invite_codes ADD COLUMN is_used BOOLEAN DEFAULT false;
ALTER TABLE invite_codes ADD COLUMN is_active BOOLEAN DEFAULT true;