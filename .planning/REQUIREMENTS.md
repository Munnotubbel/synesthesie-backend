# Requirements: synesthesie-backend Media Features

**Defined:** 2026-02-18
**Core Value:** Private Community fur Musikliebhaber mit qualitatsbewusstem Media-Management

## v1 Requirements

### Infrastructure (INFRA)

- [ ] **INFRA-01**: Neue Umgebungsvariable MEDIA_ASSETS_BUCKET fur Medien-Bucket
- [ ] **INFRA-02**: Datenbank-Modelle: Image, MusicSet, MusicTrack
- [ ] **INFRA-03**: GORM AutoMigrate fur neue Tabellen
- [ ] **INFRA-04**: Config-Erweiterung fur Media-Settings (Upload-Limits, etc.)

### Image Gallery - Admin (IMG-ADM)

- [ ] **IMG-ADM-01**: Admin kann einzelne Bilder hochladen
- [ ] **IMG-ADM-02**: Admin kann mehrere Bilder gleichzeitig hochladen (Multipart)
- [ ] **IMG-ADM-03**: Admin kann Bilder loschen (S3 + DB)
- [ ] **IMG-ADM-04**: Admin kann Sichtbarkeit von Bildern andern (private/public)
- [ ] **IMG-ADM-05**: Admin kann Bild-Metadaten bearbeiten (Titel, Beschreibung)
- [ ] **IMG-ADM-06**: MIME-Type Validierung beim Upload (Content-Erkennung)

### Image Gallery - User (IMG-USR)

- [ ] **IMG-USR-01**: User kann Liste aller freigegebenen Bilder abrufen
- [ ] **IMG-USR-02**: User kann einzelnes Bild mit presigned URL abrufen
- [ ] **IMG-USR-03**: Bilder werden schnell geladen (presigned URLs, Cache-Header)

### Music Sets - Admin (MSC-ADM)

- [ ] **MSC-ADM-01**: Admin kann Music-Set erstellen (Titel, Beschreibung)
- [ ] **MSC-ADM-02**: Admin kann Tracks zu Music-Set hinzufugen
- [ ] **MSC-ADM-03**: Admin kann Tracks aus Music-Set entfernen
- [ ] **MSC-ADM-04**: Admin kann Music-Set loschen (S3 + DB)
- [ ] **MSC-ADM-05**: Admin kann Sichtbarkeit von Music-Sets andern
- [ ] **MSC-ADM-06**: Admin kann Track-Metadaten bearbeiten (Titel, Artist)
- [ ] **MSC-ADM-07**: GroBe-Limit fur Audio-Uploads (4GB max)

### Music Sets - User (MSC-USR)

- [ ] **MSC-USR-01**: User kann Liste aller freigegebenen Music-Sets abrufen
- [ ] **MSC-USR-02**: User kann Details eines Music-Sets abrufen (mit Tracks)
- [ ] **MSC-USR-03**: User kann Track streamen (presigned URL)
- [ ] **MSC-USR-04**: User kann Track herunterladen (presigned URL)
- [ ] **MSC-USR-05**: Streaming unterstutzt Seeking (Range Requests)

### Security & Performance (SEC)

- [ ] **SEC-01**: Presigned URLs haben kurze TTL (15 min)
- [ ] **SEC-02**: Upload-Rate-Limiting pro Admin
- [ ] **SEC-03**: S3 Bucket bleibt privat (kein public access)
- [ ] **SEC-04**: Orphaned S3 Objects werden vermieden (Delete-Logik)

## v2 Requirements

### Optimization (OPT)

- **OPT-01**: Thumbnail-Generierung fur Bilder
- **OPT-02**: Audio-Qualitats-Tiers (192kbps, 320kbps)
- **OPT-03**: Local Caching mit Singleflight-Pattern
- **OPT-04**: Cache-Warming bei Freigabe

### Analytics (ANL)

- **ANL-01**: Track-Aufrufe zahlen
- **ANL-02**: Beliebteste Tracks/Music-Sets

## Out of Scope

| Feature | Reason |
|---------|--------|
| On-the-fly Audio Transcoding | Zu rechenintensiv, pre-encode statttdessen |
| HLS/DASH Streaming | Progressive Download reicht fur kleine Community |
| CDN Integration | Strato S3 erstmal ausreichend |
| Mobile App | Web-first |
| Waveform Visualization | Nice-to-have fur spater |
| Gapless Playback | Komplex, nicht kritisch |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| INFRA-01 | Phase 1 | Pending |
| INFRA-02 | Phase 1 | Pending |
| INFRA-03 | Phase 1 | Pending |
| INFRA-04 | Phase 1 | Pending |
| IMG-ADM-01 | Phase 1 | Pending |
| IMG-ADM-02 | Phase 1 | Pending |
| IMG-ADM-03 | Phase 1 | Pending |
| IMG-ADM-04 | Phase 1 | Pending |
| IMG-ADM-05 | Phase 1 | Pending |
| IMG-ADM-06 | Phase 1 | Pending |
| IMG-USR-01 | Phase 1 | Pending |
| IMG-USR-02 | Phase 1 | Pending |
| IMG-USR-03 | Phase 1 | Pending |
| MSC-ADM-01 | Phase 2 | Pending |
| MSC-ADM-02 | Phase 2 | Pending |
| MSC-ADM-03 | Phase 2 | Pending |
| MSC-ADM-04 | Phase 2 | Pending |
| MSC-ADM-05 | Phase 2 | Pending |
| MSC-ADM-06 | Phase 2 | Pending |
| MSC-ADM-07 | Phase 2 | Pending |
| MSC-USR-01 | Phase 3 | Pending |
| MSC-USR-02 | Phase 3 | Pending |
| MSC-USR-03 | Phase 3 | Pending |
| MSC-USR-04 | Phase 3 | Pending |
| MSC-USR-05 | Phase 3 | Pending |
| SEC-01 | Phase 1 | Pending |
| SEC-02 | Phase 1 | Pending |
| SEC-03 | Phase 1 | Pending |
| SEC-04 | Phase 1 | Pending |

**Coverage:**
- v1 requirements: 29 total
- Mapped to phases: 29
- Unmapped: 0

---
*Requirements defined: 2026-02-18*
*Last updated: 2026-02-18 after roadmap creation*
