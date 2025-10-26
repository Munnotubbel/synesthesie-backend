# Backup-Setup für VPS

## ⚠️ KRITISCH: Backups müssen eingerichtet werden!

Die Backups laufen **NICHT automatisch**! Sie müssen manuell auf dem VPS eingerichtet werden.

---

## 🤔 Warum Cron und nicht im Backend?

### **Cron-basierte Backups (Empfohlen) ✅**

**Vorteile:**
- ✅ **Unabhängig**: Läuft auch wenn Backend abstürzt
- ✅ **Isolation**: Blockiert Backend nicht (pg_dump kann viel RAM nutzen)
- ✅ **Zuverlässig**: Backend-Updates/Restarts unterbrechen Backups nicht
- ✅ **Standard**: So machen es alle (AWS RDS, Google Cloud SQL, etc.)
- ✅ **Sicher**: Bei Backend-Bug/Crash hast du trotzdem Backups

**Nachteile:**
- ⚠️ Einmalige Einrichtung nötig (diese Anleitung)

### **Backend-basierte Backups ❌**

**Probleme:**
- ❌ Backend-Crash = Kein Backup
- ❌ Backend-Neustart zur Backup-Zeit = Backup fehlgeschlagen
- ❌ Blockiert Backend-Ressourcen
- ❌ Schwieriger zu debuggen
- ❌ Nicht Best Practice

**Wann Backend-Backups OK sind:**
- ✅ Manuelle Admin-Backups vor kritischen Operationen
- ✅ Zusätzlich zu Cron (nicht als Ersatz!)
- ✅ Event-basierte Backups (z.B. vor Daten-Migration)

### **Empfohlene Strategie: 3-2-1 Regel**

```
3 Kopien der Daten
2 verschiedene Speicherorte
1 Kopie extern (offsite)
```

**Für dein Setup:**
1. **Primäre DB** (VPS) ← Produktivdaten
2. **Tägliche Backups** in S3 (separater Account!) ← Dieses Setup
3. **Optional: Wöchentliche Backups** auf zweitem S3 oder NAS ← Zusätzlich

**Sicherheitsmaßnahmen:**
- ✅ Separater S3-Account (nicht der gleiche wie für Medien!)
- ✅ S3 Versioning aktivieren (schützt vor Überschreiben)
- ✅ S3 Object Lock für kritische Backups (verhindert Löschen)
- ✅ Lifecycle Policy (automatisches Löschen nach 90 Tagen)
- ✅ Backup-Monitoring via Backend-API
- ✅ **Regelmäßige Restore-Tests!** (z.B. monatlich)

---

## 📍 WICHTIG: Pfad zum Projekt anpassen!

**In dieser Anleitung steht `/opt/synesthesie-backend` als Beispiel.**

**Ersetze das durch DEINEN tatsächlichen Pfad, z.B.:**
- `/home/munnotubbel/synesthesie`
- `/opt/synesthesie-backend`
- `/var/www/synesthesie-backend`

**Überall wo du `/opt/synesthesie-backend` siehst, ersetze es durch deinen echten Pfad!**

---

## 🚀 Schnell-Setup (Cron-Job)

### 1. AWS CLI installieren (falls nicht vorhanden)

```bash
# Prüfen ob aws CLI installiert ist
which aws

# Falls nicht installiert - Fedora/RHEL/Rocky Linux:
sudo dnf install awscli -y

# Oder Debian/Ubuntu:
sudo apt-get update
sudo apt-get install -y awscli

# Oder universell mit pip:
pip3 install --user awscli
export PATH=$PATH:~/.local/bin

# Oder offizieller AWS Installer (empfohlen):
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip
sudo ./aws/install
rm -rf aws awscliv2.zip

# Testen:
aws --version
```

### 2. Projektpfad finden und Backup-Skripte ausführbar machen

```bash
# Finde wo dein Projekt liegt:
# Option 1: Suche nach docker-compose.yml
find ~ -name "docker-compose.yml" -path "*/synesthesie*" 2>/dev/null

# Option 2: Wo läuft Docker Compose?
docker ps --format "{{.Names}}" | grep synesthesie
# Wenn Container laufen, dann in dem Verzeichnis wo du "docker-compose up" ausgeführt hast

# Beispiel: Projekt liegt in /home/munnotubbel/synesthesie
cd /home/munnotubbel/synesthesie  # DEINEN Pfad hier eintragen!

# Prüfe ob .env existiert:
ls -la .env

# Backup-Skripte ausführbar machen:
chmod +x backup/backup_db.sh backup/run_backup.sh
```

### 3. Manuellen Test durchführen

**WICHTIG:** Je nach Shell (bash/zsh) unterschiedlich!

#### **Variante A: Mit .env Datei (empfohlen)**

```bash
# Für bash UND zsh:
cd /DEIN/PROJEKT/PFAD  # z.B. /home/munnotubbel/synesthesie

# Prüfe docker-compose.yml für den PostgreSQL Port:
cat docker-compose.yml | grep -A 2 "postgres:" | grep ports
# Beispiel Ausgabe: - "5433:5432"  ← Port 5433 ist wichtig!

# ENV-Variablen aus .env laden und exportieren
set -a  # Automatisches Exportieren von Variablen aktivieren
source .env  # .env Datei laden
set +a  # Automatisches Exportieren wieder deaktivieren

# Backup testen
./backup/backup_db.sh
```

#### **Variante B: ENV-Variablen manuell setzen (für Test)**

```bash
# Direkt setzen (funktioniert in bash und zsh):
export DB_HOST=localhost
export DB_PORT=5433  # WICHTIG: Port aus docker-compose.yml!
export DB_USER=synesthesie
export DB_PASSWORD=your_password
export DB_NAME=synesthesie_db
export BACKUP_S3_ENDPOINT=https://s3.your-endpoint.com
export BACKUP_S3_REGION=us-east-1
export BACKUP_S3_ACCESS_KEY_ID=your_key
export BACKUP_S3_SECRET_ACCESS_KEY=your_secret
export BACKUP_BUCKET=synesthesie-backups

# Test ausführen
cd /DEIN/PROJEKT/PFAD  # z.B. /home/munnotubbel/synesthesie
./backup/backup_db.sh
```

#### **Shell-Typ prüfen:**

```bash
# Welche Shell nutze ich?
echo $SHELL

# Ausgabe:
# /bin/bash  → bash
# /bin/zsh   → zsh
# /usr/bin/zsh → zsh
```

### 4. Cron-Job einrichten (täglich um 2 Uhr nachts)

**WICHTIG:** Cron nutzt IMMER `/bin/sh` oder `/bin/bash`, egal welche Shell dein User hat!

#### **Empfohlene Variante (funktioniert mit bash und zsh .env):**

```bash
# Als normaler User:
crontab -e

# Oder als root (für System-Backups):
sudo crontab -e

# Folgende Zeile einfügen (PFAD ANPASSEN!):
0 2 * * * /bin/bash -c 'set -a && source /DEIN/PROJEKT/PFAD/.env && set +a && /DEIN/PROJEKT/PFAD/backup/backup_db.sh' >> /var/log/synesthesie-backup.log 2>&1

# Beispiel für /home/munnotubbel/synesthesie:
0 2 * * * /bin/bash -c 'set -a && source /home/munnotubbel/synesthesie/.env && set +a && /home/munnotubbel/synesthesie/backup/backup_db.sh' >> /var/log/synesthesie-backup.log 2>&1
```

**Erklärung:**
- `0 2 * * *` → Täglich um 2 Uhr nachts
- `/bin/bash -c '...'` → Explizit bash nutzen (nicht sh)
- `set -a` → Alle Variablen automatisch exportieren
- `source .env` → ENV-Variablen laden
- `set +a` → Auto-Export wieder aus
- `>> /var/log/...` → Ausgabe in Log-Datei schreiben

#### **Alternative: Wrapper-Skript (EINFACHSTE Lösung!) ✅**

Das `run_backup.sh` Skript lädt automatisch die .env und führt das Backup aus.

```bash
# Wrapper-Skript sollte bereits im Repo sein:
cd /DEIN/PROJEKT/PFAD  # z.B. /home/munnotubbel/synesthesie
ls -la backup/run_backup.sh

# Falls nicht vorhanden, vom Git-Repo kopieren oder erstellen:
nano backup/run_backup.sh
```

**Inhalt von `run_backup.sh` (sollte schon da sein):**
```bash
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Project directory: $PROJECT_DIR"
echo "Loading environment from: $PROJECT_DIR/.env"

if [ ! -f "$PROJECT_DIR/.env" ]; then
  echo "ERROR: .env file not found at $PROJECT_DIR/.env" >&2
  exit 1
fi

set -a
source "$PROJECT_DIR/.env"
set +a

echo "Environment loaded successfully"
echo "Executing backup script..."

"$SCRIPT_DIR/backup_db.sh"
```

```bash
# Ausführbar machen:
chmod +x backup/run_backup.sh

# Testen:
./backup/run_backup.sh

# Wenn erfolgreich, Cron-Job einrichten:
crontab -e

# Zeile einfügen (PFAD ANPASSEN!):
0 2 * * * /DEIN/PROJEKT/PFAD/backup/run_backup.sh >> /var/log/synesthesie-backup.log 2>&1

# Beispiel:
0 2 * * * /home/munnotubbel/synesthesie/backup/run_backup.sh >> /var/log/synesthesie-backup.log 2>&1
```

### 5. Log-Datei erstellen und Rechte setzen

```bash
# Log-Datei erstellen:
sudo touch /var/log/synesthesie-backup.log

# Rechte für deinen User (nicht root):
sudo chown $(whoami):$(whoami) /var/log/synesthesie-backup.log

# Oder für root Cron:
sudo chown root:root /var/log/synesthesie-backup.log
sudo chmod 644 /var/log/synesthesie-backup.log
```

### 6. Cron-Job Logs prüfen

```bash
# Backup-Logs ansehen
tail -f /var/log/synesthesie-backup.log

# Cron-System-Logs
sudo tail -f /var/log/syslog | grep CRON
```

---

## 📋 Systemd-Setup (Alternative - Empfohlen!)

### 1. Service-Datei erstellen

```bash
sudo nano /etc/systemd/system/synesthesie-backup.service
```

**Inhalt:**
```ini
[Unit]
Description=Synesthesie Database Backup
After=network.target postgresql.service

[Service]
Type=oneshot
User=root
WorkingDirectory=/opt/synesthesie-backend
EnvironmentFile=/opt/synesthesie-backend/.env
ExecStart=/bin/bash /opt/synesthesie-backend/backup/backup_db.sh
StandardOutput=append:/var/log/synesthesie-backup.log
StandardError=append:/var/log/synesthesie-backup.log
```

### 2. Timer-Datei erstellen

```bash
sudo nano /etc/systemd/system/synesthesie-backup.timer
```

**Inhalt:**
```ini
[Unit]
Description=Run Synesthesie DB Backup daily at 2 AM
Requires=synesthesie-backup.service

[Timer]
OnCalendar=daily
OnCalendar=*-*-* 02:00:00
Persistent=true
AccuracySec=1h

[Install]
WantedBy=timers.target
```

### 3. Timer aktivieren

```bash
# Systemd neu laden
sudo systemctl daemon-reload

# Timer aktivieren und starten
sudo systemctl enable synesthesie-backup.timer
sudo systemctl start synesthesie-backup.timer

# Status prüfen
sudo systemctl status synesthesie-backup.timer
sudo systemctl list-timers | grep synesthesie
```

### 4. Manuellen Backup-Test

```bash
# Service manuell ausführen
sudo systemctl start synesthesie-backup.service

# Status und Logs prüfen
sudo systemctl status synesthesie-backup.service
sudo journalctl -u synesthesie-backup.service -n 50 --no-pager
```

---

## 🔧 Troubleshooting

### Problem: "aws CLI not found"

```bash
# Fedora/RHEL/Rocky Linux:
sudo dnf install awscli -y

# Debian/Ubuntu:
sudo apt-get install -y awscli

# Universell (empfohlen):
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip
sudo ./aws/install
aws --version
```

### Problem: "pg_dump: command not found"

```bash
# PostgreSQL Client installieren
sudo apt-get install -y postgresql-client
```

### Problem: Backup wird erstellt, aber nicht hochgeladen

```bash
# S3-Verbindung testen
aws s3 ls s3://$BACKUP_BUCKET \
  --endpoint-url $BACKUP_S3_ENDPOINT \
  --region $BACKUP_S3_REGION

# Oder mit expliziten Credentials:
AWS_ACCESS_KEY_ID=your_key \
AWS_SECRET_ACCESS_KEY=your_secret \
aws s3 ls s3://synesthesie-backups \
  --endpoint-url https://your-endpoint \
  --region us-east-1
```

### Problem: Permission denied

```bash
# Skript ausführbar machen
chmod +x /opt/synesthesie-backend/backup/backup_db.sh

# Cron-Job als root ausführen:
sudo crontab -e
```

---

## 📊 Backup-Überwachung im Backend (Read-Only!)

### Was bedeutet "Backups synchronisieren"?

**WICHTIG:** Es werden KEINE Backups gelöscht oder geändert!

**Sync bedeutet:**
1. Backend liest die Backup-Dateien aus S3
2. Erstellt Datenbank-Einträge für jedes gefundene Backup
3. Du kannst dann im Admin-Dashboard sehen:
   - Wann wurde das letzte Backup erstellt?
   - Wie viele Backups gibt es?
   - Wie groß sind die Backups?

### Backups in Datenbank synchronisieren

Nach dem ersten Backup-Lauf:

```bash
# Als Admin im Frontend:
POST /api/v1/admin/backups/sync

# Oder mit curl:
curl -X POST https://api.synesthesie.de/api/v1/admin/backups/sync \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

**Antwort:**
```json
{
  "message": "Backups synchronized successfully",
  "synced": 5
}
```

### Backup-Status prüfen

```bash
# Liste aller Backups
GET /api/v1/admin/backups

# Statistiken
GET /api/v1/admin/backups/stats
```

**Beispiel-Antwort (Stats):**
```json
{
  "total_backups": 30,
  "completed_backups": 30,
  "failed_backups": 0,
  "total_size_bytes": 68157440,
  "latest_backup": "2025-10-03T02:00:00Z"
}
```

### ⚠️ Backups löschen = DEAKTIVIERT!

**Aus Sicherheitsgründen können Backups NICHT über die API gelöscht werden!**

Backups sind **Disaster Recovery** und sollten nur über:
- S3 Lifecycle Policies (automatisch nach 90 Tagen)
- Direkten S3-Zugriff (manuell, falls wirklich nötig)

gelöscht werden.

**Alte DELETE-Route wurde entfernt!**

---

## 🗓️ S3 Lifecycle-Policy (90 Tage Retention)

### In deinem S3-Provider (Strato/AWS):

1. Gehe zu Bucket-Einstellungen
2. Lifecycle-Regeln erstellen
3. **Prefix:** `db/`
4. **Aktion:** Objekte nach 90 Tagen löschen
5. **Status:** Aktiviert

---

## ⚙️ Spezialfall: Docker Compose Setup

**Wenn Backend + PostgreSQL in Docker laufen:**

### **Wichtig: Port-Mapping beachten!**

```yaml
# docker-compose.yml:
postgres:
  ports:
    - "5433:5432"  # Host-Port 5433 → Container-Port 5432
```

### **Problem: Backend braucht andere DB-Werte als Backup!**

**Backend (im Container):**
- `DB_HOST=postgres` (Docker-Netzwerk)
- `DB_PORT=5432` (interner Port)

**Backup (auf Host):**
- `DB_HOST=localhost` (via Port-Mapping)
- `DB_PORT=5433` (gemappter Port)

### **Lösung: Separate .env.backup Datei**

**1. .env bleibt für Backend (NICHT ändern!):**

```bash
# .env - für Backend im Docker-Container
DB_HOST=postgres   # Docker-Netzwerk
DB_PORT=5432       # Interner Port
DB_USER=synesthesie
DB_PASSWORD=dein_echtes_passwort_hier
DB_NAME=synesthesie_db

# Backup S3 Configuration
BACKUP_S3_ENDPOINT=https://dein-s3-endpoint.com
BACKUP_S3_REGION=us-east-1
BACKUP_S3_ACCESS_KEY_ID=dein_backup_s3_key
BACKUP_S3_SECRET_ACCESS_KEY=dein_backup_s3_secret
BACKUP_BUCKET=synesthesie-backup
```

**2. .env.backup für Backups (NEU erstellen!):**

```bash
# .env.backup - für Backup-Skript auf dem Host
DB_HOST=localhost  # Via Port-Mapping!
DB_PORT=5433       # Gemappter Port aus docker-compose.yml!
DB_USER=synesthesie
DB_PASSWORD=dein_echtes_passwort_hier
DB_NAME=synesthesie_db

# Backup S3 Configuration (gleich wie in .env)
BACKUP_S3_ENDPOINT=https://dein-s3-endpoint.com
BACKUP_S3_REGION=us-east-1
BACKUP_S3_ACCESS_KEY_ID=dein_backup_s3_key
BACKUP_S3_SECRET_ACCESS_KEY=dein_backup_s3_secret
BACKUP_BUCKET=synesthesie-backup
```

**Wie erstellen?**

```bash
cd /home/munnotubbel/synesthesie  # Dein Projekt-Pfad

# .env kopieren:
cp .env .env.backup

# .env.backup anpassen:
nano .env.backup
# Ändere NUR diese 2 Zeilen:
# DB_HOST=localhost
# DB_PORT=5433

# Fertig! run_backup.sh nutzt automatisch .env.backup wenn vorhanden
```

---

## 🚨 Sofort-Backup erstellen (JETZT!)

```bash
# SSH auf VPS
ssh user@your-vps

# Ins Projekt-Verzeichnis (DEINEN Pfad anpassen!)
cd /DEIN/PROJEKT/PFAD  # z.B. /home/munnotubbel/synesthesie

# Prüfe ob alle Skripte da sind:
ls -la backup/

# EINFACHSTE Methode (empfohlen):
./backup/run_backup.sh

# ODER manuell:
set -a && source .env && set +a && ./backup/backup_db.sh

# Prüfen ob Backup in S3 gelandet ist:
set -a && source .env && set +a
aws s3 ls s3://$BACKUP_BUCKET/db/ \
  --endpoint-url $BACKUP_S3_ENDPOINT \
  --region $BACKUP_S3_REGION --recursive
```

**Falls das nicht funktioniert (Shell-Kompatibilität):**

```bash
# Alternative: Variablen einzeln exportieren
cd /opt/synesthesie-backend

# ENV laden (auch für zsh):
while IFS='=' read -r key value; do
  # Kommentare und leere Zeilen überspringen
  [[ $key =~ ^#.*$ ]] && continue
  [[ -z $key ]] && continue
  # Variable exportieren
  export "$key=$value"
done < .env

# Jetzt Backup ausführen:
./backup/backup_db.sh
```

---

## ✅ Checkliste

- [ ] Projekt-Pfad gefunden (wo liegt `docker-compose.yml`?)
- [ ] PostgreSQL Port aus `docker-compose.yml` geprüft (meist 5433)
- [ ] `.env` Datei angepasst (`DB_HOST=localhost`, `DB_PORT=5433`)
- [ ] AWS CLI installiert (`which aws`)
- [ ] PostgreSQL Client installiert (`which pg_dump`)
- [ ] Backup-Skripte ausführbar (`chmod +x backup/*.sh`)
- [ ] Manueller Test erfolgreich (`./backup/run_backup.sh`)
- [ ] Backup in S3 sichtbar (mit `aws s3 ls`)
- [ ] Cron-Job eingerichtet (mit KORREKTEM Pfad!)
- [ ] Log-Datei erstellt (`/var/log/synesthesie-backup.log`)
- [ ] Cron-Job testet morgen Nacht (Logs prüfen!)
- [ ] S3 Lifecycle-Policy (90 Tage) konfiguriert
- [ ] Backups im Backend synchronisiert (`POST /admin/backups/sync`)
- [ ] Restore-Prozess getestet (optional, aber empfohlen!)

---

## 📞 Support

Bei Problemen:
1. Logs prüfen: `tail -f /var/log/synesthesie-backup.log`
2. S3-Verbindung testen (siehe Troubleshooting)
3. Backup-Skript manuell ausführen und Fehler lesen

