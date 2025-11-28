-- Add PayPal support to tickets table
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS payment_provider VARCHAR(20) DEFAULT 'stripe';
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS paypal_order_id VARCHAR(255);
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS paypal_capture_id VARCHAR(255);

-- Create index for PayPal lookups
CREATE INDEX IF NOT EXISTS idx_tickets_paypal_order_id ON tickets(paypal_order_id);
CREATE INDEX IF NOT EXISTS idx_tickets_payment_provider ON tickets(payment_provider);

-- Add comment
COMMENT ON COLUMN tickets.payment_provider IS 'Payment provider used: stripe or paypal';
COMMENT ON COLUMN tickets.paypal_order_id IS 'PayPal Order ID for tracking';
COMMENT ON COLUMN tickets.paypal_capture_id IS 'PayPal Capture ID for refunds';

