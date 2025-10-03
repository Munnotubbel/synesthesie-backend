-- Remove plus_price column from events table
ALTER TABLE events DROP COLUMN IF EXISTS plus_price;

-- Note: If check constraints were modified for allowed_group, they should be reverted
-- to only allow 'all', 'guests', 'bubble' (without 'plus')

