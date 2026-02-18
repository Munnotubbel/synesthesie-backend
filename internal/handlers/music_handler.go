package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/services"
)

type MusicHandler struct {
	musicService *services.MusicService
}

func NewMusicHandler(musicService *services.MusicService) *MusicHandler {
	return &MusicHandler{
		musicService: musicService,
	}
}

// CreateMusicSet handles music set creation (MSC-ADM-01)
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
		"created_at":  musicSet.CreatedAt,
	})
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

	// Build response with track counts
	setList := make([]gin.H, len(sets))
	for i, set := range sets {
		trackCount := len(set.Tracks)
		setList[i] = gin.H{
			"id":          set.ID,
			"title":       set.Title,
			"description": set.Description,
			"visibility":  set.Visibility,
			"track_count": trackCount,
			"created_at":  set.CreatedAt,
			"updated_at":  set.UpdatedAt,
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

// GetMusicSetDetails gets single music set details with tracks
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

	// Build tracks response
	tracks := make([]gin.H, len(musicSet.Tracks))
	for i, track := range musicSet.Tracks {
		trackData := gin.H{
			"id":          track.ID,
			"title":       track.Title,
			"artist":      track.Artist,
			"track_order": track.TrackOrder,
			"duration":    track.Duration,
			"created_at":  track.CreatedAt,
		}
		if track.Asset != nil {
			trackData["filename"] = track.Asset.Filename
			trackData["mime_type"] = track.Asset.MimeType
			trackData["size_bytes"] = track.Asset.SizeBytes
		}
		tracks[i] = trackData
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          musicSet.ID,
		"title":       musicSet.Title,
		"description": musicSet.Description,
		"visibility":  musicSet.Visibility,
		"tracks":      tracks,
		"created_at":  musicSet.CreatedAt,
		"updated_at":  musicSet.UpdatedAt,
	})
}

// DeleteMusicSet handles music set deletion (MSC-ADM-04)
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

// UpdateMusicSetVisibility handles visibility changes (MSC-ADM-05)
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

// UploadTrack handles track upload to a music set (MSC-ADM-02)
// POST /admin/music-sets/:id/tracks
func (h *MusicHandler) UploadTrack(c *gin.Context) {
	setIDStr := c.Param("id")
	setID, err := uuid.Parse(setIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid music set ID"})
		return
	}

	// Get file from form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	// Get optional metadata
	title := c.PostForm("title")
	artist := c.PostForm("artist")

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
	track, err := h.musicService.UploadTrack(c.Request.Context(), setID, header.Filename, data, title, artist)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          track.ID,
		"music_set_id": track.MusicSetID,
		"title":       track.Title,
		"artist":      track.Artist,
		"track_order": track.TrackOrder,
		"created_at":  track.CreatedAt,
	})
}

// DeleteTrack handles track deletion (MSC-ADM-03)
// DELETE /admin/tracks/:id
func (h *MusicHandler) DeleteTrack(c *gin.Context) {
	trackIDStr := c.Param("id")
	trackID, err := uuid.Parse(trackIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid track ID"})
		return
	}

	if err := h.musicService.DeleteTrack(c.Request.Context(), trackID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "track not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete track"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "track deleted successfully"})
}

// UpdateTrackMetadata updates track title and artist (MSC-ADM-06)
// PUT /admin/tracks/:id
func (h *MusicHandler) UpdateTrackMetadata(c *gin.Context) {
	trackIDStr := c.Param("id")
	trackID, err := uuid.Parse(trackIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid track ID"})
		return
	}

	var req struct {
		Title  string `json:"title"`
		Artist string `json:"artist"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.musicService.UpdateTrackMetadata(trackID, req.Title, req.Artist); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "track updated successfully"})
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

	// Build response with presigned URLs for tracks
	setList := make([]gin.H, len(sets))
	for i, set := range sets {
		tracks := make([]gin.H, len(set.Tracks))
		for j, track := range set.Tracks {
			presignedURL := ""
			if track.Asset != nil {
				presignedURL, _ = h.musicService.GetPresignedTrackURL(c.Request.Context(), track.Asset.Key)
			}
			tracks[j] = gin.H{
				"id":           track.ID,
				"title":        track.Title,
				"artist":       track.Artist,
				"track_order":  track.TrackOrder,
				"duration":     track.Duration,
				"presigned_url": presignedURL,
			}
		}
		setList[i] = gin.H{
			"id":          set.ID,
			"title":       set.Title,
			"description": set.Description,
			"tracks":      tracks,
			"created_at":  set.CreatedAt,
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

	// Build tracks with presigned URLs
	tracks := make([]gin.H, len(musicSet.Tracks))
	for i, track := range musicSet.Tracks {
		presignedURL := ""
		if track.Asset != nil {
			presignedURL, _ = h.musicService.GetPresignedTrackURL(c.Request.Context(), track.Asset.Key)
		}
		tracks[i] = gin.H{
			"id":            track.ID,
			"title":         track.Title,
			"artist":        track.Artist,
			"track_order":   track.TrackOrder,
			"duration":      track.Duration,
			"presigned_url": presignedURL,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          musicSet.ID,
		"title":       musicSet.Title,
		"description": musicSet.Description,
		"tracks":      tracks,
		"created_at":  musicSet.CreatedAt,
	})
}
