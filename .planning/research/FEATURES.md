# Feature Landscape

**Domain:** Event ticketing backend with media gallery and music streaming
**Researched:** 2026-02-18
**Focus:** Image gallery + Audio streaming for music lovers community

---

## Table Stakes

Features users expect. Missing = product feels incomplete.

### Image Gallery

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Grid view with thumbnails | Standard gallery UX | Low | Lazy loading required for performance |
| Lightbox/full-screen view | Users expect to tap-to-expand | Low | Basic swipe navigation between images |
| Pagination or infinite scroll | Galleries can have 100+ photos | Medium | Cursor-based preferred for performance |
| Image metadata display | Date, event context, photographer | Low | EXIF data optionally preserved |
| Basic filtering | By event/date | Low | Essential for multi-event galleries |
| Responsive images | Mobile users are 60%+ of traffic | Medium | Serve WebP/AVIF with JPEG fallback |

### Audio Streaming

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Stream playback | Core expectation | Low | Range request support |
| Track listing | See what's available | Low | Metadata display (artist, title, duration) |
| Play/pause/seek | Basic player controls | Medium | Seek requires accurate duration metadata |
| Volume control | Standard player feature | Low | |
| Download option | Music lovers want offline | Medium | Permission-gated |
| Background playback | Mobile users expect this | Medium | iOS requires special handling |

### Admin Upload

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Multi-file selection | Uploading 20+ images one-by-one is painful | Medium | HTML5 multiple file input |
| Upload progress | Feedback for large files | Medium | WebSocket or SSE for real-time updates |
| Drag-and-drop | Modern UX standard | Low | |
| Bulk delete/remove | Mistakes happen, need cleanup | Low | |
| File type validation | Prevent upload of unsupported formats | Low | Client + server validation |
| Size limits | Prevent abuse/server overload | Low | Configurable per file type |

---

## Differentiators

Features that set product apart. Not expected, but valued.

### For Music Lovers

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| High-quality audio toggle | Offer FLAC/320kbps for audiophiles | High | Requires multiple encodes, CDN strategy |
| Audio waveform visualization | Visual appeal, shows where drops/buildups are | Medium | Generate on upload, cache |
| Track artwork/thumbnails | Professional presentation | Low | Extract from embedded art or upload separately |
| Set/playlist continuity | Gapless playback for DJ sets | High | Requires precise encoding/segmentation |
| BPM/genre tagging | Music discovery and organization | Medium | Auto-detect BPM libraries available |
| Shareable clips | 30-second previews for social | Medium | Generate clips on upload |

### For Community

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| User-generated tags | Community organization | Medium | Moderation required |
| Favorite/bookmark | Save tracks for later | Low | Personal feature, low privacy concerns |
| Download history | Re-download previously purchased | Low | |
| QR code gallery access | At-event access without login | Low | Temporary token-based access |

### For Admins

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Upload presets | Pre-configured quality settings | Low | Speed up bulk uploads |
| Batch metadata editing | Apply artist/event to multiple files | Medium | Spreadsheet-like UI |
| Upload scheduling | Content goes live at specific time | Medium | Delayed publication |
| Analytics dashboard | Most viewed/listened content | High | Requires tracking infrastructure |
| CDN integration preview | See where files will be served | Low | |

---

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Social comments on media | Moderation nightmare, not core value | Keep chat/feedback separate from media |
| User uploads to main gallery | Permission complexity, moderation | Separate "fan photos" section if needed |
| Video streaming | Complexity explosion, different CDN needs | Use YouTube/Vimeo embedding for video |
| Real-time collaboration | Overkill for this use case | Simple concurrent upload handling |
| In-browser editing | Feature creep, better done elsewhere | Link to external tools or do basic crops only |

---

## Feature Dependencies

```
Multi-file upload → Batch metadata editing → Upload scheduling

Image thumbnails → Lightbox view → Gallery filters

Audio transcode (multiple qualities) → Quality toggle → Bandwidth optimization

Waveform generation → Waveform visualization → Track preview clips
```

---

## MVP Recommendation

### Phase 1: Core Gallery (Table Stakes)
**Priority: HIGH**
- Multi-file image upload with progress
- Grid view with thumbnails
- Lightbox full-screen view
- Basic pagination
- Image format validation (JPG, PNG, WebP)

**Rationale:** Photos are primary event documentation. Users expect this immediately.

### Phase 2: Core Audio Streaming (Table Stakes)
**Priority: HIGH**
- Multi-file audio upload with progress
- Basic audio player (play, pause, seek, volume)
- Track listing with metadata
- Stream with range requests
- Download permission gate

**Rationale:** Core value for music lovers. Must work flawlessly.

### Phase 3: Optimization (Quality + Bandwidth)
**Priority: MEDIUM**
- Audio quality toggle (streaming vs download quality)
- Responsive image generation (WebP + sizes)
- CDN integration for media delivery
- Waveform visualization

**Rationale:** Performance differentiator. Bandwidth costs matter.

### Defer to Later
- Gapless playback (Phase 4)
- Advanced analytics (Phase 4)
- User-generated tags (Phase 5)
- Shareable clips (Phase 5)

---

## Audio Format Recommendations

### Streaming (Low Bandwidth)
| Format | Bitrate | Quality | Use Case |
|--------|---------|---------|----------|
| AAC | 128-192 kbps | Good | Default streaming, mobile |
| Opus | 96-128 kbps | Very Good | Low-bandwidth mode |

### High Quality (Download/Audiophile)
| Format | Bitrate/Depth | Quality | Use Case |
|--------|--------------|---------|----------|
| MP3 | 320 kbps (CBR) | Very Good | Download option, broad compatibility |
| FLAC | 16-bit/44.1kHz | Lossless | Premium download option |
| AAC | 256 kbps | Excellent | Apple ecosystem preference |

### Recommendation Strategy
```
Upload Source (WAV/FLAC/AIFF)
    ↓
[Transcode on upload]
    ├→ 128kbps AAC (streaming default)
    ├→ 256kbps AAC (high quality toggle)
    └→ 320kbps MP3 (download)
```

**Storage trade-off:** ~3x source size for all three qualities
**Bandwidth savings:** 85% reduction streaming 128kbps vs lossless

---

## Image Optimization Strategy

### Generate on Upload
| Size | Max Dimension | Use Case |
|------|--------------|----------|
| Thumbnail | 300x300 | Gallery grid |
| Medium | 1200x1200 | Lightbox default |
| Full | 2400x2400 (or original) | Download/high-res view |

### Format Priority
1. **WebP** - Modern browsers, 25-35% smaller than JPEG
2. **JPEG** - Fallback for older browsers
3. **AVIF** - Next-gen, 50% smaller than JPEG (Phase 4)

### Compression Settings
| Image Type | Quality | Notes |
|------------|---------|-------|
| Photos (event) | 80-85% | Balance quality vs size |
| Thumbnails | 75% | Artifacts less visible at small size |
| Artwork | 90% | Square, high detail retention |

---

## Multi-File Upload Patterns for Go

### Pattern 1: Multipart Form (Standard)
**When:** Admin uploads via web UI
**Pros:** Browser native, no JS libraries needed
**Cons:** Single request timeout for many files

```go
// Handler signature
func UploadMultiple(c *gin.Context) {
    form, err := c.MultipartForm()
    files := form.File["files"]
    // Process each file
}
```

### Pattern 2: Chunked Upload (Large Files)
**When:** Individual files >50MB (DJ sets, FLAC albums)
**Pros:** Resumable, better timeout handling
**Cons:** Requires client-side chunking logic

```go
// Chunk reassembly on server
func UploadChunk(c *gin.Context) {
    chunkIndex := c.PostForm("chunkIndex")
    totalChunks := c.PostForm("totalChunks")
    fileId := c.PostForm("fileId")
    // Append chunk to temp file
    // If last chunk, finalize
}
```

### Pattern 3: Background Processing
**When:** Transcoding/encoding required
**Pros:** Non-blocking upload, better UX
**Cons:** Requires job queue

```go
// 1. Accept upload, store in temp
// 2. Return immediately with job ID
// 3. Process in background (goroutine or job queue)
// 4. Notify when ready (WebSocket/poll)
```

### Recommendation
- **Phase 1:** Pattern 1 (Multipart) for simplicity
- **Phase 2:** Add Pattern 3 (Background) for audio transcoding
- **Phase 3:** Pattern 2 (Chunked) if sets exceed 100MB

---

## Quality vs Bandwidth Trade-offs

### Decision Framework
```
┌─────────────────────────────────────────────────────────┐
│                    User Connection                      │
├─────────────────────────────────────────────────────────┤
│  Slow (<3Mbps)     │  Medium (3-10Mbps)  │  Fast (>10Mbps) │
│  ─────────────────────────────────────────────────────  │
│  96kbps Opus       │  128kbps AAC        │  256kbps AAC    │
│  600px images      │  1200px images      │  Full quality   │
└─────────────────────────────────────────────────────────┘
```

### Adaptive Streaming vs Progressive Download

| Approach | Pros | Cons | Recommendation |
|----------|------|------|----------------|
| **Progressive Download** | Simple, works everywhere, seek after download starts | No adaptation to network, wastes bandwidth if not fully played | **YES for Phase 1-2** |
| **HLS/DASH** | Adaptive bitrate, standard in industry | Complex setup, requires segmentation, higher latency | **Phase 3** if bandwidth costs critical |
| **Hybrid** | Progressive for short tracks, HLS for long mixes | Implementation complexity | **Phase 4** |

### Recommendation for This Context
**Progressive download with quality toggle** is optimal because:
- DJ sets are consumed start-to-finish (high completion rate)
- Private community → bandwidth costs more predictable
- Simpler implementation = faster time to value
- Quality toggle respects audiophile preferences

**Bandwidth optimization tactics:**
1. Serve lower quality by default, high quality on request
2. Implement aggressive browser caching (1 year for immutable assets)
3. Use CDN with edge caching for popular content
4. Preload next track in playlist (hidden iframe)

---

## Storage Estimates

### Per Event
| Media Type | Quantity | Avg Size | Storage |
|------------|----------|----------|---------|
| Photos | 200 | 2MB (optimized) | 400 MB |
| Audio tracks | 15 | 80MB (3 qualities) | 1.2 GB |
| **Per event total** | | | **~1.6 GB** |

### Annual (50 events)
| Type | Storage |
|------|---------|
| Photos | 20 GB |
| Audio | 60 GB |
| **Total** | **80 GB** |

### CDN Costs (Estimate)
- **Storage:** $0.05/GB/month = $4/month
- **Egress:** $0.08/GB × 500GB/month = $40/month
- **Total estimated:** ~$50/month for media delivery

---

## Sources

**Confidence: MEDIUM** (Based on established industry practices and platform patterns)

- Image optimization: WebP adoption patterns (Can I Use, 2024 data)
- Audio streaming: Spotify/SoundCloud public technical discussions
- Multipart upload: Go standard library documentation
- CDN pricing: Cloudflare R2, AWS CloudFront 2025 pricing

**Research gaps (flag for Phase-specific research):**
- Exact Go libraries for audio transcoding (ffmpeg wrappers)
- CDN selection for this use case
- Exact implementation of gapless playback
