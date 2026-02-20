package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/services"
)

// audioMimeTypeFromExt returns the correct MIME type for common audio extensions.
func audioMimeTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".flac":
		return "audio/flac"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	case ".ogg", ".oga":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}

type MusicHandler struct {
	musicService      *services.MusicService
	storageService    *services.StorageService
	audioCacheService *services.AudioCacheService
}

func NewMusicHandler(musicService *services.MusicService, storageService *services.StorageService, audioCacheService *services.AudioCacheService) *MusicHandler {
	return &MusicHandler{
		musicService:      musicService,
		storageService:    storageService,
		audioCacheService: audioCacheService,
	}
}

// TokenFromQueryMiddleware extracts JWT token from query param and sets it as Authorization header
// This allows <audio src="/stream?token=xxx"> to work with standard auth middleware
func TokenFromQueryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no Authorization header but token in query, set it
		if c.GetHeader("Authorization") == "" {
			token := c.Query("token")
			if token != "" {
				c.Request.Header.Set("Authorization", "Bearer "+token)
			}
		}
		c.Next()
	}
}

// getStreamURL returns the proxy stream URL for a music set (with auth)
func (h *MusicHandler) getStreamURL(setID uuid.UUID, isAdmin bool) string {
	if isAdmin {
		return fmt.Sprintf("/api/v1/admin/music-sets/%s/stream", setID)
	}
	return fmt.Sprintf("/api/v1/user/music-sets/%s/stream", setID)
}

// CreateMusicSet handles music set creation
// POST /admin/music-sets
// Body: {"title": "...", "description": "..."}
func (h *MusicHandler) CreateMusicSet(c *gin.Context) {
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	musicSet, err := h.musicService.CreateMusicSet(req.Title, req.Description)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          musicSet.ID,
		"title":       musicSet.Title,
		"description": musicSet.Description,
		"visibility":  musicSet.Visibility,
		"has_file":    false,
		"created_at":  musicSet.CreatedAt,
	})
}

// UploadMusicSetFile handles audio file upload for a music set
// POST /admin/music-sets/:id/upload
// Multipart form: file (required)
func (h *MusicHandler) UploadMusicSetFile(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	// Parse multipart form with large limit for audio files (500MB)
	maxMemory := int64(500 * 1024 * 1024)
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form: " + err.Error()})
		return
	}

	// Get file from form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	// Read file content
	data := make([]byte, 0)
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := file.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Upload via MusicService
	musicSet, err := h.musicService.UploadMusicSetFile(c.Request.Context(), setID, header.Filename, data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate S3 presigned URL (direct streaming, no auth needed)
	streamURL := ""
	if musicSet.Asset != nil {
		streamURL = h.getStreamURL(musicSet.ID, true)
	}

	response := gin.H{
		"id":          musicSet.ID,
		"title":       musicSet.Title,
		"description": musicSet.Description,
		"visibility":  musicSet.Visibility,
		"has_file":    musicSet.AssetID != nil,
		"created_at":  musicSet.CreatedAt,
	}

	if musicSet.Asset != nil {
		response["filename"] = musicSet.Asset.Filename
		response["mime_type"] = musicSet.Asset.MimeType
		response["size_bytes"] = musicSet.Asset.SizeBytes
		response["presigned_url"] = streamURL // S3 presigned URL
	}

	c.JSON(http.StatusOK, response)
}

// GetAllMusicSets lists all music sets for admin
// GET /admin/music-sets?page=1&limit=20
func (h *MusicHandler) GetAllMusicSets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	sets, total, err := h.musicService.GetAllMusicSets(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve music sets"})
		return
	}

	// Build response
	setList := make([]gin.H, len(sets))
	for i, set := range sets {
		setData := gin.H{
			"id":          set.ID,
			"title":       set.Title,
			"description": set.Description,
			"visibility":  set.Visibility,
			"has_file":    set.AssetID != nil,
			"duration":    set.Duration,
			"created_at":  set.CreatedAt,
			"updated_at":  set.UpdatedAt,
		}
		if set.Asset != nil {
			setData["filename"] = set.Asset.Filename
			setData["mime_type"] = set.Asset.MimeType
			setData["size_bytes"] = set.Asset.SizeBytes
		}
		setList[i] = setData
	}

	c.JSON(http.StatusOK, gin.H{
		"music_sets": setList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetMusicSetDetails gets single music set details with stream URL
// GET /admin/music-sets/:id
func (h *MusicHandler) GetMusicSetDetails(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	musicSet, err := h.musicService.GetMusicSetByID(setID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "music set not found"})
		return
	}

	// Generate S3 presigned URL (direct streaming, no auth needed)
	streamURL := ""
	if musicSet.Asset != nil {
		streamURL = h.getStreamURL(musicSet.ID, true)
	}

	response := gin.H{
		"id":            musicSet.ID,
		"title":         musicSet.Title,
		"description":   musicSet.Description,
		"visibility":    musicSet.Visibility,
		"has_file":      musicSet.AssetID != nil,
		"duration":      musicSet.Duration,
		"presigned_url": streamURL, // S3 presigned URL
		"created_at":    musicSet.CreatedAt,
		"updated_at":    musicSet.UpdatedAt,
	}

	if musicSet.Asset != nil {
		response["filename"] = musicSet.Asset.Filename
		response["mime_type"] = musicSet.Asset.MimeType
		response["size_bytes"] = musicSet.Asset.SizeBytes
	}

	c.JSON(http.StatusOK, response)
}

// DeleteMusicSet handles music set deletion
// DELETE /admin/music-sets/:id
func (h *MusicHandler) DeleteMusicSet(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	if err := h.musicService.DeleteMusicSet(c.Request.Context(), setID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "music set not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete music set"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "music set deleted successfully",
		"id":      setID,
	})
}

// UpdateMusicSetVisibility handles visibility changes
// PUT /admin/music-sets/:id/visibility
func (h *MusicHandler) UpdateMusicSetVisibility(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	var req struct {
		Visibility string `json:"visibility" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "visibility is required"})
		return
	}

	if err := h.musicService.UpdateMusicSetVisibility(setID, req.Visibility); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "visibility updated successfully", "visibility": req.Visibility})
}

// UpdateMusicSetMetadata updates title and description
// PUT /admin/music-sets/:id
func (h *MusicHandler) UpdateMusicSetMetadata(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.musicService.UpdateMusicSetMetadata(setID, req.Title, req.Description); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "music set updated successfully"})
}

// GetPublicMusicSets returns all public music sets for users
// GET /user/music-sets
func (h *MusicHandler) GetPublicMusicSets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	sets, total, err := h.musicService.GetPublicMusicSets(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve music sets"})
		return
	}

	// Build response with S3 presigned URLs
	setList := make([]gin.H, len(sets))
	for i, set := range sets {
		streamURL := ""
		if set.Asset != nil {
			streamURL = h.getStreamURL(set.ID, false)
		}
		setList[i] = gin.H{
			"id":            set.ID,
			"title":         set.Title,
			"description":   set.Description,
			"duration":      set.Duration,
			"presigned_url": streamURL, // S3 presigned URL
			"created_at":    set.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"music_sets": setList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetPublicMusicSet returns a single public music set
// GET /user/music-sets/:id
func (h *MusicHandler) GetPublicMusicSet(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	musicSet, err := h.musicService.GetMusicSetByID(setID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "music set not found"})
		return
	}

	// Check visibility
	if musicSet.Visibility != "public" {
		c.JSON(http.StatusNotFound, gin.H{"error": "music set not found"})
		return
	}

	// Generate S3 presigned URL (direct streaming, no auth needed)
	streamURL := ""
	if musicSet.Asset != nil {
		streamURL = h.getStreamURL(musicSet.ID, false)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            musicSet.ID,
		"title":         musicSet.Title,
		"description":   musicSet.Description,
		"duration":      musicSet.Duration,
		"presigned_url": streamURL, // S3 presigned URL
		"created_at":    musicSet.CreatedAt,
	})
}

// StreamMusicSet streams audio file for admin (with caching, Range support)
// GET /admin/music-sets/:id/stream?token=xxx
func (h *MusicHandler) StreamMusicSetAdmin(c *gin.Context) {
	h.streamMusicSet(c, false)
}

// StreamMusicSet streams audio file for user (with caching, Range support)
// GET /user/music-sets/:id/stream?token=xxx
func (h *MusicHandler) StreamMusicSetUser(c *gin.Context) {
	h.streamMusicSet(c, true)
}

// streamMusicSet handles the actual streaming with local caching
// Auth is ALWAYS checked (secure), files are cached locally for speed
func (h *MusicHandler) streamMusicSet(c *gin.Context, checkVisibility bool) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		log.Printf("[Stream] Invalid music set ID: %s", setIDStr)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	log.Printf("[Stream] Looking up music set %s", setID)
	musicSet, err := h.musicService.GetMusicSetByID(setID)
	if err != nil {
		log.Printf("[Stream] Music set not found: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "music set not found"})
		return
	}

	// Check visibility for user endpoint
	if checkVisibility && musicSet.Visibility != "public" {
		log.Printf("[Stream] Music set %s not public (visibility: %s)", setID, musicSet.Visibility)
		c.JSON(http.StatusNotFound, gin.H{"error": "music set not found"})
		return
	}

	// Check if set has a file
	if musicSet.Asset == nil {
		log.Printf("[Stream] Music set %s has no asset", setID)
		c.JSON(http.StatusNotFound, gin.H{"error": "no audio file uploaded"})
		return
	}

	log.Printf("[Stream] Music set %s has asset: key=%s, mime=%s", setID, musicSet.Asset.Key, musicSet.Asset.MimeType)

	// Check if audioCacheService is initialized
	if h.audioCacheService == nil {
		log.Printf("[Stream] ERROR: audioCacheService is nil!")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "audio cache service not initialized"})
		return
	}

	// Resolve Content-Type: trust the stored MimeType, re-derive from extension as fallback
	contentType := musicSet.Asset.MimeType
	if contentType == "" || contentType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(musicSet.Asset.Filename))
		contentType = audioMimeTypeFromExt(ext)
	}

	// Set headers for audio streaming
	c.Header("Content-Type", contentType)
	c.Header("Accept-Ranges", "bytes")
	c.Header("Cache-Control", "public, max-age=31536000") // 1 year cache
	c.Header("Content-Disposition", "inline; filename=\""+musicSet.Asset.Filename+"\"")

	localPath := h.audioCacheService.GetLocalPath(musicSet.Asset.Key)
	log.Printf("[Stream] Local path for %s: %s", musicSet.Asset.Key, localPath)

	// Fast path: serve from local cache (Range-aware, handles seek for waveform)
	if h.audioCacheService.IsCached(musicSet.Asset.Key) {
		log.Printf("[Stream] Serving from cache: %s", localPath)
		http.ServeFile(c.Writer, c.Request, localPath)
		return
	}

	// Cache miss: stream directly from S3 (no blocking download).
	// Simultaneously kick off a background download so future requests use the cache.
	log.Printf("[Stream] Cache miss — proxying from S3, warming cache in background: %s", musicSet.Asset.Key)
	h.audioCacheService.StartBackgroundDownload(context.Background(), musicSet.Asset.Key)

	s3Stream, streamErr := h.audioCacheService.StreamFromS3(c.Request.Context(), musicSet.Asset.Key)
	if streamErr != nil {
		log.Printf("[Stream] ERROR streaming from S3: %v", streamErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "audio unavailable: " + streamErr.Error()})
		return
	}
	defer s3Stream.Close()

	c.Writer.WriteHeader(http.StatusOK)
	io.Copy(c.Writer, s3Stream) //nolint:errcheck
}
