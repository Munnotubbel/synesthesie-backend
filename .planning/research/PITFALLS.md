# Domain Pitfalls

**Domain:** Media Management (Private S3, Audio Streaming, Multi-File Upload)
**Researched:** 2026-02-18
**Production Context:** Live deployment only - no local testing environment

---

## Critical Pitfalls

Mistakes that cause rewrites, data loss, security breaches, or production outages.

### Pitfall 1: Presigned URL Token Leakage
**What goes wrong:** Presigned URLs shared externally, cached by browsers/CDNs, or logged allow unauthorized access to private media long after expiration.

**Why it happens:**
- URLs are logged in access logs, analytics, or shared in browser history
- Browser prefetching caches URLs
- Users bookmark or share direct S3 URLs
- URLs get indexed by search engines if mistakenly public

**Consequences:**
- Private media becomes publicly accessible
- Authentication bypassed entirely
- Cannot revoke access without invalidating all URLs and regenerating keys

**Prevention:**
1. **Short TTL:** Use 15 minutes or less for presigned URLs (current code uses 15min - GOOD)
2. **Per-request tokens:** Generate fresh URLs for each access, never cache/share
3. **No direct linking:** Always proxy through API if possible, use presigned only for large files
4. **IP restriction:** Strato S3 may not support this, but AWS does
5. **User-agent tracking:** Log who requested what URL for audit trails

**Detection:**
- Monitor access logs for same URL being accessed multiple times
- Spike in S3 traffic without corresponding API requests
- URLs appearing in referrer headers from external sites

**Phase:** Must be addressed in **Phase 1** (Security foundation)

---

### Pitfall 2: Race Conditions in Cache-Aside Pattern
**What goes wrong:** Multiple concurrent requests for uncached audio cause duplicate S3 downloads and potential cache corruption.

**Why it happens:**
Current code (user_handler.go:551-556) uses goroutine without synchronization:
```go
if cfg.MediaCacheAudio {
    go func() {
        cachePath := filepath.Join(cfg.AudioCachePath, ...)
        _ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
        _ = h.S3Service.DownloadMediaToFile(c, cfg.MediaAudioBucket, asset.Key, cachePath)
    }()
}
```

**Consequences:**
- Multiple users trigger simultaneous downloads of same file
- S3 traffic costs multiplied (Strato charges per GB)
- Disk I/O contention
- Potential partial writes if file is written concurrently

**Prevention:**
1. **Singleflight pattern:** Use golang.org/x/sync/singleflight to deduplicate concurrent downloads
2. **Cache lock:** Per-key locking during download
3. **Cache stampede protection:** First request sets "downloading" flag, others wait or redirect
4. **Pre-warm cache:** Download on asset publish, not first user access

**Detection:**
- Same file downloaded multiple times in quick succession
- High S3 GET requests vs API requests ratio
- Cache directory containing partial files (.*.tmp)

**Phase:** **Phase 2** (Audio streaming optimization)

---

### Pitfall 3: Strato S3 Compatibility Issues
**What goes wrong:** AWS SDK features not supported by Strato S3 cause runtime failures or unexpected behavior.

**Why it happens:**
- Strato S3 implements S3 protocol but may miss newer AWS features
- Different endpoint behavior, especially with virtual-hosted vs path-style
- ACL support may be partial
- Presigned URL generation may differ
- Multipart upload thresholds differ

**Consequences:**
- Uploads fail silently or with cryptic errors
- Presigned URLs don't work
- CORS configuration not respected
- Cannot set proper ACLs (already using ObjectCannedACLPrivate - good)

**Current code vulnerability:**
- Line 58 in s3_service.go: `o.UsePathStyle = pathStyle` - crucial for Strato
- Must verify path-style is ALWAYS true for Strato endpoints

**Prevention:**
1. **Hard-require path-style:** Force path-style for Strato endpoints (configurable)
2. **Test all operations:** Upload, download, presign, list, delete on Strato specifically
3. **Small part size:** Current 10MB is good, but Strato may have lower limits
4. **Fallback mechanisms:** If presigned fails, proxy through API
5. **Feature testing:** Create test script that verifies each S3 operation works

**Detection:**
- Upload succeeds but presigned URL returns 403/404
- Multipart upload hangs at specific file size
- List operations return incomplete results
- "NotImplemented" errors in logs

**Phase:** **Phase 1** (Before any media features go live)

---

### Pitfall 4: Memory Exhaustion from Multi-File Upload
**What goes wrong:** Admin uploads many large files simultaneously, server runs out of memory.

**Why it happens:**
Current code (admin_handler.go:1079) processes uploads synchronously without size pooling:
```go
file, err := c.FormFile("file")
if file.Size > 4*1024*1024*1024 {
    c.JSON(http.StatusBadRequest, gin.H{"error": "audio too large"})
}
```
This checks individual file size but not concurrent/uploads.

**Consequences:**
- Server OOM killed during multi-upload
- Database connection loss
- All in-flight requests fail
- Production downtime

**Prevention:**
1. **Per-admin upload queue:** Limit concurrent uploads per admin
2. **Total size limit:** Check combined size of all uploads in progress
3. **Stream directly:** Don't buffer entire file in memory (current code does stream - good)
4. **Progress tracking:** Show upload progress, allow cancellation
5. **Background uploads:** Accept file, save to temp, upload in background
6. **Rate limiting:** Limit upload request rate per admin

**Detection:**
- Memory usage spikes during uploads
- Upload handler getting OOM killed
- Container restarts during upload operations

**Phase:** **Phase 1** (Multi-file upload feature)

---

### Pitfall 5: Audio Quality Loss from Transcoding
**What goes wrong:** Converting FLAC to lower quality for streaming loses the audiophile value proposition.

**Why it happens:**
- Transcoding to reduce traffic uses low bitrate
- Poor encoder settings
- Wrong codec choice (Opus vs AAC vs MP3)
- Transcoding happens on every request instead of caching

**Consequences:**
- Audiophile users disappointed, core value lost
- Re-transcoding wastes CPU
- Inconsistent quality across tracks
- More S3 traffic (transcoded files not cached)

**Current approach:**
- Code serves FLAC directly from S3 via presigned URL
- No transcoding implemented yet

**Prevention:**
1. **Don't transcode by default:** Serve original if acceptable traffic
2. **Quality tiers:** Store original + quality tiers (320kbps MP3, AAC)
3. **Pre-generate tiers:** On upload, generate all tiers, don't do on-the-fly
4. **User choice:** Let user select quality tier in preferences
5. **Intelligent downsizing:** Only for mobile/detect poor connection

**Detection:**
- User complaints about audio quality
- High CPU usage during audio serving
- Different quality for same file on different plays

**Phase:** **Phase 2** (Audio streaming feature)

---

### Pitfall 6: Orphaned S3 Objects
**What goes wrong:** Deleted assets in database leave files in S3, wasting storage and money.

**Why it happens:**
- Database transaction succeeds but S3 delete fails
- Asset record deleted without deleting S3 object
- Rollback doesn't clean up S3 uploads
- No garbage collection for orphaned objects

**Consequences:**
- Storage costs grow indefinitely
- S3 full of inaccessible files
- Cannot easily identify which files are safe to delete
- Compliance issues (GDPR right to deletion)

**Current code:**
- admin_handler.go:1076-1154 shows upload creates asset record AFTER S3 upload (good order)
- No delete endpoint visible yet

**Prevention:**
1. **Delete first, then DB:** Delete S3 object, then remove DB record
2. **Two-phase delete:** Mark as deleted, clean up S3 async, then remove record
3. **Garbage collection:** Periodic job comparing DB assets to S3 objects
4. **Soft delete:** Don't actually delete, just mark hidden
5. **Tombstone records:** Keep record of deleted S3 keys for cleanup

**Detection:**
- S3 list API returns more objects than DB count
- Storage costs growing despite active cleanup
- Reconciliation script finds differences

**Phase:** **Phase 1** (Delete operations)

---

## Moderate Pitfalls

Issues that cause problems but are recoverable or workable.

### Pitfall 7: Range Request Not Supported by ServeFile
**What goes wrong:** Audio players seeking/skipping cause entire file to re-download.

**Why it happens:**
Current code (storage_service.go:84):
```go
func (s *StorageService) ServeFileWithRange(w http.ResponseWriter, req *http.Request, absPath, downloadName string) error {
    http.ServeFile(w, req, absPath)
    return nil
}
```
`http.ServeFile` DOES support Range requests, but this isn't explicitly verified for Strato S3 presigned URLs.

**Consequences:**
- Users cannot seek in audio tracks
- Re-downloads increase S3 traffic costs
- Poor user experience for long tracks
- Mobile data waste

**Prevention:**
1. **Test range requests:** Verify curl -H "Range: bytes=1000-2000" works
2. **Explicit range handling:** Implement range support if http.ServeFile insufficient
3. **Cache full file locally:** For audio, cache locally first, then serve with range
4. **Proxy range requests:** Intercept range requests and proxy partial content from S3

**Detection:**
- Audio player always starts from beginning on seek
- S3 logs show full-size GET for partial plays
- Range headers appearing in logs but 206 responses missing

**Phase:** **Phase 2** (Audio streaming)

---

### Pitfall 8: MIME Type Sniffing Security
**What goes wrong:** Browser executes malicious content uploaded as image/audio due to wrong Content-Type.

**Why it happens:**
Current code (admin_handler.go:1121):
```go
ctype := mime.TypeByExtension(ext)
```
Trusts file extension only, doesn't validate actual content.

**Consequences:**
- XSS attacks via uploaded HTML/JS files with image extensions
- Content injection
- Security vulnerabilities

**Prevention:**
1. **Detect actual MIME:** Use http.DetectContentType on first 512 bytes
2. **Allowlist types:** Only accept specific MIME types (audio/flac, audio/mpeg, image/jpeg, etc.)
3. **Reject mismatches:** If extension says .jpg but content says HTML, reject
4. **Sanitize filenames:** Prevent directory traversal (already using filepath.Base - good)

**Detection:**
- Files with mismatched extension/content
- Uploaded files executing as scripts
- Virus scanner alerts

**Phase:** **Phase 1** (Upload security)

---

### Pitfall 9: Upload Timeout on Large Files
**What goes wrong:** Large audio uploads timeout before completing, leaving partial state.

**Why it happens:**
- Default HTTP timeout too short for 4GB FLAC files
- Network issues during upload
- No resume capability for multipart uploads
- S3 session timeout

**Consequences:**
- Failed uploads waste bandwidth
- Admin must restart entire upload
- Partial S3 objects wasting storage
- Poor admin UX

**Prevention:**
1. **Long timeout:** Set appropriate timeout for upload handlers
2. **Multipart upload:** Already using AWS SDK uploader with 10MB parts (good!)
3. **Resume capability:** Track upload IDs, allow resume
4. **Progress feedback:** Show upload progress to admin
5. **Background uploads:** Accept file, then upload async with status tracking

**Detection:**
- Upload failures at consistent duration
- Context deadline exceeded errors
- Incomplete files in S3

**Phase:** **Phase 1** (Upload reliability)

---

### Pitfall 10: Cache Stampede on Asset Publish
**What goes wrong:** Admin publishes asset, all users try to access simultaneously, overwhelming S3.

**Why it happens:**
- Asset published with fanfare (email, notification)
- Hundreds of users request same file within minutes
- Cache empty, all requests hit S3
- Presigned URLs generated rapidly

**Consequences:**
- S3 rate limiting or throttling
- High egress costs (Strato charges)
- Slow responses for users
- Possible service degradation

**Prevention:**
1. **Pre-warm cache:** Download to cache on publish, before users access
2. **Staggered release:** Don't notify all users instantly
3. **CDN consideration:** For popular content, consider CDN with private origin
4. **Queue requests:** Limit concurrent S3 requests for same object
5. **Graceful degradation:** Serve lower quality tier during high load

**Detection:**
- S3 request spike after asset publish
- API latency increases
- Cache misses on new assets

**Phase:** **Phase 2** (Cache optimization)

---

## Minor Pitfalls

Annoyances that don't break functionality but should be avoided.

### Pitfall 11: Filename Collisions
**What goes wrong:** Two files with same name overwrite each other or cause confusion.

**Why it happens:**
Current key generation (storage_service.go:32):
```go
func (s *StorageService) BuildObjectKey(kind string, originalName string) string {
    return fmt.Sprintf("%s/%s", kind, originalName)
}
```
Uses original filename only, no uniqueness.

**Prevention:**
1. **Add UUID prefix:** `{uuid}-{filename}` or `{kind}/{uuid}/{filename}`
2. **Check existence:** Return error if key exists
3. **Version suffix:** Add v2, v3 for duplicates
4. **Timestamp prefix:** Add upload timestamp

**Phase:** **Phase 1**

---

### Pitfall 12: Missing Content-Length for Streaming
**What goes wrong:** Audio players can't show duration or progress bar.

**Why it happens:**
Serving via presigned URL doesn't guarantee Content-Length header.

**Prevention:**
1. **Store file size:** Already in asset model (Size field)
2. **Include in response:** Send Content-Length when serving via proxy
3. **HEAD request:** Allow client to query size before download

**Phase:** **Phase 2**

---

### Pitfall 13: No Upload Progress Feedback
**What goes wrong:** Admin uploads 4GB file with no progress indication, appears frozen.

**Prevention:**
1. **WebSocket progress:** Real-time progress updates
2. **Polling endpoint:** GET /upload/status/{id}
3. **Asset status field:** Track upload status (uploading, complete, failed)

**Phase:** **Phase 3** (UX improvement)

---

## Production-Specific Pitfalls

**CRITICAL:** These are amplified by production-only testing constraint.

### Pitfall 14: Breaking Change in Production
**What goes wrong:** Media feature update breaks existing asset serving for all users.

**Why it happens:**
- No local testing means bugs only found in production
- Database migration affects existing asset records
- S3 key format change breaks old assets

**Consequences:**
- ALL media becomes inaccessible
- Emergency rollback needed
- User trust damaged

**Prevention:**
1. **Feature flags:** Gate new features behind config flag
2. **Blue-green deployment:** Keep old version running alongside new
3. **Backward compatibility:** New code must handle old asset format
4. **Migration scripts:** Test migrations on copy of production data first
5. **Rollback plan:** Have instant rollback procedure ready
6. **Canary releases:** Test with small subset of users first

**Phase:** ALL phases - this is ongoing practice

---

### Pitfall 15: Strato Traffic Cost Surprise
**What goes wrong:** Unexpected S3 egress costs from serving media directly.

**Why it happens:**
- Presigned URLs allow direct S3 access, bypassing any caching
- No visibility into per-user consumption
- Cache invalidation too frequent
- Not using Strato's free tier effectively

**Prevention:**
1. **Aggressive caching:** Cache everything locally that can be cached
2. **Serve via proxy:** Control egress, monitor per-user
3. **Cache invalidation strategy:** Don't invalidate unless necessary
4. **Bandwidth limits:** Per-user or per-IP limits on media access
5. **Monitoring:** Track S3 API calls and egress bytes

**Detection:**
- Strato bill higher than expected
- S3 logs show high GET request count
- Cache miss rate high

**Phase:** **Phase 2** (Cost monitoring)

---

## Phase-Specific Warnings

| Phase | Topic | Likely Pitfall | Mitigation |
|-------|-------|----------------|------------|
| Phase 1 | Upload security | MIME type spoofing | Implement content detection |
| Phase 1 | S3 setup | Strato compatibility | Test all operations with Strato |
| Phase 1 | Multi-upload | Memory exhaustion | Add concurrency limits |
| Phase 1 | Delete operations | Orphaned S3 objects | Implement garbage collection |
| Phase 2 | Audio streaming | Cache stampede | Implement singleflight |
| Phase 2 | Range requests | No seeking | Verify and test Range support |
| Phase 2 | Transcoding | Quality loss | Pre-generate quality tiers |
| Phase 2 | Cache warming | Cold start after publish | Download to cache on publish |
| Phase 3 | Analytics | Privacy concerns | Anonymize access logs |
| All | Production safety | Breaking existing assets | Use feature flags, test rollback |

---

## Sources

| Source | Confidence | Notes |
|--------|------------|-------|
| Existing codebase analysis | HIGH | Reviewed s3_service.go, storage_service.go, handlers |
| AWS SDK Go documentation | HIGH | Standard patterns for S3 operations |
| S3 protocol best practices | MEDIUM | General S3 patterns, Strato-specific needs verification |
| Audio streaming patterns | MEDIUM | Best practices, audiophile requirements need validation |

**Verification needed:**
- Strato S3 specific capabilities and limitations (official docs)
- Strato S3 presigned URL behavior
- Strato S3 CORS configuration
- Optimal audio format/quality tiers for audiophile audience
