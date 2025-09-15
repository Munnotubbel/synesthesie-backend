# Synesthesie Backend - Lokale Entwicklung

## 🚀 Quick Start

```bash
cd docker/local
docker-compose up -d
```

## 📍 Services

Nach dem Start sind folgende Services verfügbar:

- **Backend API**: http://localhost:8080
- **PostgreSQL**: localhost:5433
- **Redis**: localhost:6379

## 🌐 Frontend

Das Frontend wird auf http://localhost:8081 erwartet.

## 🛠️ Nützliche Befehle

```bash
# Services starten
docker-compose up -d

# Logs anzeigen
docker-compose logs -f

# Nur App-Logs
docker-compose logs -f app

# Services stoppen
docker-compose down

# Services stoppen und Daten löschen
docker-compose down -v

# Status prüfen
docker-compose ps
```

## 🔧 Konfiguration

Die Konfiguration erfolgt über Environment-Variablen in der `docker-compose.yml`.

Wichtige Variablen:
- `CORS_ALLOWED_ORIGINS`: Enthält http://localhost:8081 für dein Frontend
- `ADMIN_USERNAME/PASSWORD`: Standard Admin-Zugangsdaten
- `JWT_SECRET`: Sollte in Produktion geändert werden

## 📝 Hinweise

- Die Datenbank wird automatisch initialisiert
- Volumes bleiben zwischen Neustarts erhalten
- Logs werden im `logs/` Verzeichnis gespeichert