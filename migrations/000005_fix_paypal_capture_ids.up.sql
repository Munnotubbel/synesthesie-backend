-- Fix PayPal Capture IDs
-- Problem: Alte Tickets haben Order ID statt Capture ID gespeichert
-- Lösung: Setze PayPal Capture ID auf NULL für alte Test-Tickets
--         Neue Tickets werden korrekt erstellt

-- Setze Capture ID auf NULL für alle bestehenden PayPal-Tickets
-- (Refunds für diese Tickets nicht möglich, aber neue Tickets funktionieren)
UPDATE tickets
SET paypal_capture_id = NULL
WHERE payment_provider = 'paypal'
  AND status = 'paid'
  AND paypal_capture_id IS NOT NULL
  AND created_at < NOW();

-- Log: Zeige wie viele Tickets betroffen waren
-- (wird in Migration-Logs erscheinen)

