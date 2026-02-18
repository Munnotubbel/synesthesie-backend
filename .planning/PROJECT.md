# synesthesie-backend

## What This Is

Backend für ein privates Party-Event-System. Nur User mit Invite-Code können sich registrieren und Tickets für Veranstaltungen über Stripe/PayPal kaufen. Läuft produktiv und stabil - Änderungen müssen extrem sorgfältig sein.

## Core Value

Zuverlässige Ticketbuchung und private Community für Musikliebhaber - stabile Produktion darf niemals gefährdet werden.

## Requirements

### Validated

- ✓ User-Authentifizierung (Registrierung nur mit Invite-Code, Login, JWT) - existing
- ✓ Event-Verwaltung (CRUD, Deaktivierung) - existing
- ✓ Ticket-Buchung und -Verwaltung - existing
- ✓ Invite-Code-System - existing
- ✓ Stripe Payment Integration - existing
- ✓ PayPal Payment Integration - existing
- ✓ Email-Versand (Bestätigungen, Announcements) - existing
- ✓ Admin-Dashboard Endpoints - existing
- ✓ Backup-System (S3 bei Strato) - existing
- ✓ Audit-Logging - existing

### Active

- [ ] **Bilder-Galerie Feature**
  - Admin kann Bilder hochladen (mehrere gleichzeitig)
  - Bilder in privatem S3 Bucket speichern
  - Admin kann Bilder löschen
  - Admin kann Bilder freigeben/entziehen
  - User können freigegebene Bilder anzeigen
  - Schnelles Laden im Frontend (trotz privatem Bucket)

- [ ] **Musik-Sets Feature**
  - Admin kann Musik-Sets hochladen
  - Admin kann Musik-Sets löschen
  - Admin kann Musik-Sets freigeben/entziehen
  - User können freigegebene Musik-Sets streamen
  - User können Musik-Sets herunterladen
  - Qualitätsbewusstes Streaming (für Musikliebhaber)
  - Traffic-Optimiert (keine riesigen WAV/FLAC direkt ausliefern)

### Out of Scope

- OAuth Login - Email/Password ausreichend
- Mobile App - Web-first
- Public S3 Buckets - Alle Medien bleiben privat

## Context

- Private Party-Reihe für Musikliebhaber
- Kleine, exklusive Community
- Produktion läuft stabil - kein lokales Testing möglich
- Deployment-Testzyklus: Build Image → Live → Testen
- Strato S3 für Backups bereits vorhanden
- Neuer S3 Bucket für Medien (Bilder + Musik) benötigt

## Constraints

- **Tech Stack**: Go 1.23, Gin, GORM - Bestehende Codebasis
- **Database**: PostgreSQL 16, Redis 7 - Produktionsumgebung
- **Payments**: Stripe + PayPal müssen beide funktionieren
- **Storage**: Strato S3 - Traffic-Optimierung kritisch
- **Testing**: Nur Live-Testing möglich - extreme Vorsicht bei Änderungen
- **S3 Security**: Bucket muss privat bleiben (presigned URLs oder Proxy)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Presigned URLs vs Proxy | Private S3 Inhalte ausliefern | — Pending |
| Audio-Format für Streaming | Qualität vs Traffic | — Pending |
| Multi-File Upload Strategie | Admin UX für mehrere Dateien | — Pending |

---
*Last updated: 2026-02-18 after initialization*
