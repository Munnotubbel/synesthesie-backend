# Synesthesie Backend API Dokumentation

Alle Endpunkte sind unter dem Präfix `/api/v1` erreichbar.

---

### Assets und Medien

#### `POST /api/v1/admin/assets/upload`
- Beschreibung: Upload von Dateien (Admin). Unterstützt Bilder und Audio.
- Auth: Admin erforderlich.
- Content-Type: `multipart/form-data`
- Felder:
  - `file` (required): Datei
  - `kind` (optional): `images`|`audio` (Default: `images`)
- Verhalten:
  - Bilder: werden lokal gespeichert und zusätzlich in `MEDIA_IMAGES_BUCKET` hochgeladen.
  - Audio (.flac): werden direkt in `MEDIA_AUDIO_BUCKET` hochgeladen (kein lokaler Speicher).
- Response (200 OK):
  ```json
  { "id": "uuid", "key": "images/.. oder audio/..", "path": "string?", "size": 12345, "checksum": "..." }
  ```

#### `POST /api/v1/admin/assets/images/sync-missing`
- Beschreibung: Synchronisiert fehlende Bilddateien aus dem Image-Bucket lokal (Cache/Erstbefüllung).
- Auth: Admin erforderlich.
- Response (200 OK):
  ```json
  { "synced": 42 }
  ```

#### `GET /api/v1/user/assets/:id/download`
- Beschreibung: Download eines Assets für eingeloggte Benutzer (Range-Unterstützung).
- Auth: erforderlich.
- Verhalten:
  - Audio: 302 Redirect auf eine kurzlebige presigned S3-URL. Optionaler lokaler Audio-Cache, wenn `MEDIA_CACHE_AUDIO=true`.
  - Bilder: Stream von der lokalen Platte. Falls Datei fehlt, wird sie einmalig aus dem Image-Bucket nachgeladen und lokal gecached.

---

### Bildergalerie (User)

#### `GET /api/v1/user/images`
- Beschreibung: Ruft alle öffentlichen Bilder ab.
- Auth: erforderlich.
- Query-Parameter:
  - `page` (optional, default: `1`): Seitennummer
  - `limit` (optional, default: `20`): Anzahl pro Seite (max: 100)
- Response (200 OK):
  ```json
  {
    "images": [
      {
        "id": "uuid",
        "title": "string",
        "description": "string",
        "presigned_url": "string (15 Min gültig)",
        "created_at": "time.Time"
      }
    ],
    "pagination": { "page": 1, "limit": 20, "total": 50 }
  }
  ```

#### `GET /api/v1/user/images/:id`
- Beschreibung: Ruft ein einzelnes öffentliches Bild ab.
- Auth: erforderlich.
- Response (200 OK):
  ```json
  {
    "id": "uuid",
    "title": "string",
    "description": "string",
    "presigned_url": "string (15 Min gültig)",
    "created_at": "time.Time"
  }
  ```

#### `GET /api/v1/user/images/:id/file`
- Beschreibung: Serviert ein öffentliches Bild direkt aus dem lokalen Cache (schnell!).
- Auth: erforderlich.
- Response: Bilddatei (image/webp bevorzugt, fallback zu image/jpeg/png)
- Headers:
  - `Cache-Control: public, max-age=31536000` (1 Jahr Cache)
  - `Content-Type: image/webp` (oder original Format)
  - `Content-Disposition: inline; filename="..."`
- Hinweis:
  - Lädt automatisch von S3 nach falls nicht lokal vorhanden
  - **WebP bevorzugt**: Background-Worker konvertiert automatisch zu WebP (90% Qualität)
  - Deutlich schneller als presigned URLs von Strato S3
  - WebP ist 25-35% kleiner bei gleicher Qualität

---

### Bildergalerie (Admin)

#### `GET /api/v1/admin/images`
- Beschreibung: Ruft alle Bilder ab (privat und öffentlich).
- Auth: Admin erforderlich.
- Query-Parameter: `page`, `limit`
- Response (200 OK):
  ```json
  {
    "images": [
      {
        "id": "uuid",
        "title": "string",
        "description": "string",
        "visibility": "private|public",
        "created_at": "time.Time"
      }
    ],
    "pagination": { "page": 1, "limit": 20, "total": 50 }
  }
  ```

#### `GET /api/v1/admin/images/:id`
- Beschreibung: Ruft Details eines einzelnen Bildes ab.
- Auth: Admin erforderlich.
- Response (200 OK):
  ```json
  {
    "id": "uuid",
    "title": "string",
    "description": "string",
    "visibility": "private|public",
    "asset": {
      "filename": "string",
      "mime_type": "string",
      "size_bytes": 12345
    },
    "created_at": "time.Time",
    "updated_at": "time.Time"
  }
  ```

#### `GET /api/v1/admin/images/:id/file`
- Beschreibung: Serviert ein Bild direkt aus dem lokalen Cache (alle Bilder, auch private).
- Auth: Admin erforderlich.
- Response: Bilddatei (image/jpeg, image/png, image/webp)
- Headers:
  - `Cache-Control: private, max-age=3600` (1 Stunde Cache)
  - `Content-Type: image/...`
  - `Content-Disposition: inline; filename="..."`

#### `POST /api/v1/admin/images`
- Beschreibung: Lädt ein einzelnes Bild hoch.
- Auth: Admin erforderlich.
- Content-Type: `multipart/form-data`
- Felder:
  - `file` (required): Bilddatei (JPEG, PNG, WebP; max 25MB)
  - `title` (optional): Titel
  - `description` (optional): Beschreibung
  - `visibility` (optional): `private` oder `public` (default: `private`)
- Response (201 Created):
  ```json
  {
    "id": "uuid",
    "title": "string",
    "description": "string",
    "visibility": "string",
    "created_at": "time.Time"
  }
  ```
- Hinweis: Rate Limiting - max. 30 Uploads pro Tag pro Admin.

#### `POST /api/v1/admin/images/batch`
- Beschreibung: Lädt mehrere Bilder gleichzeitig hoch.
- Auth: Admin erforderlich.
- Content-Type: `multipart/form-data`
- Felder:
  - `files` (required): Mehrere Bilddateien
  - `visibility` (optional): `private` oder `public` (default: `private`)
- Response (201 Created):
  ```json
  {
    "images": [
      { "id": "uuid", "title": "filename.jpg", "visibility": "string" }
    ],
    "uploaded": 5,
    "failed": 0
  }
  ```

#### `PUT /api/v1/admin/images/:id/visibility`
- Beschreibung: Ändert die Sichtbarkeit eines Bildes.
- Auth: Admin erforderlich.
- Request Body:
  ```json
  { "visibility": "private|public" }
  ```
- Response (200 OK):
  ```json
  { "message": "visibility updated successfully", "visibility": "public" }
  ```

#### `PUT /api/v1/admin/images/:id/metadata`
- Beschreibung: Aktualisiert Titel und Beschreibung eines Bildes.
- Auth: Admin erforderlich.
- Request Body:
  ```json
  { "title": "string", "description": "string" }
  ```
- Response (200 OK):
  ```json
  { "message": "image updated successfully" }
  ```

#### `DELETE /api/v1/admin/images/:id`
- Beschreibung: Löscht ein Bild inkl. S3-Objekt.
- Auth: Admin erforderlich.
- Response (200 OK):
  ```json
  { "message": "image deleted successfully", "id": "uuid" }
  ```

---

### Music Sets (User)

#### `GET /api/v1/user/music-sets`
- Beschreibung: Ruft alle öffentlichen Music Sets ab.
- Auth: erforderlich.
- Query-Parameter: `page`, `limit`
- Response (200 OK):
  ```json
  {
    "music_sets": [
      {
        "id": "uuid",
        "title": "string",
        "description": "string",
        "tracks": [
          {
            "id": "uuid",
            "title": "string",
            "artist": "string",
            "track_order": 1,
            "duration": 180,
            "presigned_url": "string (15 Min gültig)"
          }
        ],
        "created_at": "time.Time"
      }
    ],
    "pagination": { "page": 1, "limit": 20, "total": 10 }
  }
  ```

#### `GET /api/v1/user/music-sets/:id`
- Beschreibung: Ruft ein einzelnes öffentliches Music Set ab.
- Auth: erforderlich.
- Response (200 OK):
  ```json
  {
    "id": "uuid",
    "title": "string",
    "description": "string",
    "tracks": [
      {
        "id": "uuid",
        "title": "string",
        "artist": "string",
        "track_order": 1,
        "duration": 180,
        "presigned_url": "string (15 Min gültig)"
      }
    ],
    "created_at": "time.Time"
  }
  ```

---

### Music Sets (Admin)

#### `GET /api/v1/admin/music-sets`
- Beschreibung: Ruft alle Music Sets ab (privat und öffentlich).
- Auth: Admin erforderlich.
- Query-Parameter: `page`, `limit`
- Response (200 OK):
  ```json
  {
    "music_sets": [
      {
        "id": "uuid",
        "title": "string",
        "description": "string",
        "visibility": "private|public",
        "track_count": 5,
        "created_at": "time.Time",
        "updated_at": "time.Time"
      }
    ],
    "pagination": { "page": 1, "limit": 20, "total": 10 }
  }
  ```

#### `GET /api/v1/admin/music-sets/:id`
- Beschreibung: Ruft Details eines Music Sets mit allen Tracks ab.
- Auth: Admin erforderlich.
- Response (200 OK):
  ```json
  {
    "id": "uuid",
    "title": "string",
    "description": "string",
    "visibility": "private|public",
    "tracks": [
      {
        "id": "uuid",
        "title": "string",
        "artist": "string",
        "track_order": 1,
        "duration": 180,
        "filename": "track.flac",
        "mime_type": "audio/flac",
        "size_bytes": 50000000,
        "created_at": "time.Time"
      }
    ],
    "created_at": "time.Time",
    "updated_at": "time.Time"
  }
  ```

#### `POST /api/v1/admin/music-sets`
- Beschreibung: Erstellt ein neues Music Set.
- Auth: Admin erforderlich.
- Request Body:
  ```json
  { "title": "string (required)", "description": "string (optional)" }
  ```
- Response (201 Created):
  ```json
  {
    "id": "uuid",
    "title": "string",
    "description": "string",
    "visibility": "private",
    "created_at": "time.Time"
  }
  ```

#### `PUT /api/v1/admin/music-sets/:id`
- Beschreibung: Aktualisiert Titel und Beschreibung eines Music Sets.
- Auth: Admin erforderlich.
- Request Body:
  ```json
  { "title": "string", "description": "string" }
  ```
- Response (200 OK):
  ```json
  { "message": "music set updated successfully" }
  ```

#### `PUT /api/v1/admin/music-sets/:id/visibility`
- Beschreibung: Ändert die Sichtbarkeit eines Music Sets.
- Auth: Admin erforderlich.
- Request Body:
  ```json
  { "visibility": "private|public" }
  ```
- Response (200 OK):
  ```json
  { "message": "visibility updated successfully", "visibility": "public" }
  ```

#### `DELETE /api/v1/admin/music-sets/:id`
- Beschreibung: Löscht ein Music Set inkl. aller Tracks und S3-Objekte.
- Auth: Admin erforderlich.
- Response (200 OK):
  ```json
  { "message": "music set deleted successfully", "id": "uuid" }
  ```

#### `POST /api/v1/admin/music-sets/:id/tracks`
- Beschreibung: Lädt einen Track in ein Music Set hoch.
- Auth: Admin erforderlich.
- Content-Type: `multipart/form-data`
- Felder:
  - `file` (required): FLAC-Audiodatei
  - `title` (optional): Track-Titel
  - `artist` (optional): Künstlername
- Response (201 Created):
  ```json
  {
    "id": "uuid",
    "music_set_id": "uuid",
    "title": "string",
    "artist": "string",
    "track_order": 1,
    "created_at": "time.Time"
  }
  ```
- Hinweis: Rate Limiting - max. 30 Uploads pro Tag pro Admin.

---

### Track-Management (Admin)

#### `PUT /api/v1/admin/tracks/:id`
- Beschreibung: Aktualisiert Titel und Künstler eines Tracks.
- Auth: Admin erforderlich.
- Request Body:
  ```json
  { "title": "string", "artist": "string" }
  ```
- Response (200 OK):
  ```json
  { "message": "track updated successfully" }
  ```

#### `DELETE /api/v1/admin/tracks/:id`
- Beschreibung: Löscht einen Track inkl. S3-Objekt.
- Auth: Admin erforderlich.
- Response (200 OK):
  ```json
  { "message": "track deleted successfully" }
  ```

---

### Invite QR-Codes

#### `GET /api/v1/admin/invites/:id/qr.pdf`
- Beschreibung: Generiert eine druckfähige PDF mit QR-Code für den Invite-Link `FRONTEND_URL/register?invite=<code>` und markiert den Invite als erstellt (`qr_generated=true`).
- Auth: Admin erforderlich.
- Response: `application/pdf` (Download)

#### `GET /api/v1/admin/invites/export.csv`
- Beschreibung: Exportiert alle noch nicht exportierten Einladungscodes als CSV-Tabelle für den Druckdienstleister.
- Auth: Admin erforderlich.
- Query-Parameter:
  - `limit` (optional): Max. Anzahl der Datensätze in diesem Export.
  - `group` (optional): Filter nach Gruppe (`bubble`|`guests`|`plus`).
- CSV-Spalten:
  - `ID` (Datenbank-ID des Codes)
  - `QR-Link` (kompletter Link: `FRONTEND_URL/register?invite=<code>`)
- Verhalten:
  - Es werden nur Codes exportiert, deren Feld `exported_at` noch leer ist.
  - Nach erfolgreichem Export werden alle enthaltenen Codes in der Datenbank mit `exported_at=NOW()` markiert und erscheinen bei zukünftigen Exporten nicht mehr.
- Response (200 OK):
  - `text/csv` als Datei-Download. Falls keine Daten vorliegen: `{ "status": "no_invites_to_export" }`.

#### `GET /api/v1/admin/invites/export_bubble.csv`
- Beschreibung: Exportiert nur nicht exportierte Einladungscodes der Gruppe "bubble" als CSV.
- Auth: Admin erforderlich.
- CSV-Spalten: `Public-ID`, `QR-Link`
- Response (200 OK): `text/csv` als Datei-Download oder `{ "status": "no_invites_to_export" }`.

#### `GET /api/v1/admin/invites/export_guests.csv`
- Beschreibung: Exportiert nur nicht exportierte Einladungscodes der Gruppe "guests" als CSV.
- Auth: Admin erforderlich.
- CSV-Spalten: `Public-ID` (leer), `QR-Link`
- Response (200 OK): `text/csv` als Datei-Download oder `{ "status": "no_invites_to_export" }`.

#### `GET /api/v1/admin/invites/export_plus.csv`
- Beschreibung: Exportiert nur nicht exportierte Einladungscodes der Gruppe "plus" als CSV.
- Auth: Admin erforderlich.
- CSV-Spalten: `Public-ID`, `QR-Link`
- Response (200 OK): `text/csv` als Datei-Download oder `{ "status": "no_invites_to_export" }`.

InviteCode Felder (Erweiterung):
- `qr_generated` (bool): true, sobald die PDF einmal erzeugt/heruntergeladen wurde.
- `exported_at` (timestamp|null): Zeitpunkt des CSV-Exports für den Druck.
- `group` (string): Kategorie des Codes, entweder `bubble`, `guests` oder `plus`.
  - Codeschema:
    - `guests` → zufällige UUID, PublicID NULL
    - `bubble` → fortlaufend nummeriert `1..1000` als PublicID
    - `plus` → zufällige PublicID mit Format `P` + 4 alphanumerische Zeichen (z.B. `PA12`, `P3X9`)

---

### Backups

- Tägliche Datenbank-Backups in separaten S3-Account/Bucket (Backup-S3) vorgesehen.
- Skript: `backup/backup_db.sh` (nutzt `pg_dump`, Komprimierung und Upload via S3 API)
- Systemd-Beispiele: `backup/README.md` (Timer und Service).
- Retention: 90 Tage per S3 Lifecycle Policy (Prefix `db/`).

#### Admin Backup-Management

##### `GET /api/v1/admin/backups`
- **Beschreibung:** Ruft eine paginierte Liste aller Backups ab.
- **Auth:** Admin erforderlich.
- **Query-Parameter:**
  - `page` (optional, default: `1`): Seitennummer
  - `limit` (optional, default: `50`): Anzahl pro Seite
- **Response (200 OK):**
  ```json
  {
    "backups": [
      {
        "id": "uuid",
        "filename": "synesthesie_2025-10-03T12-00-00Z.sql.gz",
        "s3_key": "db/synesthesie/2025-10-03T12-00-00Z.sql.gz",
        "size_bytes": 1234567,
        "status": "completed", // completed, failed, in_progress
        "type": "automatic", // automatic, manual
        "started_at": "time.Time",
        "completed_at": "time.Time",
        "error_message": "string (nur bei failed)",
        "created_at": "time.Time"
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 50,
      "total": 120
    }
  }
  ```

##### `GET /api/v1/admin/backups/stats`
- **Beschreibung:** Ruft Statistiken über alle Backups ab.
- **Auth:** Admin erforderlich.
- **Response (200 OK):**
  ```json
  {
    "total_backups": 120,
    "completed_backups": 118,
    "failed_backups": 2,
    "total_size_bytes": 5678901234,
    "latest_backup": "time.Time"
  }
  ```

##### `POST /api/v1/admin/backups/sync`
- **Beschreibung:** Synchronisiert Backup-Einträge aus dem S3-Bucket in die Datenbank. Nützlich, um externe Backups (z.B. vom Cron-Job) sichtbar zu machen.
- **Auth:** Admin erforderlich.
- **Response (200 OK):**
  ```json
  {
    "message": "Backups synchronized successfully",
    "synced": 5
  }
  ```

##### ~~`DELETE /api/v1/admin/backups/:id`~~ ❌ DEAKTIVIERT
**Aus Sicherheitsgründen können Backups NICHT über die API gelöscht werden!**

Backups sind Disaster Recovery und sollten nur über:
- S3 Lifecycle Policies (automatisch nach 90 Tagen)
- Direkten S3-Zugriff (wenn unbedingt nötig)

gelöscht werden.

---

### Relevante Umgebungsvariablen (Erweiterung)

- **Payment Provider:**
  - Stripe (Standard):
    - `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`
    - `STRIPE_SUCCESS_URL`, `STRIPE_CANCEL_URL`
  - PayPal (Optional):
    - `PAYPAL_ENABLED` (true/false) – PayPal aktivieren
    - `PAYPAL_MODE` (sandbox/live) – Umgebung
    - `PAYPAL_CLIENT_ID`, `PAYPAL_SECRET` – Credentials
    - `PAYPAL_WEBHOOK_ID` – Webhook ID aus PayPal Dashboard
    - `PAYPAL_SUCCESS_URL`, `PAYPAL_CANCEL_URL`

- Media S3 (getrennter Account):
  - `MEDIA_S3_ENDPOINT`, `MEDIA_S3_REGION`, `MEDIA_S3_ACCESS_KEY_ID`, `MEDIA_S3_SECRET_ACCESS_KEY`, `MEDIA_S3_USE_PATH_STYLE`
  - `MEDIA_IMAGES_BUCKET`, `MEDIA_AUDIO_BUCKET`
- Backup S3 (separater Account):
  - `BACKUP_S3_ENDPOINT`, `BACKUP_S3_REGION`, `BACKUP_S3_ACCESS_KEY_ID`, `BACKUP_S3_SECRET_ACCESS_KEY`, `BACKUP_S3_USE_PATH_STYLE`
  - `BACKUP_BUCKET`
- Lokal/Cache/Sync:
  - `LOCAL_ASSETS_PATH` (Standard `/data/assets`)
  - `MEDIA_SYNC_ON_START` (true/false) – fehlende Bilder bei Start synchronisieren
  - `WEBP_CONVERSION_ENABLED` (true/false, Standard: true) – Automatische WebP-Konvertierung
  - `MEDIA_CACHE_AUDIO` (true/false) – Audio lokal cachen
  - `AUDIO_CACHE_PATH` (Standard `/data/assets_cache/audio`)
- URL TTL:
  - `PRESIGNED_URL_TTL_MINUTES` (Standard: 15) – Gültigkeit für Bild-URLs
  - `AUDIO_URL_TTL_MINUTES` (Standard: 120) – Gültigkeit für Audio-URLs (2h für lange Sets)

### **Health Check**

#### `GET /health`
- **Beschreibung:** Überprüft den Systemstatus des Backends.
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "status": "healthy"
  }
  ```

---

### **Öffentliche Endpunkte (`/public`)**

#### `GET /public/events`
- **Beschreibung:** Ruft eine paginierte Liste der bevorstehenden und aktiven Events ab.
- **Query-Parameter:**
    - `page` (optional, default: `1`): Die abzurufende Seite.
    - `limit` (optional, default: `10`): Die Anzahl der Events pro Seite.
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "events": [
      {
        "id": "uuid",
        "name": "string",
        "description": "string",
        "date_from": "time.Time",
        "date_to": "time.Time",
        "time_from": "string (Format: HH:MM)",
        "time_to": "string (Format: HH:MM)",
        "price": "float64", // gruppenabhängig: guests=100.0, bubble=35.0, plus=50.0
        "max_participants": "int",
        "available_spots": "int"
      }
    ],
    "pagination": {
      "page": "int",
      "limit": "int",
      "total": "int"
    }
  }
  ```

#### `GET /public/invite/:code`
- **Beschreibung:** Überprüft die Gültigkeit und den Status eines Einladungscodes ohne ihn zu verbrauchen.
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "valid": "boolean",
    "code": "string",
    "status": "string", // "new", "assigned", "viewed", "registered", "inactive"
    "group": "string",  // "bubble" | "guests" | "plus"
    "message": "string"
  }
  ```

#### `POST /public/invite/:code/view`
- **Beschreibung:** Markiert einen Einladungscode als "angesehen" beim ersten Aufruf. Dies ist ein einmaliger Vorgang pro Code.
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "valid": true,
    "code": "string",
    "status": "viewed",
    "group": "string",  // "bubble" | "guests" | "plus"
    "message": "Invite code has been marked as viewed. You can now proceed with registration."
  }
  ```
- **Response Body (400 Bad Request) - Code bereits angesehen oder ungültig:**
  ```json
  {
    "error": "invite code has already been viewed or is no longer available"
  }
  ```

---

### **Authentifizierungs-Endpunkte (`/auth`)**

#### `POST /auth/register`
- **Beschreibung:** Registriert einen neuen Benutzer. Ein gültiger Einladungscode ist erforderlich, der zuvor über `/public/invite/:code/view` als "angesehen" markiert wurde.
- **Request Body:**
  ```json
  {
    "invite_code": "string (muss Status 'viewed' haben)",
    "username": "string (min: 3, max: 30)",
    "email": "string (gültiges E-Mail-Format)",
    "password": "string (min: 12, komplex)",
    "name": "string",
    "mobile": "string (optional)",
    "drink1": "string (optional)",
    "drink2": "string (optional)",
    "drink3": "string (optional)"
  }
  ```
- **Response Body (400 Bad Request) - Code nicht angesehen:**
  ```json
  {
    "error": "invite code must be viewed first before registration"
  }
  ```
- **Response Body (201 Created):**
  ```json
  {
    "message": "Registration successful",
    "user": {
      "id": "uuid",
      "username": "string",
      "email": "string",
      "name": "string",
      "group": "bubble" | "guests" | "plus"
    }
  }
  ```

#### `POST /auth/login`
- **Beschreibung:** Meldet einen Benutzer an und liefert Access- und Refresh-Tokens.
- **Hinweis:** Das Feld `username` akzeptiert sowohl den Benutzernamen als auch die E-Mail-Adresse.
- **Request Body:**
  ```json
  {
    "username": "string (Username oder E-Mail-Adresse)",
    "password": "string"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "access_token": "string",
    "refresh_token": "string",
    "user": {
      "id": "uuid",
      "username": "string",
      "email": "string",
      "name": "string",
      "is_admin": "boolean",
      "group": "bubble" | "guests" | "plus"
    }
  }
  ```

#### `POST /auth/refresh`
- **Beschreibung:** Erneuert einen abgelaufenen Access-Token mithilfe eines Refresh-Tokens.
- **Request Body:**
  ```json
  {
    "refresh_token": "string"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "access_token": "string"
  }
  ```

#### `POST /auth/logout`
- **Beschreibung:** Meldet den aktuell authentifizierten Benutzer ab.
- **Benötigt Authentifizierung.**
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "message": "Logout successful"
  }
  ```

---

### **Benutzer-Endpunkte (`/user`)**
**Alle Endpunkte in diesem Abschnitt erfordern eine Authentifizierung.**

#### `GET /user/profile`
- **Beschreibung:** Ruft die Profildaten des aktuellen Benutzers ab.
- **Response Body (200 OK):**
  ```json
  {
    "id": "uuid",
    "username": "string",
    "email": "string",
    "name": "string",
    "mobile": "string",
    "drink1": "string",
    "drink2": "string",
    "drink3": "string",
    "group": "bubble" | "guests" | "plus",
    "created_at": "time.Time"
  }
  ```

#### `PUT /user/profile`
- **Beschreibung:** Aktualisiert die Profildaten des aktuellen Benutzers.
- **Verhalten bei `mobile`:**
  - Wenn `SMS_VERIFICATION_ENABLED=true`: Die neue Nummer wird gespeichert, `mobile_verified=false` gesetzt und ein Verifizierungscode per SMS versendet. Response enthält Hinweis auf Verifizierung.
  - Wenn `SMS_VERIFICATION_ENABLED=false`: Die neue Nummer wird direkt übernommen, `mobile_verified` bleibt/ist true.
- **Request Body (alle Felder optional):**
  ```json
  {
    "mobile": "string",
    "drink1": "string",
    "drink2": "string",
    "drink3": "string"
  }
  ```
- **Responses:**
  - 200 OK (Verifizierung aktiv): `{ "message": "Profile updated. Please verify your new mobile number." }`
  - 200 OK (Verifizierung aus): `{ "message": "Profile updated successfully" }`

#### `GET /user/settings/pickup-price`
- **Beschreibung:** Liefert den aktuellen Preis für den Abholservice für Benutzeransichten.
- **Response Body (200 OK):**
  ```json
  {
    "price": "float64"
  }
  ```

#### `GET /user/events`
- **Beschreibung:** Ruft bevorstehende Events ab und zeigt an, ob der Benutzer bereits ein Ticket hat.
- **Query-Parameter:** `page`, `limit`.
- **Response Body (200 OK):**
  ```json
  {
    "events": [
      {
        "id": "uuid",
        "name": "string",
        "description": "string",
        "date_from": "time.Time",
        "date_to": "time.Time",
        "price": "float64", // gruppenabhängig: guests=100.0, bubble=35.0, plus=50.0
        "available_spots": "int",
        "has_ticket": "boolean",
        "ticket": { // Nur vorhanden, wenn has_ticket true ist
          "id": "uuid",
          "status": "string",
          "includes_pickup": "boolean"
        }
      }
    ],
    "pagination": { "page": "int", "limit": "int", "total": "int" }
  }
  ```

#### `GET /user/tickets`
- **Beschreibung:** Ruft eine Liste aller Tickets ab, die der aktuelle Benutzer gebucht hat.
- **Response Body (200 OK):**
  ```json
  {
    "tickets": [
      {
        "id": "uuid",
        "status": "string", // pending, paid, cancelled, refunded
        "total_amount": "float64",
        "created_at": "time.Time",
        "event": {
          "id": "uuid",
          "name": "string",
          "date_from": "time.Time"
        }
        // ... weitere Ticket-Details
      }
    ]
  }
  ```

#### `POST /user/tickets`
- **Beschreibung:** Startet den Buchungsprozess für ein Event-Ticket mit Stripe oder PayPal.
- **Request Body:**
  ```json
  {
    "event_id": "string (uuid)",
    "includes_pickup": "boolean",
    "pickup_address": "string (erforderlich, wenn includes_pickup true ist)",
    "payment_provider": "string (optional: 'stripe' oder 'paypal', default: 'stripe')"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "ticket_id": "uuid",
    "checkout_url": "string (Stripe oder PayPal URL)",
    "payment_provider": "stripe" | "paypal"
  }
  ```
- **Hinweis:** PayPal muss serverseitig aktiviert sein (`PAYPAL_ENABLED=true`)

#### `POST /user/tickets/:id/retry-checkout`
- **Beschreibung:** Generiert eine neue Checkout-URL für ein pending Ticket (z.B. wenn User das Zahlungsfenster geschlossen hat).
- **Benötigt Authentifizierung.**
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "checkout_url": "https://checkout.stripe.com/... oder https://paypal.com/...",
    "payment_provider": "stripe" | "paypal",
    "message": "Checkout URL generated successfully"
  }
  ```
- **Response Body (400 Bad Request):**
  ```json
  {
    "error": "ticket is not pending (status: paid)"
  }
  ```
  oder
  ```json
  {
    "error": "ticket not found"
  }
  ```
- **Hinweis:** Funktioniert nur für Tickets mit Status `pending`. Der gleiche Payment Provider wie beim ursprünglichen Checkout wird verwendet.

#### `POST /user/tickets/:id/confirm-payment`
- **Beschreibung:** Proaktive Zahlungsbestätigung wenn User von Payment-Provider zurückkehrt. Prüft SOFORT bei Stripe/PayPal den Payment-Status.
- **Benötigt Authentifizierung.**
- **Request Body:**
  ```json
  {
    "token": "string (PayPal Order Token, optional)",
    "payer_id": "string (PayPal Payer ID, optional)",
    "session_id": "string (Stripe Session ID, optional)"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "status": "paid",
    "message": "Payment confirmed successfully"
  }
  ```
- **Response Body (202 Accepted):**
  ```json
  {
    "status": "pending",
    "message": "Payment verification in progress"
  }
  ```
- **Hinweis:** Ermöglicht sofortige Bestätigung ohne auf Webhooks zu warten. Reaktiviert auch `pending_cancellation` Tickets (Grace Period).

#### `DELETE /user/tickets/:id`
- **Beschreibung:** Storniert ein gebuchtes Ticket.
- **Benötigt Authentifizierung.**
- **Verhalten:**
  - **Pending Tickets:** Werden auf `pending_cancellation` gesetzt mit **5 Minuten Grace Period**
    - ⏱️ **Grace Period:** Wenn PayPal/Stripe-Zahlung innerhalb von 5 Minuten abgeschlossen wird, wird das Ticket automatisch auf `paid` gesetzt ✅
    - ⏱️ **Nach 5 Minuten:** Ticket wird endgültig auf `cancelled` gesetzt
    - 🛡️ **Schutz:** Verhindert Race Condition (User cancelt → Zahlung kommt durch → User hat bezahlt aber kein Ticket)
  - **Paid Tickets:** Werden storniert, ggf. mit Refund (abhängig von Cancellation Policy)
- **Response Body (200 OK):**
  ```json
  {
    "message": "Ticket cancelled successfully"
  }
  ```
- **Hinweis:** Die Grace Period schützt vor dem Szenario, dass ein User während der Zahlung das Ticket abbricht, die Zahlung aber trotzdem durchgeht.

---

### **Admin-Endpunkte (`/admin`)**
**Alle Endpunkte in diesem Abschnitt erfordern Admin-Rechte.**

#### Event-Management

##### `GET /admin/events`
- **Beschreibung:** Ruft alle Events ab (auch inaktive).
- **Query-Parameter:** `page`, `limit`, `include_inactive` (boolean).
- **Response Body (200 OK):**
  ```json
  {
    "events": [
      {
        "id": "uuid",
        "name": "string",
        "description": "string",
        "date_from": "time.Time",
        "date_to": "time.Time",
        "time_from": "string (HH:MM)",
        "time_to": "string (HH:MM)",
        "max_participants": "int",
        "price": "float64",
        "is_active": "boolean",
        "available_spots": "int",
        "created_at": "time.Time",
        "updated_at": "time.Time"
      }
    ],
    "pagination": { "page": "int", "limit": "int", "total": "int" }
  }
  ```

##### `POST /admin/events`
- **Beschreibung:** Erstellt ein neues Event.
- **Request Body:**
  ```json
  {
    "name": "string",
    "description": "string",
    "date_from": "time.Time",
    "date_to": "time.Time",
    "time_from": "string (HH:MM)",
    "time_to": "string (HH:MM)",
    "max_participants": "int",
    "allowed_group": "string (optional: 'all'|'guests'|'bubble'|'plus', default: 'all')",
    "guests_price": "float64 (optional, default: 100.0)",
    "bubble_price": "float64 (optional, default: 35.0)",
    "plus_price": "float64 (optional, default: 50.0)"
  }
  ```
- **Response Body (201 Created):**
  ```json
  {
    "message": "Event created successfully",
    "event": {
      // Vollständiges Event-Objekt, siehe models.Event
    }
  }
  ```

##### `POST /admin/users/announce`
- **Beschreibung:** Sendet eine generische Ankündigungs-Email an **alle aktiven Benutzer** der Plattform.
- **Request Body:**
  ```json
  {
    "subject": "string (optional: Betreff der Email, Standard: 'Neuigkeiten von Synesthesie')",
    "message": "string (required: HTML-Nachricht, die im Email-Template angezeigt wird)"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "message": "Announcement sent to X users",
    "sent": 10,
    "failed": 0,
    "total_users": 10
  }
  ```
- **Hinweis:**
  - Nutzt das Template `generic_announcement.html`.
  - Sendet E-Mails mit Rate-Limiting (10 E-Mails/Sekunde).
  - Ignoriert inaktive Benutzer.

##### `PUT /admin/events/:id`
- **Beschreibung:** Aktualisiert ein bestehendes Event.
- **Request Body (alle Felder optional):**
  ```json
  {
    "name": "string",
    "description": "string",
    "date_from": "time.Time",
    "date_to": "time.Time",
    "time_from": "string (HH:MM)",
    "time_to": "string (HH:MM)",
    "max_participants": "int",
    "allowed_group": "string ('all'|'guests'|'bubble'|'plus')",
    "guests_price": "float64",
    "bubble_price": "float64",
    "plus_price": "float64"
  }
  ```
- **Response Body (200 OK):** `{"message": "Event updated successfully"}`

##### `DELETE /admin/events/:id`
- **Beschreibung:** Löscht ein Event.
- **Response Body (200 OK):** `{"message": "Event deleted successfully"}`

##### `POST /admin/events/:id/deactivate`
- **Beschreibung:** Deaktiviert ein Event.
- **Response Body (200 OK):** `{"message": "Event deactivated successfully"}`

##### `POST /admin/events/:id/refund`
- **Beschreibung:** Löst die Rückerstattung für alle Tickets eines Events aus.
- **Response Body (200 OK):** `{"message": "All tickets refunded successfully"}`

##### `POST /admin/events/:id/announce`
- **Beschreibung:** Sendet eine Ankündigungs-Email an alle Teilnehmer eines Events (nur bezahlte Tickets).
- **Request Body:**
  ```json
  {
    "subject": "string (optional: Betreff der Email, Standard: 'Wichtige Informationen zu [EVENTNAME]')",
    "message": "string (required: HTML-Nachricht, die im Email-Template angezeigt wird)"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "message": "Announcement sent to X participants",
    "sent": 10,
    "failed": 0,
    "total_participants": 10
  }
  ```
- **Hinweis:** Die Nachricht wird im `event_announcement.html` Template gerendert und enthält automatisch Event-Details (Name, Datum, Uhrzeit).

##### `GET /admin/events/:id`
- **Beschreibung:** Ruft detaillierte Informationen zu einem Event ab, inklusive Teilnehmerliste gruppiert nach Benutzergruppen.
- **Response Body (200 OK):**
  ```json
  {
    "event": {
      "id": "uuid",
      "name": "string",
      "description": "string",
      "date_from": "time.Time",
      "date_to": "time.Time",
      "time_from": "string (HH:MM)",
      "time_to": "string (HH:MM)",
      "max_participants": "int",
      "guests_price": "float64",
      "bubble_price": "float64",
      "plus_price": "float64",
      "allowed_group": "string",
      "is_active": "boolean",
      "available_spots": "int",
      "total_participants": "int",
      "turnover": "float64",
      "created_at": "time.Time",
      "updated_at": "time.Time"
    },
    "participants": {
      "guests": [
        {
          "name": "string",
          "email": "string",
          "drink1": "string",
          "drink2": "string",
          "drink3": "string",
          "group": "guests"
        }
      ],
      "bubble": [...],
      "plus": [...]
    }
  }
  ```

##### `GET /admin/events/:id/participants.csv`
- **Beschreibung:** Exportiert die Teilnehmerliste eines Events als CSV-Datei.
- **CSV-Spalten:** `Gruppe`, `Name`, `Email`, `Lieblingsgetraenk 1`, `Lieblingsgetraenk 2`, `Lieblingsgetraenk 3`
- **Sortierung:** Gruppiert nach Benutzergruppe (bubble, guests, plus), innerhalb der Gruppe alphabetisch nach Name sortiert.
- **Dateiname:** `Teilnehmer_DD-MM-YYYY_EVENTNAME.csv`
- **Response:**
  - 200 OK: `text/csv` als Datei-Download (auch wenn keine Teilnehmer, wird eine leere CSV mit Header zurückgegeben).

##### `GET /admin/events/:id/drinks.xlsx`
- **Beschreibung:** Exportiert eine Statistik der Lieblingsgetränke aller Event-Teilnehmer als Excel-kompatible CSV.
- **CSV-Spalten:** `Getränk`, `Anzahl`, `Gewählt von` (kommaseparierte Liste der Namen)
- **Dateiname:** `Getränke_DD-MM-YYYY_EVENTNAME.csv`
- **Response:**
  - 200 OK: `text/csv` als Datei-Download mit Häufigkeitsauswertung und Teilnehmerliste.
  - 200 OK: `{ "status": "no_participants" }`, wenn keine bezahlten Tickets vorhanden sind.

---
#### Einladungs-Management

##### `GET /admin/invites`
- **Beschreibung:** Ruft alle Einladungscodes ab.
- **Query-Parameter:**
  - `page` (optional, default: 1): Seitennummer
  - `limit` (optional, default: 20): Anzahl pro Seite
  - `include_used` (optional, boolean): Zeigt auch bereits verwendete Codes
  - `group` (optional): Filtert nach Gruppe (`bubble`, `guests`, `plus`)
  - `status` (optional): Filtert nach Status (`new`, `assigned`, `viewed`, `registered`, `inactive`)
- **Response Body (200 OK):**
  ```json
  {
    "invites": [
      {
        "id": "uuid",
        "public_id": "string",
        "code": "string",
        "status": "string", // "new", "assigned", "viewed", "registered", "inactive"
        "group": "string",  // "bubble" | "guests" | "plus"
        "viewed_at": "time.Time",
        "registered_at": "time.Time",
        "created_at": "time.Time",
        "registered_by": { // Nur wenn registriert
          "id": "uuid",
          "username": "string",
          "name": "string"
        }
      }
    ],
    "pagination": { "page": "int", "limit": "int", "total": "int" }
  }
  ```

##### `GET /admin/invites/stats`
- **Beschreibung:** Ruft Statistiken über Einladungscodes ab, inklusive Liste aller registrierten User.
- **Response Body (200 OK):**
  ```json
  {
    "total": 1000,
    "new": 450,
    "assigned": 150,
    "viewed": 250,
    "used": 250,
    "registered": 280,
    "inactive": 20,
    "registered_users": [
      {
        "id": "uuid",
        "username": "string",
        "name": "string",
        "email": "string",
        "group": "bubble" | "guests" | "plus",
        "invite_id": "uuid",
        "public_id": "string",
        "created_at": "time.Time"
      }
    ]
  }
  ```

##### `POST /admin/invites`
- **Beschreibung:** Erstellt einen oder mehrere Einladungscodes.
- **Request Body:**
  ```json
  {
    "count": "int (optional, default: 1)",
    "group": "string (optional: 'bubble'|'guests'|'plus'; default: 'guests')"
  }
  ```
- **Response Body (201 Created):**
  ```json
  // Für count = 1
  {
    "message": "Invite code created successfully",
    "invite": { "id": "uuid", "code": "string", "group": "string" }
  }
  // Für count > 1
  {
    "message": "Invite codes created successfully",
    "invites": [ { "id": "uuid", "code": "string", "group": "string" } ]
  }
  ```

##### `DELETE /admin/invites/:id`
- **Beschreibung:** Deaktiviert einen Einladungscode.
- **Response Body (200 OK):** `{"message": "Invite deactivated successfully"}`

##### `POST /admin/invites/:id/assign`
- **Beschreibung:** Markiert einen Einladungscode als "assigned" (vergeben). Nur möglich, wenn Status "new" ist.
- **Request Body:** Keiner.
- **Response Body (200 OK):**
  ```json
  {
    "message": "Invite marked as assigned",
    "status": "assigned",
    "code": "string"
  }
  ```
- **Response Body (400 Bad Request):**
  ```json
  {
    "error": "invite code can only be marked as assigned when status is 'new'"
  }
  ```

---
#### Benutzer-Management

##### `GET /admin/users`
- **Beschreibung:** Ruft alle Benutzer mit Suchfunktion ab.
- **Query-Parameter:** `page`, `limit`, `search` (string).
- **Response Body (200 OK):**
  ```json
  {
    "users": [
      {
        "id": "uuid",
        "username": "string",
        "email": "string",
        "name": "string",
        "group": "string",
        "is_active": "boolean",
        "created_at": "time.Time"
        // ... weitere Benutzer-Details
      }
    ],
    "pagination": { "page": "int", "limit": "int", "total": "int" }
  }
  ```

##### `GET /admin/users/:id`
- **Beschreibung:** Ruft die Details eines Benutzers inklusive Ticket-Historie ab.
- **Response Body (200 OK):**
  ```json
  {
    "user": {
      "id": "uuid",
      "username": "string",
      "email": "string",
      "name": "string",
      "mobile": "string",
      "drink1": "string",
      "drink2": "string",
      "drink3": "string",
      "group": "bubble" | "guests" | "plus",
      "is_active": "boolean",
      "registered_with_code": "string",
      "created_at": "time.Time"
    },
    "ticket_history": [
      {
        "id": "uuid",
        "event_name": "string",
        "event_date": "time.Time",
        "status": "string",
        "total_amount": "float64",
        "includes_pickup": "boolean",
        "created_at": "time.Time"
      }
    ]
  }
  ```

##### `PUT /admin/users/:id/password`
##### `PUT /admin/users/:id/active`
- **Beschreibung:** Setzt den Aktiv-Status eines Benutzers.
- **Request Body:**
  ```json
  { "is_active": true }
  ```
- **Response Body (200 OK):** `{"message": "User active status updated", "is_active": true}`

##### `PUT /admin/users/:id/group`
- **Beschreibung:** Weist einem Benutzer eine Gruppe zu oder ändert sie.
- **Request Body:**
  ```json
  { "group": "bubble" | "guests" | "plus" }
  ```
- **Response Body (200 OK):** `{"message": "User group updated successfully", "group": "bubble"}`

- **Beschreibung:** Setzt das Passwort eines Benutzers zurück.
- **Response Body (200 OK):**
  ```json
  {
    "message": "Password reset successfully",
    "new_password": "string" // Im produktiven Einsatz nur per E-Mail senden!
  }
  ```

---
#### Ticket-Management

##### `POST /admin/tickets/:id/cancel`
- **Beschreibung:** Storniert ein Ticket als Administrator (ohne Prüfung des Ticket-Besitzes). Identisch zur User-Stornierung, jedoch wird dem Benutzer in der Email mitgeteilt, dass die Stornierung durch einen Administrator erfolgte.
- **Sicherheit:**
  - ⚠️ **Rate Limiting:** Max. 10 Stornierungen pro 5 Minuten
  - 🔒 **Audit Logging:** Jede Aktion wird protokolliert
  - 📧 **Alert-System:** Bei >5 Stornierungen in 5 Min wird Admin-Email gesendet
  - 🚫 **Auto-Block:** Bei >5 Stornierungen in 5 Min wird Account für **1 Stunde blockiert**
- **Query-Parameter:**
  - `mode` (optional, default: `auto`): Stornierungsmodus
    - `auto`: Automatische Refund-Prüfung basierend auf Policy (Standard)
    - `refund`: Explizit mit Refund (schlägt fehl, wenn nicht berechtigt)
    - `no_refund`: Stornierung ohne Refund
- **Request Body:** Keiner.
- **Verhalten:**
  - Bei `pending` Status: Ticket wird gelöscht
  - Bei `paid` Status:
    - Prüft Refund-Berechtigung (Tage bis Event)
    - Führt ggf. Stripe-Refund durch
    - Setzt Status auf `cancelled`
    - Sendet Bestätigungs-Email mit Hinweis "Storniert durch Administrator"
    - Loggt Aktion im Audit Log
- **Response Body (200 OK):**
  ```json
  {
    "message": "Ticket cancelled successfully by admin"
  }
  ```
- **Response Body (429 Too Many Requests):**
  ```json
  {
    "error": "rate_limit_exceeded",
    "message": "Too many actions in a short time. Please wait a few minutes.",
    "retry_after_minutes": 5,
    "warning": "Further attempts will result in a 1-hour block."
  }
  ```
- **Response Body (403 Forbidden - Nach >5 Stornierungen):**
  ```json
  {
    "error": "admin_temporarily_blocked",
    "message": "Too many actions detected. Your account has been temporarily blocked for 1 hour. If this was not you, please contact the system administrator immediately.",
    "blocked_for_minutes": 60
  }
  ```
- **Response Body (400 Bad Request):**
  ```json
  {
    "error": "refund_not_eligible"
  }
  ```
  oder
  ```json
  {
    "error": "ticket not found"
  }
  ```

---
#### Audit Log (Admin-Sicherheit)

##### `GET /admin/audit/logs`
- **Beschreibung:** Ruft Audit-Log-Einträge ab (alle Admin-Aktionen werden protokolliert).
- **Query-Parameter:**
  - `page` (optional, default: 1): Seitennummer
  - `limit` (optional, default: 50): Anzahl pro Seite
  - `action` (optional): Filter nach Aktionstyp (z.B. `cancel_ticket`)
  - `admin_id` (optional, UUID): Filter nach Admin-Benutzer
- **Response Body (200 OK):**
  ```json
  {
    "logs": [
      {
        "id": "uuid",
        "admin_id": "uuid",
        "admin": {
          "id": "uuid",
          "username": "string",
          "name": "string",
          "email": "string"
        },
        "action": "cancel_ticket",
        "target_type": "ticket",
        "target_id": "uuid",
        "details": "{\"mode\":\"auto\",\"ticket_id\":\"...\"}",
        "ip_address": "192.168.1.1",
        "user_agent": "Mozilla/5.0...",
        "created_at": "time.Time"
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 50,
      "total": 150
    }
  }
  ```

##### `GET /admin/audit/stats`
- **Beschreibung:** Ruft Statistiken über Admin-Aktionen ab.
- **Response Body (200 OK):**
  ```json
  {
    "total_actions": 1500,
    "actions_by_type": [
      {"action": "cancel_ticket", "count": 450},
      {"action": "create_invite", "count": 300}
    ],
    "most_active_admins_30d": [
      {"admin_id": "uuid", "count": 120}
    ],
    "actions_last_24h": 45
  }
  ```

---

### **Preis-Management (Admin)**

##### `GET /admin/settings/pickup-price`
- **Beschreibung:** Ruft den Preis für den Abholservice ab.
- **Response Body (200 OK):**
  ```json
  {
    "price": "float64"
  }
  ```

##### `PUT /admin/settings/pickup-price`
- **Beschreibung:** Aktualisiert den Preis für den Abholservice.
- **Request Body:**
  ```json
  {
    "price": "float64"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "message": "Pickup service price updated successfully",
    "price": "float64"
  }
  ```

---

### **Payment Webhooks**

#### `POST /stripe/webhook`
- **Beschreibung:** Empfängt und verarbeitet Ereignisse von Stripe. Dieser Endpunkt ist entscheidend für die Aktualisierung des Ticket-Status nach einer Zahlung. Er wird von Stripe aufgerufen und ist nicht für die manuelle Verwendung vorgesehen.
- **Verarbeitete Events:**
  - `checkout.session.completed`: Wird nach einer erfolgreichen Zahlung ausgelöst. Aktualisiert den Ticketstatus von `pending` auf `paid` und speichert die Payment Intent ID.
  - `payment_intent.payment_failed`: Wird protokolliert, wenn eine Zahlung fehlschlägt.
- **Request Body:** `stripe.Event` Objekt (wird von Stripe gesendet).
- **Response Body (200 OK):**
  ```json
  {
    "status": "success",
    "message": "Payment confirmed"
  }
  ```
  Oder im Falle eines nicht behandelten Events:
  ```json
  {
    "status": "success",
    "message": "Unhandled event type"
  }
  ```

#### `POST /paypal/webhook`
- **Beschreibung:** Empfängt und verarbeitet Ereignisse von PayPal. Dieser Endpunkt ist entscheidend für die Aktualisierung des Ticket-Status nach einer PayPal-Zahlung.
- **Verarbeitete Events:**
  - `PAYMENT.CAPTURE.COMPLETED`: Wird nach einer erfolgreichen Zahlung ausgelöst. Aktualisiert den Ticketstatus von `pending` auf `paid` und speichert die Capture ID.
  - `CHECKOUT.ORDER.APPROVED`: Order wurde genehmigt (noch nicht captured).
  - `PAYMENT.CAPTURE.DENIED`: Zahlung wurde abgelehnt.
  - `PAYMENT.CAPTURE.REFUNDED`: Zahlung wurde erstattet.
- **Request Body:** PayPal Webhook Event (wird von PayPal gesendet).
- **Response Body (200 OK):**
  ```json
  {
    "status": "success",
    "message": "Webhook received"
  }
  ```

---

## **Einladungscode-Workflow**

Der neue Einladungscode-Workflow funktioniert wie folgt:

### **1. QR-Code scannen**
- Benutzer scannt einen QR-Code, der eine URL mit dem Einladungscode enthält (z.B. `https://app.synesthesie.de/register?invite=ABC123`)

### **2. Code als "angesehen" markieren**
- Das Frontend ruft automatisch `POST /public/invite/:code/view` auf
- Der Code wird **einmalig** von "new" auf "viewed" gesetzt
- Jeder weitere Aufruf dieses Endpunkts schlägt fehl

### **3. Registrierung**
- Benutzer kann sich nur mit Codes im Status "viewed" registrieren
- Nach erfolgreicher Registrierung wird der Code auf "registered" gesetzt

### **Status-Übersicht:**
- `new`: Frisch erstellt, noch nicht aufgerufen
- `assigned`: Vom Admin vergeben (z.B. QR-Code kopiert/heruntergeladen), aber noch nicht vom Benutzer aufgerufen
- `viewed`: Einmal vom Benutzer aufgerufen, bereit für Registrierung
- `registered`: Für Registrierung verwendet
- `inactive`: Vom Admin deaktiviert

**Wichtig:** Ein Code kann nur einmal "angesehen" werden. Schließt der Benutzer den Browser oder startet das Gerät neu, ist die Chance vertan.

### Admin – Pickup-Export

#### `GET /api/v1/admin/pickups/export.csv`
- Beschreibung: Exportiert alle Tickets mit gebuchtem Abholservice als CSV.
- Query-Parameter:
  - `event_id` (optional, UUID): Filtert auf ein Event.
  - `status` (optional, Default `paid`): `paid` oder `all` (inkl. `pending`).
- CSV-Spalten: `Name`, `Mobile`, `Pickup-Address`
- Response:
  - 200 OK `text/csv` Download.
  - 200 OK `{ "status": "no_pickups" }`, wenn keine Einträge.

### Auth – Passwort zurücksetzen

#### `POST /api/v1/auth/password/forgot`
- Beschreibung: Fordert einen Passwort-Reset per E-Mail an.
- Request:
```json
{ "email": "user@example.com" }
```
- Response (immer 200): `{ "message": "If the email exists, a reset link has been sent." }`

#### `POST /api/v1/auth/password/reset`
- Beschreibung: Setzt mit Token ein neues Passwort.
- Request:
```json
{ "token": "string", "new_password": "string" }
```
- Response 200: `{ "message": "Password reset successful" }`