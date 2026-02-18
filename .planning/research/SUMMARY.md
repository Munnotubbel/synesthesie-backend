# Research Summary: Media Gallery & Music Streaming Features

**Domain:** Private event ticketing backend - Media gallery and music streaming extension
**Researched:** 2026-02-18
**Overall confidence:** HIGH (based on existing codebase analysis)

---

## Executive Summary

This research addresses adding **image gallery** and **music streaming** capabilities to an existing event ticketing backend for a private party community of music lovers. Key constraints:
- Production-only testing (no local environment)
- Strato S3 for storage (must use path-style, presigned URLs)
- Quality is CRITICAL for audiophiles
- Traffic costs must be optimized

The existing codebase already has:
- `S3Service` with presigned URL support (`internal/services/s3_service.go:85-92`)
- Asset model with visibility field
- Upload patterns in admin_handler.go
- Local caching for audio files

---

## Key Findings

### Stack Recommendations

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Content delivery | **Presigned URLs (primary)** + Proxy fallback | Direct S3 access, lower cost; proxy if Strato issues |
| URL TTL | 15 minutes | Already implemented, good security |
| Path style | REQUIRED for Strato | `UsePathStyle = true` critical for compatibility |
| Audio format | Store original + pre-encoded tiers | No on-the-fly transcoding |
| Image format | Original + thumbnails on upload | Multiple sizes pre-generated |

### Audio Quality Tiers (Recommended)

| Tier | Format | Bitrate | Use Case |
|------|--------|---------|----------|
| Original | FLAC/Source | Lossless | Download only |
| High | MP3 | 320 kbps | WiFi streaming |
| Medium | MP3 | 192 kbps | Default streaming |

### Critical Pitfalls to Avoid

1. **Presigned URL Leakage** - Short TTL, never cache, per-request generation
2. **Strato S3 Compatibility** - Always use path-style, test all operations
3. **Memory Exhaustion** - Limit concurrent uploads per admin
4. **Orphaned S3 Objects** - Delete S3 before DB record
5. **Audio Quality Loss** - Pre-encode, don't transcode on-the-fly
6. **Cache Stampede** - Use singleflight pattern for concurrent downloads

---

## Architecture Integration

**New Components Needed:**

| Component | Location | Purpose |
|-----------|----------|---------|
| Image model | `internal/models/image.go` | Image metadata + visibility |
| MusicSet model | `internal/models/music_set.go` | Music set + tracks |
| MediaService | `internal/services/media_service.go` | Business logic orchestration |
| MediaHandler | `internal/handlers/media_handler.go` | HTTP endpoints |

**Reuse Existing:**
- `S3Service` - Upload, presign, download
- `AssetService` - Asset record management
- `StorageService` - Local file operations
- Auth middleware - Already working

---

## Implications for Roadmap

Based on research, recommended **4-phase approach**:

### Phase 1: Infrastructure & Image Gallery
- Database models (Image, MusicSet, MusicTrack)
- Config for new media bucket
- Image upload/delete/visibility endpoints
- User image viewing endpoints
- Presigned URL generation for images

### Phase 2: Music Set Management
- MusicSet CRUD operations
- Track upload and management
- Visibility controls
- Admin management UI endpoints

### Phase 3: Audio Streaming & Downloads
- Stream endpoint with presigned URLs
- Download endpoint
- Quality tier selection
- Local caching with singleflight

### Phase 4: Optimization & Polish
- Thumbnail generation
- Audio quality tier pre-encoding
- Cache optimization
- Analytics (optional)

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Existing S3 patterns | HIGH | Code reviewed, presigned URLs working |
| Image features | HIGH | Standard patterns, existing upload code |
| Audio streaming | MEDIUM | Strato presigned URLs for large files needs testing |
| Multi-file upload | HIGH | Go multipart well-established |
| Quality optimization | MEDIUM | Audiophile requirements need validation |

---

## Gaps to Address During Implementation

1. **Strato S3 Presigned URL Testing** - Verify works with large audio files
2. **Range Request Support** - Test seeking in audio via presigned URLs
3. **Concurrent Upload Limits** - Implement per-admin rate limiting
4. **Audio Pre-encoding** - Decide: accept only pre-encoded or add ffmpeg?

---

## Files Created

| File | Lines | Content |
|------|-------|---------|
| STACK.md | ~180 | Technology choices, SDKs, config |
| FEATURES.md | 327 | Feature categorization, table stakes |
| ARCHITECTURE.md | 810 | Models, services, handlers, integration |
| PITFALLS.md | 489 | 15 pitfalls with prevention strategies |
| SUMMARY.md | This file | Synthesis and recommendations |

---

*Research completed: 2026-02-18*
