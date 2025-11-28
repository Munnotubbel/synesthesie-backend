# 💳 PayPal Integration - Setup Guide

## 🎯 Übersicht

Das Synesthesie Backend unterstützt **parallel** zwei Payment-Provider:
- ✅ **Stripe** (Standard, immer aktiv)
- ✅ **PayPal** (Optional, kann aktiviert werden)

User können beim Ticket-Kauf wählen, welchen Provider sie nutzen möchten.

---

## 🚀 PayPal aktivieren

### **1. PayPal Developer Account erstellen**

1. Gehe zu: https://developer.paypal.com
2. Melde dich an oder erstelle einen Account
3. Gehe zu **Dashboard** → **Apps & Credentials**

### **2. Sandbox Credentials holen (für Testing)**

1. Wähle **Sandbox** Tab
2. Erstelle eine neue App oder nutze die Default-App
3. Kopiere:
   - **Client ID**
   - **Secret**

### **3. Live Credentials holen (für Production)**

1. Wähle **Live** Tab
2. Erstelle eine neue App
3. Kopiere:
   - **Client ID**
   - **Secret**

---

## ⚙️ Umgebungsvariablen setzen

### **Sandbox (Testing):**

```bash
PAYPAL_ENABLED=true
PAYPAL_MODE=sandbox
PAYPAL_CLIENT_ID=dein_sandbox_client_id
PAYPAL_SECRET=dein_sandbox_secret
PAYPAL_SUCCESS_URL=https://synesthesie.de/payment/success
PAYPAL_CANCEL_URL=https://synesthesie.de/payment/cancel
```

### **Live (Production):**

```bash
PAYPAL_ENABLED=true
PAYPAL_MODE=live
PAYPAL_CLIENT_ID=dein_live_client_id
PAYPAL_SECRET=dein_live_secret
PAYPAL_SUCCESS_URL=https://synesthesie.de/payment/success
PAYPAL_CANCEL_URL=https://synesthesie.de/payment/cancel
```

---

## 🔔 Webhook einrichten

### **1. Webhook in PayPal Dashboard erstellen**

1. Gehe zu: https://developer.paypal.com/dashboard/webhooks
2. Klicke auf **Add Webhook**
3. Webhook URL: `https://api.synesthesie.de/api/v1/paypal/webhook`
4. Wähle folgende Events:
   - ✅ `PAYMENT.CAPTURE.COMPLETED`
   - ✅ `PAYMENT.CAPTURE.DENIED`
   - ✅ `PAYMENT.CAPTURE.REFUNDED`
   - ✅ `CHECKOUT.ORDER.APPROVED`
5. Speichern

### **2. Webhook ID kopieren**

Nach dem Erstellen siehst du eine **Webhook ID**. Kopiere diese und setze:

```bash
PAYPAL_WEBHOOK_ID=deine_webhook_id
```

---

## 🗄️ Datenbank-Migration

Die Migration läuft automatisch beim nächsten Start:

```sql
-- Wird automatisch ausgeführt: migrations/000004_add_paypal_support.up.sql
ALTER TABLE tickets ADD COLUMN payment_provider VARCHAR(20) DEFAULT 'stripe';
ALTER TABLE tickets ADD COLUMN paypal_order_id VARCHAR(255);
ALTER TABLE tickets ADD COLUMN paypal_capture_id VARCHAR(255);
```

---

## 🧪 Testing

### **1. Ticket mit PayPal kaufen (API)**

```bash
curl -X POST https://api.synesthesie.de/api/v1/user/tickets \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "event_id": "event-uuid",
    "includes_pickup": false,
    "payment_provider": "paypal"
  }'
```

**Response:**
```json
{
  "ticket_id": "ticket-uuid",
  "checkout_url": "https://www.sandbox.paypal.com/checkoutnow?token=...",
  "payment_provider": "paypal"
}
```

### **2. PayPal Sandbox Test-Accounts**

PayPal erstellt automatisch Test-Accounts:
- **Buyer Account:** Zum Testen von Käufen
- **Seller Account:** Dein Business-Account

Finde diese unter: https://developer.paypal.com/dashboard/accounts

### **3. Webhook testen**

PayPal sendet Test-Events im Dashboard:
1. Gehe zu **Webhooks**
2. Wähle deinen Webhook
3. Klicke auf **Send Test Event**
4. Wähle `PAYMENT.CAPTURE.COMPLETED`

---

## 🔍 Monitoring & Debugging

### **Logs prüfen:**

```bash
# Backend-Logs zeigen PayPal-Events
tail -f /var/log/synesthesie-backend.log | grep PayPal
```

**Beispiel-Log:**
```
PayPal webhook received: event_type=PAYMENT.CAPTURE.COMPLETED, id=WH-xxx
PayPal webhook: ticket abc-123 marked as paid (capture: CAPTURE-xxx)
```

### **Ticket-Status prüfen:**

```sql
-- Alle PayPal-Tickets anzeigen
SELECT id, status, payment_provider, paypal_order_id, paypal_capture_id
FROM tickets
WHERE payment_provider = 'paypal';
```

---

## 💰 Gebühren-Vergleich

| Provider | Gebühren (EU) | Auszahlung |
|----------|---------------|------------|
| **Stripe** | 1,5% + 0,25€ | 2-7 Tage |
| **PayPal** | 2,49% + 0,35€ | 1-2 Tage |

**Bei 50€ Ticket:**
- Stripe: 50€ - 1,00€ = **49,00€**
- PayPal: 50€ - 1,60€ = **48,40€**

**→ Stripe ist günstiger!**

---

## 🔄 Refunds

Refunds funktionieren automatisch für beide Provider:

```go
// Backend macht automatisch:
if ticket.PaymentProvider == "paypal" {
    paypalProvider.ProcessRefund(ticket, amount)
} else {
    stripeProvider.ProcessRefund(ticket, amount)
}
```

---

## ⚠️ Troubleshooting

### **Problem: "PayPal is not enabled"**

**Lösung:** Setze `PAYPAL_ENABLED=true` in den Umgebungsvariablen.

### **Problem: "Failed to get PayPal access token"**

**Lösung:**
- Prüfe `PAYPAL_CLIENT_ID` und `PAYPAL_SECRET`
- Prüfe `PAYPAL_MODE` (sandbox oder live)
- Stelle sicher, dass die Credentials zum Mode passen

### **Problem: Webhook wird nicht empfangen**

**Lösung:**
- Prüfe, ob die Webhook-URL erreichbar ist
- Prüfe PayPal Dashboard → Webhooks → Event History
- Prüfe Backend-Logs

### **Problem: Ticket bleibt auf "pending"**

**Lösung:**
- Prüfe, ob Webhook empfangen wurde (Logs)
- Prüfe, ob `custom_id` im Webhook gesetzt ist
- Manuell Ticket-Status setzen:
  ```sql
  UPDATE tickets SET status = 'paid', paypal_capture_id = 'CAPTURE-xxx' WHERE id = 'ticket-uuid';
  ```

---

## 📊 Statistiken

### **Tickets nach Provider:**

```sql
SELECT
    payment_provider,
    COUNT(*) as count,
    SUM(total_amount) as revenue
FROM tickets
WHERE status = 'paid'
GROUP BY payment_provider;
```

---

## 🔒 Sicherheit

- ✅ PayPal Webhooks sollten verifiziert werden (TODO: Signature-Verification)
- ✅ Credentials niemals im Code speichern
- ✅ Separate Sandbox/Live Credentials verwenden
- ✅ Webhook-URL über HTTPS

---

## 📚 Weitere Ressourcen

- PayPal Developer Docs: https://developer.paypal.com/docs/
- PayPal Sandbox: https://www.sandbox.paypal.com
- PayPal Dashboard: https://developer.paypal.com/dashboard/
- PayPal Webhook Events: https://developer.paypal.com/api/rest/webhooks/

---

**Bei Fragen:** info@synesthesie.de

