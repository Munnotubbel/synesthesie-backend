-- Remove PayPal support from tickets table
DROP INDEX IF EXISTS idx_tickets_paypal_order_id;
DROP INDEX IF EXISTS idx_tickets_payment_provider;

ALTER TABLE tickets DROP COLUMN IF EXISTS paypal_capture_id;
ALTER TABLE tickets DROP COLUMN IF EXISTS paypal_order_id;
ALTER TABLE tickets DROP COLUMN IF EXISTS payment_provider;

