# Technology Stack for Media Features

**Domain:** Media Management (Private S3, Audio Streaming, Multi-File Upload)
**Researched:** 2026-02-18
**Confidence:** HIGH (based on existing codebase analysis)

---

## Recommended Stack

### Storage Layer

**Primary: S3-Compatible Object Storage (Strato)**

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Object Storage | Strato S3 | Already configured, works with existing backup system |
| Access Pattern | Presigned URLs | Already implemented in `s3_service.go:85-92` |
| URL TTL | 15 minutes | Short TTL for security, configurable |
| Path Style | REQUIRED | `UsePathStyle = true` for Strato compatibility |

**Current Implementation (`internal/services/s3_service.go`):**
```go
// Presign GET from media - ALREADY EXISTS
func (s *S3Service) PresignMediaGet(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
    presigner := s3.NewPresignClient(s.mediaClient)
    out, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key}, s3.WithPresignExpires(ttl))
    return out.URL, err
}
```

**What We Need to Add:**
1. New bucket configuration for media (images + audio)
2. Delete operation for S3 objects
3. Potentially: Proxy endpoint as fallback if presigned URLs have issues with Strato

### Presigned URL vs Proxy Decision

| Aspect | Presigned URLs | Backend Proxy |
|--------|----------------|---------------|
| Performance | ✓ Direct from S3, no server bottleneck | ✗ All traffic through server |
| Cost | ✓ Lower server bandwidth | ✗ Double bandwidth (S3→Server→Client) |
| Security | ⚠️ URLs can be shared (short TTL mitigates) | ✓ Full control, can validate each request |
| Compatibility | ⚠️ Depends on Strato S3 support | ✓ Works with any S3 |
| Caching | ⚠️ No server-side cache | ✓ Can implement server cache |

**Recommendation: Presigned URLs as Primary, Proxy as Fallback**

Implement both:
- Primary: Presigned URLs for speed and cost
- Fallback: Proxy endpoint `/api/v1/user/media/:id/stream` if presigned fails

### SDK & Libraries

| Library | Version | Purpose |
|---------|---------|---------|
| `github.com/aws/aws-sdk-go-v2` | Current | S3 operations (already in use) |
| `github.com/aws/aws-sdk-go-v2/feature/s3/manager` | Current | Multipart upload/download (already in use) |
| `golang.org/x/sync/singleflight` | NEW | Prevent cache stampede on concurrent requests |

### Image Processing

**For Thumbnails/Resizing (Optional - Phase 2):**

| Option | Pros | Cons |
|--------|------|------|
| Native Go (`image` package) | No dependencies | Slower, limited formats |
| `github.com/disintegration/imaging` | Simple API, good performance | CGO not needed |
| Pre-generate on upload | Best runtime performance | More storage |

**Recommendation:** Pre-generate thumbnails on upload, store multiple sizes in S3.

### Audio Processing

**Quality Tiers (Recommendation):**

| Tier | Format | Bitrate | Use Case |
|------|--------|---------|----------|
| Original | FLAC/Source | Lossless | Download only |
| High | MP3 | 320 kbps | WiFi streaming |
| Medium | MP3 | 192 kbps | Default streaming |
| Low | AAC | 128 kbps | Mobile/limited data |

**Important:** Pre-encode on upload, don't transcode on-the-fly.

### Upload Handling

**Current Pattern (`admin_handler.go:1079-1121`):**
- Multipart form upload
- Direct streaming to S3 via manager.Uploader
- 10MB part size for multipart
- Individual file size limit: 4GB

**What to Add:**
1. Concurrency limit per admin (prevent memory exhaustion)
2. Total upload size limit per request
3. MIME type validation by content, not extension

---

## New Environment Variables Needed

```bash
# New media bucket (images + audio)
MEDIA_ASSETS_BUCKET=your-strato-bucket-name

# Reuse existing Strato S3 credentials
# MEDIA_S3_ENDPOINT, MEDIA_S3_ACCESS_KEY_ID, MEDIA_S3_SECRET_ACCESS_KEY already exist

# Optional: CDN configuration (future)
# MEDIA_CDN_ENABLED=false
# MEDIA_CDN_URL=
```

---

## Configuration Changes

**Add to `internal/config/config.go`:**

```go
// Media bucket for images and audio
MediaAssetsBucket string

// Upload limits
MaxConcurrentUploads int    // Per admin
MaxUploadSizeTotal   int64  // Total bytes across all files in request

// Audio quality settings
AudioQualityDefault string // "medium", "high", "original"
AudioPreEncode      bool   // Pre-encode quality tiers on upload
```

---

## Database Models

**New Tables Needed:**

| Table | Purpose |
|-------|---------|
| `images` | Image metadata with visibility |
| `music_sets` | Music set/album metadata |
| `music_tracks` | Individual tracks within sets |

See ARCHITECTURE.md for detailed model definitions.

---

## API Endpoints

### Admin Endpoints (Protected by AdminOnly middleware)

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/admin/images/upload` | Upload single/multiple images |
| PUT | `/api/v1/admin/images/:id` | Update image metadata/visibility |
| DELETE | `/api/v1/admin/images/:id` | Delete image |
| POST | `/api/v1/admin/music-sets` | Create music set |
| POST | `/api/v1/admin/music-sets/:id/tracks` | Add track to set |
| DELETE | `/api/v1/admin/music-sets/:id` | Delete music set |
| PUT | `/api/v1/admin/music-sets/:id/visibility` | Toggle visibility |

### User Endpoints (Protected by Auth middleware)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/user/images` | List visible images |
| GET | `/api/v1/user/images/:id` | Get image with presigned URL |
| GET | `/api/v1/user/music-sets` | List visible music sets |
| GET | `/api/v1/user/music-sets/:id` | Get music set details |
| GET | `/api/v1/user/music-sets/:id/stream/:trackId` | Get stream URL |
| GET | `/api/v1/user/music-sets/:id/download/:trackId` | Get download URL |

---

## Fallback: Proxy Endpoint

If presigned URLs don't work reliably with Strato S3:

```go
// Proxy endpoint that fetches from S3 and serves to client
func (h *MediaHandler) StreamAudio(c *gin.Context) {
    // 1. Validate user has access
    // 2. Get presigned URL (or direct download)
    // 3. Proxy response with Range support
    // 4. Support seeking via Range headers
}
```

---

## Sources

| Source | Confidence | Notes |
|--------|------------|-------|
| Existing s3_service.go | HIGH | Presigned URL implementation verified |
| Existing config patterns | HIGH | Follow existing patterns |
| Strato S3 documentation | MEDIUM | Need to verify presigned URL support |
| Audio streaming best practices | MEDIUM | Industry standard patterns |

**Verification Needed:**
- Test presigned URLs with Strato S3 specifically
- Verify Range request support with presigned URLs
- Confirm multipart upload works reliably with Strato

---

*Stack research completed: 2026-02-18*
