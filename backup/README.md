# Backups

Dieses Verzeichnis enthält Skripte und Hinweise für tägliche Datenbank-Backups in einen separaten S3-Account/Bucket.

## Tägliches Postgres-Backup

Skript: `backup_db.sh`
- erzeugt ein gzip-komprimiertes `pg_dump` der DB
- lädt es in den Backup-Bucket hoch: `s3://$BACKUP_BUCKET/db/$DB_NAME/$DATE.sql.gz`

Benötigte ENV-Variablen:
- DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
- BACKUP_S3_ENDPOINT, BACKUP_S3_REGION
- BACKUP_S3_ACCESS_KEY_ID, BACKUP_S3_SECRET_ACCESS_KEY
- BACKUP_BUCKET

### Systemd-Timer (Beispiel)

Datei `/etc/systemd/system/synesthesie-backup.service`:
```
[Unit]
Description=Synesthesie DB Backup

[Service]
Type=oneshot
EnvironmentFile=/etc/synesthesie/env
WorkingDirectory=/opt/synesthesie-backend
ExecStart=/bin/sh backup/backup_db.sh
```

Datei `/etc/systemd/system/synesthesie-backup.timer`:
```
[Unit]
Description=Run Synesthesie DB Backup daily

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

Aktivieren:
```
sudo systemctl daemon-reload
sudo systemctl enable --now synesthesie-backup.timer
```

### Retention 90 Tage

Stelle im Backup-S3-Bucket eine Lifecycle-Policy ein:
- Prefix: `db/`
- Expiration: 90 Tage

Damit werden Backups automatisch nach 90 Tagen gelöscht.

## Restore (Kurz)
- Lade die gewünschte Datei aus `s3://$BACKUP_BUCKET/db/$DB_NAME/<timestamp>.sql.gz`
- `gunzip -c dump.sql.gz | psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME`
