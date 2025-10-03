-- Add plus_price column to events table
ALTER TABLE events ADD COLUMN IF NOT EXISTS plus_price DECIMAL(10,2) NOT NULL DEFAULT 50.00;

-- Update allowed_group check constraint to include 'plus'
-- Note: PostgreSQL check constraints might need to be dropped and recreated
-- If a check constraint exists on allowed_group, it should be updated to include 'plus'

-- Add comment for documentation
COMMENT ON COLUMN events.plus_price IS 'Price for users in the plus group';

