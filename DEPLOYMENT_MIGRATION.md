# 🚀 Deployment mit PayPal Migration

## ⚠️ WICHTIG: Automatische Migration

Das Backend führt beim Start **automatisch** die PayPal-Migration aus!

### Was passiert beim Start:

```
1. Backend startet
2. Verbindet zur Datenbank
3. Prüft ob PayPal-Spalten existieren
4. Falls NEIN: Führt Migration aus
5. Falls JA: Überspringt Migration
6. Startet normal weiter
```

---

## 📋 Deployment-Schritte

### 1. Code auf Server hochladen

```bash
# Auf dem Server
cd /path/to/synesthesie-backend
git pull origin main
```

### 2. Docker Image neu bauen

```bash
docker-compose build
```

### 3. Backend neu starten

```bash
docker-compose down
docker-compose up -d
```

### 4. Logs prüfen

```bash
docker logs synesthesie-api --tail 50 -f
```

**Erwartete Logs:**
```
Running manual migrations...
Adding PayPal support columns to tickets table...
✅ PayPal support columns added successfully
Manual migrations completed successfully
Database connection established
Starting server on port 8080
```

---

## ✅ Migration erfolgreich?

Prüfe ob Spalten existieren:

```bash
docker exec -it synesthesie-postgres psql -U synesthesie -d synesthesie -c "\d tickets"
```

**Sollte zeigen:**
```
 payment_provider     | character varying(20) | not null default 'stripe'::character varying
 paypal_order_id      | character varying(255)|
 paypal_capture_id    | character varying(255)|
```

---

## 🔧 Manuelle Migration (falls nötig)

Falls die automatische Migration fehlschlägt:

```bash
docker exec -it synesthesie-postgres psql -U synesthesie -d synesthesie
```

```sql
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS payment_provider VARCHAR(20) NOT NULL DEFAULT 'stripe';
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS paypal_order_id VARCHAR(255);
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS paypal_capture_id VARCHAR(255);
\q
```

Dann Backend neu starten:
```bash
docker-compose restart
```

---

## 🎯 Nach dem Deployment

### Test 1: Prüfe Backend-Logs
```bash
docker logs synesthesie-api --tail 100 | grep -E "(Migration|PayPal|CheckPendingCancellations)"
```

### Test 2: Prüfe Polling
Warte 10 Sekunden und prüfe ob Polling läuft:
```bash
docker logs synesthesie-api --tail 50 | grep "CheckPendingCancellations"
```

**Sollte zeigen:**
```
✅ CheckPendingCancellations: Checking X tickets in grace period
✅ Payment check: PayPal order ... status: COMPLETED
✅ Payment check: Completed PayPal ticket ... confirmed as paid
```

**OHNE ERROR!**

---

## 🚨 Troubleshooting

### Problem: Migration läuft nicht
```bash
# Prüfe Datenbankverbindung
docker logs synesthesie-api | grep "Database connection"

# Prüfe Fehler
docker logs synesthesie-api | grep -i error
```

### Problem: Spalten fehlen noch
```bash
# Manuelle Migration ausführen (siehe oben)
# Dann Backend neu starten
docker-compose restart
```

---

## 📊 Zusammenfassung

| Schritt | Status | Command |
|---------|--------|---------|
| **1. Code hochladen** | ⏳ | `git pull` |
| **2. Image bauen** | ⏳ | `docker-compose build` |
| **3. Backend starten** | ⏳ | `docker-compose up -d` |
| **4. Migration prüfen** | ⏳ | `docker logs synesthesie-api` |
| **5. Spalten prüfen** | ⏳ | `\d tickets` |
| **6. Polling testen** | ⏳ | Warte 10 Sek, prüfe Logs |

---

**Nach erfolgreichem Deployment:**
- ✅ PayPal-Spalten existieren
- ✅ Backend-Polling läuft
- ✅ Tickets werden automatisch auf `paid` gesetzt
- ✅ Grace Period funktioniert

**Edge-Case gelöst!** 🎉

