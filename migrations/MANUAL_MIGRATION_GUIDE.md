# Manuelle Migrations-Anleitung

## ⚠️ Wichtig

Dieses Projekt verwendet **GORM AutoMigrate**, nicht golang-migrate!

Die SQL-Dateien in diesem Ordner werden **NICHT automatisch** ausgeführt.

---

## Wie man Migrationen ausführt:

### **Option 1: Direkt in PostgreSQL (empfohlen)**

```bash
# SSH zum Server
ssh munnotubbel@synesthesie

# In PostgreSQL-Container
docker exec -it synesthesie-postgres psql -U synesthesie -d synesthesie_db

# Migration ausführen
\i /path/to/migration.up.sql
```

### **Option 2: Via Docker Exec**

```bash
# Auf dem Server
docker exec -i synesthesie-postgres psql -U synesthesie -d synesthesie_db < migrations/000005_fix_paypal_capture_ids.up.sql
```

### **Option 3: Via psql (lokal)**

```bash
# Von deinem lokalen Rechner
psql -h synesthesie -U synesthesie -d synesthesie_db -f migrations/000005_fix_paypal_capture_ids.up.sql
```

---

## Aktuelle Migrationen:

### **000005_fix_paypal_capture_ids** (NEU!)

**Problem:** Alte PayPal-Tickets haben Order ID statt Capture ID gespeichert

**Lösung:** Setze `paypal_capture_id` auf NULL für alte Tickets

**Ausführen:**
```bash
docker exec -i synesthesie-postgres psql -U synesthesie -d synesthesie_db << 'EOF'
UPDATE tickets
SET paypal_capture_id = NULL
WHERE payment_provider = 'paypal'
  AND status = 'paid'
  AND paypal_capture_id IS NOT NULL
  AND created_at < NOW();
EOF
```

**Prüfen:**
```bash
docker exec -it synesthesie-postgres psql -U synesthesie -d synesthesie_db -c "SELECT id, status, payment_provider, paypal_order_id, paypal_capture_id FROM tickets WHERE payment_provider = 'paypal';"
```

---

## Warum keine automatischen Migrationen?

GORM AutoMigrate kann:
- ✅ Tabellen erstellen
- ✅ Spalten hinzufügen
- ❌ **KEINE Daten-Migrationen** (UPDATE, DELETE, etc.)
- ❌ **KEINE komplexen Schema-Änderungen**

Für Daten-Migrationen müssen wir SQL manuell ausführen.

---

## Alternative: golang-migrate integrieren (für die Zukunft)

Falls gewünscht, können wir golang-migrate integrieren:

```go
import "github.com/golang-migrate/migrate/v4"

func RunMigrations(db *gorm.DB) error {
    m, err := migrate.New(
        "file://migrations",
        "postgres://...",
    )
    if err != nil {
        return err
    }
    return m.Up()
}
```

Aber das ist ein größeres Refactoring.

