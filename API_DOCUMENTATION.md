# Synesthesie Backend API Dokumentation

Alle Endpunkte sind unter dem Präfix `/api/v1` erreichbar.

---

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
    "favorite_drink": "string (optional)",
    "favorite_cocktail": "string (optional)",
    "favorite_shot": "string (optional)"
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
      "name": "string"
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
      "is_admin": "boolean"
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
    "favorite_drink": "string",
    "favorite_cocktail": "string",
    "favorite_shot": "string",
    "created_at": "time.Time"
  }
  ```

#### `PUT /user/profile`
- **Beschreibung:** Aktualisiert die Profildaten des aktuellen Benutzers.
- **Request Body (alle Felder optional):**
  ```json
  {
    "name": "string",
    "favorite_drink": "string",
    "favorite_cocktail": "string",
    "favorite_shot": "string"
  }
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
        "price": "float64",
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
    "count": "int (optional, default: 1)"
  }
  ```
- **Response Body (201 Created):**
  ```json
  // Für count = 1
  {
    "message": "Invite code created successfully",
    "invite": { "id": "uuid", "code": "string" }
  }
  // Für count > 1
  {
    "message": "Invite codes created successfully",
    "invites": [ { "id": "uuid", "code": "string" } ]
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

### **Stripe Webhook**

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
- `viewed`: Einmal aufgerufen, bereit für Registrierung
- `registered`: Für Registrierung verwendet
- `inactive`: Vom Admin deaktiviert

**Wichtig:** Ein Code kann nur einmal "angesehen" werden. Schließt der Benutzer den Browser oder startet das Gerät neu, ist die Chance vertan.