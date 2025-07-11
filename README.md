# Synesthesie Backend

Ein modernes Event-Management-System mit QR-Code-basierter Einladungsfunktion, entwickelt in Go.

## 🚀 Quick Start

```bash
# Repository klonen
git clone https://github.com/synesthesie/backend.git
cd synesthesie-backend

# Entwicklungsumgebung starten (Datenbank + Redis)
make dev-deps

# In neuem Terminal: API starten
make run

# Oder alles zusammen:
make dev
```

Die API läuft dann auf http://localhost:8080

**Zusätzliche Tools:**
- Adminer (Datenbank-UI): http://localhost:8081
- Health Check: http://localhost:8080/api/v1/health

## Features

- **Benutzer-Management**: Registrierung über QR-Code-Einladungen
- **Event-Management**: Erstellung und Verwaltung von Events
- **Ticket-System**: Stripe-Integration für Zahlungen
- **Abhol- und Bringservice**: Optional buchbar für Events
- **Admin-Dashboard**: Umfassende Verwaltungsfunktionen
- **E-Mail-Benachrichtigungen**: Automatische Bestätigungen und Erinnerungen

## Tech Stack

- **Backend**: Go 1.21+ mit Gin Framework
- **Datenbank**: PostgreSQL
- **Cache**: Redis
- **Zahlungen**: Stripe
- **Container**: Docker & Docker Compose
- **Deployment**: Ubuntu VPS mit Nginx

## Entwicklungsumgebung einrichten

### Voraussetzungen

- Go 1.21 oder höher
- Docker und Docker Compose
- Git

### Installation

1. Repository klonen:
```bash
git clone https://github.com/synesthesie/backend.git
cd synesthesie-backend
```

2. Umgebungsvariablen konfigurieren:
```bash
cp .env.example .env
# .env Datei mit deinen Werten anpassen
```

3. Dependencies installieren:
```bash
go mod download
```

4. Docker Container starten:
```bash
docker-compose up -d postgres redis
```

5. Datenbank-Migrationen ausführen:
```bash
go run cmd/api/main.go
# Die App führt Migrationen automatisch beim Start aus
```

### Entwicklung

Backend lokal starten:
```bash
go run cmd/api/main.go
```

Das Backend läuft dann auf `http://localhost:8080`

### Tests ausführen

```bash
go test ./...
```

## API Endpoints

### Öffentliche Endpoints

- `GET /api/v1/health` - Health Check
- `GET /api/v1/public/events` - Kommende Events anzeigen
- `GET /api/v1/public/invite/:code` - Einladungscode prüfen

### Auth Endpoints

- `POST /api/v1/auth/register` - Registrierung mit Einladungscode
- `POST /api/v1/auth/login` - Anmeldung
- `POST /api/v1/auth/refresh` - Token erneuern
- `POST /api/v1/auth/logout` - Abmelden

### User Endpoints (Auth erforderlich)

- `GET /api/v1/user/profile` - Profil anzeigen
- `PUT /api/v1/user/profile` - Profil aktualisieren
- `GET /api/v1/user/events` - Events mit Ticketstatus
- `GET /api/v1/user/tickets` - Eigene Tickets
- `POST /api/v1/user/tickets` - Ticket buchen
- `DELETE /api/v1/user/tickets/:id` - Ticket stornieren

### Admin Endpoints (Admin-Rechte erforderlich)

#### Event-Verwaltung
- `GET /api/v1/admin/events` - Alle Events
- `POST /api/v1/admin/events` - Event erstellen
- `PUT /api/v1/admin/events/:id` - Event bearbeiten
- `DELETE /api/v1/admin/events/:id` - Event löschen
- `POST /api/v1/admin/events/:id/deactivate` - Event deaktivieren
- `POST /api/v1/admin/events/:id/refund` - Alle Tickets erstatten

#### Einladungs-Verwaltung
- `GET /api/v1/admin/invites` - Alle Einladungscodes
- `POST /api/v1/admin/invites` - Einladungscode(s) erstellen
- `DELETE /api/v1/admin/invites/:id` - Einladung deaktivieren

#### Benutzer-Verwaltung
- `GET /api/v1/admin/users` - Alle Benutzer
- `GET /api/v1/admin/users/:id` - Benutzerdetails
- `PUT /api/v1/admin/users/:id/password` - Passwort zurücksetzen

#### Einstellungen
- `GET /api/v1/admin/settings/pickup-price` - Abholservice-Preis
- `PUT /api/v1/admin/settings/pickup-price` - Preis aktualisieren

### Stripe Webhook

- `POST /api/v1/stripe/webhook` - Stripe Payment Events

## Deployment

### Docker Build

```bash
docker build -t synesthesie-api .
```

### Docker Compose (Production)

```bash
docker-compose up -d
```

### Nginx Konfiguration

```nginx
server {
    listen 443 ssl http2;
    server_name api.synesthesie.de;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Umgebungsvariablen

Siehe `.env.example` für alle verfügbaren Konfigurationsoptionen.

Wichtige Variablen:
- `JWT_SECRET` - Sicherer Secret für JWT-Tokens
- `STRIPE_SECRET_KEY` - Stripe API Key
- `STRIPE_WEBHOOK_SECRET` - Stripe Webhook Secret
- `ADMIN_USERNAME/PASSWORD` - Initial Admin Credentials

## Sicherheit

- Passwörter werden mit bcrypt gehasht
- JWT-Tokens für Authentifizierung
- Rate Limiting zum Schutz vor Brute Force
- CORS konfiguriert für Frontend-Domain
- Eingabevalidierung auf allen Endpoints

## Lizenz

Proprietary - Alle Rechte vorbehalten