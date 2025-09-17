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

### Invite QR-Codes

#### `GET /api/v1/admin/invites/:id/qr.pdf`
- Beschreibung: Generiert eine druckfähige PDF mit QR-Code für den Invite-Link `FRONTEND_URL/register?invite=<code>` und markiert den Invite als erstellt (`qr_generated=true`).
- Auth: Admin erforderlich.
- Response: `application/pdf` (Download)

InviteCode Felder (Erweiterung):
- `qr_generated` (bool): true, sobald die PDF einmal erzeugt/heruntergeladen wurde.

---

### Backups

- Tägliche Datenbank-Backups in separaten S3-Account/Bucket (Backup-S3) vorgesehen.
- Skript: `backup/backup_db.sh` (nutzt `pg_dump`, Komprimierung und Upload via S3 API)
- Systemd-Beispiele: `backup/README.md` (Timer und Service).
- Retention: 90 Tage per S3 Lifecycle Policy (Prefix `db/`).

---

### Relevante Umgebungsvariablen (Erweiterung)

- Media S3 (getrennter Account):
  - `MEDIA_S3_ENDPOINT`, `MEDIA_S3_REGION`, `MEDIA_S3_ACCESS_KEY_ID`, `MEDIA_S3_SECRET_ACCESS_KEY`, `MEDIA_S3_USE_PATH_STYLE`
  - `MEDIA_IMAGES_BUCKET`, `MEDIA_AUDIO_BUCKET`
- Backup S3 (separater Account):
  - `BACKUP_S3_ENDPOINT`, `BACKUP_S3_REGION`, `BACKUP_S3_ACCESS_KEY_ID`, `BACKUP_S3_SECRET_ACCESS_KEY`, `BACKUP_S3_USE_PATH_STYLE`
  - `BACKUP_BUCKET`
- Lokal/Cache/Sync:
  - `LOCAL_ASSETS_PATH` (Standard `/data/assets`)
  - `MEDIA_SYNC_ON_START` (true/false) – fehlende Bilder bei Start synchronisieren
  - `MEDIA_CACHE_AUDIO` (true/false) – Audio lokal cachen
  - `AUDIO_CACHE_PATH` (Standard `/data/assets_cache/audio`)


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
        "price": "float64",
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
    "status": "string", // "new", "viewed", "registered", "inactive"
    "group": "string",  // "bubble" | "guests"
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
    "group": "string",  // "bubble" | "guests"
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
    "password": "string (min: 8, muss komplex sein)",
    "name": "string",
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
      "group": "bubble" | "guests"
    }
  }
  ```

#### `POST /auth/login`
- **Beschreibung:** Meldet einen Benutzer an und liefert Access- und Refresh-Tokens.
- **Request Body:**
  ```json
  {
    "username": "string",
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
      "group": "bubble" | "guests"
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

### Auth – SMS-Verifizierung

#### `POST /auth/register`
- Erweitert um Pflichtfeld `mobile` (E.164, z. B. `+491701234567`).
- Response enthält Hinweis, dass die Mobilnummer verifiziert werden muss.

#### `POST /auth/verify-mobile` (authentifiziert)
- Body: `{ "code": "string" }` (6-stellig)
- Verifiziert die Mobilnummer des eingeloggten Users.
- Response: `{ "message": "Mobile verified" }`

#### `POST /auth/verify-mobile/resend` (authentifiziert)
- Sendet einen neuen Code an die beim User hinterlegte Mobilnummer.
- Response: `{ "message": "Verification code sent" }`

Benutzerfelder erweitert:
- `mobile`: string
- `mobile_verified`: boolean

---

### Auth – Passwort-Reset
- `POST /api/v1/auth/password/forgot` → sendet Reset-Link (immer 200)
- `POST /api/v1/auth/password/reset` → setzt per Token ein neues Passwort

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
    "group": "bubble" | "guests",
    "created_at": "time.Time"
  }
  ```

#### `PUT /user/profile`
- **Beschreibung:** Aktualisiert Profildaten (nur `mobile`, `drink1-3`).
- **Request Body (alle Felder optional):**
  ```json
  { "mobile": "string", "drink1": "string", "drink2": "string", "drink3": "string" }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "message": "Profile updated successfully"
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
        "price": "float64", // gruppenabhängig: guests=200.0, bubble=35.0
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
- **Beschreibung:** Startet den Buchungsprozess für ein Event-Ticket.
- **Request Body:**
  ```json
  {
    "event_id": "string (uuid)",
    "includes_pickup": "boolean",
    "pickup_address": "string (erforderlich, wenn includes_pickup true ist)"
  }
  ```
- **Response Body (200 OK):**
  ```json
  {
    "ticket_id": "uuid",
    "checkout_url": "string (Stripe URL)"
  }
  ```

#### `DELETE /user/tickets/:id`
- **Beschreibung:** Storniert ein gebuchtes Ticket.
- **Response Body (200 OK):**
  ```json
  {
    "message": "Ticket cancelled successfully"
  }
  ```

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
    "price": "float64"
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
    "price": "float64"
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

---
#### Einladungs-Management

##### `GET /admin/invites`
- **Beschreibung:** Ruft alle Einladungscodes ab.
- **Query-Parameter:** `page`, `limit`, `include_used` (boolean).
- **Response Body (200 OK):**
  ```json
  {
    "invites": [
      {
        "id": "uuid",
        "code": "string",
        "status": "string", // "new", "viewed", "registered", "inactive"
        "group": "string",  // "bubble" | "guests"
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

##### `POST /admin/invites`
- **Beschreibung:** Erstellt einen oder mehrere Einladungscodes.
- **Request Body:**
  ```json
  {
    "count": "int (optional, default: 1)",
    "group": "string (optional: 'bubble'|'guests'; default: 'guests')"
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
      // Vollständiges Benutzer-Objekt
    },
    "ticket_history": [
      {
        "id": "uuid",
        "event_name": "string",
        "event_date": "time.Time",
        "status": "string",
        "total_amount": "float64",
        "created_at": "time.Time"
      }
    ]
  }
  ```

##### `PUT /admin/users/:id/password`
##### `PUT /admin/users/:id/group`
- **Beschreibung:** Weist einem Benutzer eine Gruppe zu oder ändert sie.
- **Request Body:**
  ```json
  { "group": "bubble" | "guests" }
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
#### Preis-Management

##### `GET /admin/settings/pickup-price`
- **Beschreibung:** Ruft den Preis für den Abholservice ab.
- **Response Body (200 OK):**
  ```json
  {
    "price": "float64"
  }
  ```

##### `