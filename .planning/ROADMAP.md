# Roadmap: synesthesie-backend Media Features

## Overview

Adding image gallery and music streaming capabilities to a production event ticketing backend for a private party community. This is a live system with no local testing environment, so each phase is designed to be safely deployable without breaking existing functionality. The work progresses from foundation (database, S3, image upload) to music management (admin tools) to user-facing features (streaming, downloads).

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Infrastructure & Image Gallery** - Foundation for media storage and complete image gallery feature
- [ ] **Phase 2: Music Set Management** - Admin tools for creating and managing music sets
- [ ] **Phase 3: Music Streaming & Downloads** - User-facing music playback and download functionality

## Phase Details

### Phase 1: Infrastructure & Image Gallery

**Goal**: Admins can upload and manage images, users can view approved images through secure presigned URLs

**Depends on**: Nothing (extends existing system)

**Requirements**: INFRA-01, INFRA-02, INFRA-03, INFRA-04, IMG-ADM-01, IMG-ADM-02, IMG-ADM-03, IMG-ADM-04, IMG-ADM-05, IMG-ADM-06, IMG-USR-01, IMG-USR-02, IMG-USR-03, SEC-01, SEC-02, SEC-03, SEC-04

**Success Criteria** (what must be TRUE):
1. Admin can upload single and multiple images to private S3 bucket with metadata
2. Admin can delete images and change their visibility (private/public)
3. User can retrieve list of approved images and load them via presigned URLs
4. S3 bucket remains private with no public access, all content delivered via presigned URLs
5. Upload rate limiting prevents abuse and orphaned S3 objects are cleaned up on delete

**Plans**: TBD

Plans:
- [ ] 01-01: Database models and migrations for Image table
- [ ] 01-02: S3 bucket configuration and media service foundation
- [ ] 01-03: Admin image upload endpoints (single and multipart)
- [ ] 01-04: Admin image management endpoints (delete, visibility, metadata)
- [ ] 01-05: User image viewing endpoints with presigned URLs
- [ ] 01-06: Security controls (rate limiting, TTL, orphan cleanup)

### Phase 2: Music Set Management

**Goal**: Admins can create music sets, upload tracks, and manage metadata and visibility

**Depends on**: Phase 1 (reuses infrastructure patterns)

**Requirements**: MSC-ADM-01, MSC-ADM-02, MSC-ADM-03, MSC-ADM-04, MSC-ADM-05, MSC-ADM-06, MSC-ADM-07

**Success Criteria** (what must be TRUE):
1. Admin can create music sets with title and description
2. Admin can upload audio tracks to music sets (including large files up to 4GB)
3. Admin can remove tracks from sets and delete entire music sets
4. Admin can change music set visibility and edit track metadata (title, artist)
5. S3 storage is properly managed - deletion removes both DB records and S3 objects

**Plans**: TBD

Plans:
- [ ] 02-01: Database models for MusicSet and MusicTrack tables
- [ ] 02-02: Admin music set CRUD endpoints
- [ ] 02-03: Admin track upload and management endpoints
- [ ] 02-04: Admin visibility and metadata management endpoints

### Phase 3: Music Streaming & Downloads

**Goal**: Users can browse, stream, and download approved music sets

**Depends on**: Phase 2 (requires music sets to exist)

**Requirements**: MSC-USR-01, MSC-USR-02, MSC-USR-03, MSC-USR-04, MSC-USR-05

**Success Criteria** (what must be TRUE):
1. User can retrieve list of all approved music sets with track details
2. User can stream audio tracks through presigned URLs that support seeking
3. User can download tracks directly from S3 via presigned URLs
4. Streaming works efficiently with range requests for audio players

**Plans**: TBD

Plans:
- [ ] 03-01: User endpoints for browsing music sets and tracks
- [ ] 03-02: Streaming endpoint with presigned URL generation
- [ ] 03-03: Download endpoint with presigned URL generation
- [ ] 03-04: Range request support for audio seeking

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Infrastructure & Image Gallery | 0/6 | Not started | - |
| 2. Music Set Management | 0/4 | Not started | - |
| 3. Music Streaming & Downloads | 0/4 | Not started | - |
