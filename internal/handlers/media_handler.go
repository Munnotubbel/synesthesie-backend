package handlers

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/services"
)

type MediaHandler struct {
	mediaService   *services.MediaService
	storageService *services.StorageService
}

func NewMediaHandler(mediaService *services.MediaService, storageService *services.StorageService) *MediaHandler {
	return &MediaHandler{
		mediaService:   mediaService,
		storageService: storageService,
	}
}

// UploadImage handles single image upload (IMG-ADM-01)
// POST /admin/images
// Multipart form: file (required), title (optional), description (optional)
func (h *MediaHandler) UploadImage(c *gin.Context) {
	// Get file from form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	// Read file content for MIME detection and upload
	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file"})
		return
	}

	// Get optional metadata
	title := c.PostForm("title")
	description := c.PostForm("description")

	// Upload via MediaService (handles MIME validation internally)
	image, err := h.mediaService.UploadImage(c.Request.Context(), header.Filename, data, title, description)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          image.ID,
		"asset_id":    image.AssetID,
		"title":       image.Title,
		"description": image.Description,
		"visibility":  image.Visibility,
		"filename":    image.Asset.Filename,
		"mime_type":   image.Asset.MimeType,
		"size_bytes":  image.Asset.SizeBytes,
		"created_at":  image.CreatedAt,
	})
}

// UploadImages handles multiple image upload (IMG-ADM-02)
// POST /admin/images/batch
// Multipart form: files[] (multiple files), default_title (optional), default_description (optional)
func (h *MediaHandler) UploadImages(c *gin.Context) {
	// Parse multipart form with larger memory limit for multiple files
	maxMemory := int64(50 * 1024 * 1024) // 50MB total
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form"})
		return
	}

	form := c.Request.MultipartForm
	files, ok := form.File["files[]"]
	if !ok || len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "files[] is required"})
		return
	}

	// Limit concurrent uploads to prevent memory exhaustion
	maxConcurrent := 3 // matches config default
	if len(files) > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maximum 10 files per batch"})
		return
	}

	// Get optional default metadata
	defaultTitle := c.PostForm("default_title")
	defaultDescription := c.PostForm("default_description")

	// Process uploads with concurrency limit
	type uploadResult struct {
		ID     uuid.UUID `json:"id"`
		Status string    `json:"status"`
		Error  string    `json:"error,omitempty"`
	}
	results := make([]uploadResult, len(files))

	// Simple semaphore for concurrency limiting
	sem := make(chan struct{}, maxConcurrent)
	done := make(chan int, len(files))

	for i, fileHeader := range files {
		go func(idx int, fh *multipart.FileHeader) {
			sem <- struct{}{} // acquire
			defer func() { <-sem; done <- idx }()

			file, err := fh.Open()
			if err != nil {
				results[idx] = uploadResult{Status: "error", Error: "failed to open file"}
				return
			}
			defer file.Close()

			data, err := io.ReadAll(file)
			if err != nil {
				results[idx] = uploadResult{Status: "error", Error: "failed to read file"}
				return
			}

			image, err := h.mediaService.UploadImage(c.Request.Context(), fh.Filename, data, defaultTitle, defaultDescription)
			if err != nil {
				results[idx] = uploadResult{Status: "error", Error: err.Error()}
				return
			}

			results[idx] = uploadResult{ID: image.ID, Status: "success"}
		}(i, fileHeader)
	}

	// Wait for all goroutines
	for range files {
		<-done
	}

	// Count successes and failures
	success := 0
	failed := 0
	for _, r := range results {
		if r.Status == "success" {
			success++
		} else {
			failed++
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "batch upload complete",
		"total":   len(files),
		"success": success,
		"failed":  failed,
		"results": results,
	})
}

// GetAllImages lists all images for admin
// GET /admin/images?page=1&limit=20
func (h *MediaHandler) GetAllImages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	images, total, err := h.mediaService.GetAllImages(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve images"})
		return
	}

	// Build response with proxy URLs (faster than S3 presigned URLs)
	imageList := make([]gin.H, len(images))
	for i, img := range images {
		fileURL := fmt.Sprintf("/api/v1/admin/images/%s/file", img.ID)
		imageList[i] = gin.H{
			"id":            img.ID,
			"asset_id":      img.AssetID,
			"title":         img.Title,
			"description":   img.Description,
			"visibility":    img.Visibility,
			"filename":      img.Asset.Filename,
			"mime_type":     img.Asset.MimeType,
			"size_bytes":    img.Asset.SizeBytes,
			"presigned_url": fileURL, // Proxy URL for compatibility (faster than S3)
			"file_url":      fileURL,
			"created_at":    img.CreatedAt,
			"updated_at":    img.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"images": imageList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetImageDetails gets single image details for admin
// GET /admin/images/:id
func (h *MediaHandler) GetImageDetails(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	image, err := h.mediaService.GetImageByID(imageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	// Generate proxy URL (faster than S3 presigned URL)
	fileURL := fmt.Sprintf("/api/v1/admin/images/%s/file", image.ID)

	c.JSON(http.StatusOK, gin.H{
		"id":            image.ID,
		"asset_id":      image.AssetID,
		"title":         image.Title,
		"description":   image.Description,
		"visibility":    image.Visibility,
		"filename":      image.Asset.Filename,
		"mime_type":     image.Asset.MimeType,
		"size_bytes":    image.Asset.SizeBytes,
		"presigned_url": fileURL, // Proxy URL for compatibility (faster than S3)
		"created_at":    image.CreatedAt,
		"updated_at":    image.UpdatedAt,
	})
}

// DeleteImage handles image deletion (IMG-ADM-03)
// DELETE /admin/images/:id
func (h *MediaHandler) DeleteImage(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	if err := h.mediaService.DeleteImage(c.Request.Context(), imageID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "image deleted successfully"})
}

// UpdateImageVisibility changes image visibility (IMG-ADM-04)
// PUT /admin/images/:id/visibility
func (h *MediaHandler) UpdateImageVisibility(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	var req struct {
		Visibility string `json:"visibility" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "visibility is required"})
		return
	}

	if err := h.mediaService.UpdateImageVisibility(imageID, req.Visibility); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "visibility updated successfully", "visibility": req.Visibility})
}

// UpdateImageMetadata updates image title and description (IMG-ADM-05)
// PUT /admin/images/:id/metadata
func (h *MediaHandler) UpdateImageMetadata(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
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

	if err := h.mediaService.UpdateImageMetadata(imageID, req.Title, req.Description); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "metadata updated successfully"})
}

// GetPublicImages returns all public images for users (IMG-USR-01)
// GET /user/images
func (h *MediaHandler) GetPublicImages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	images, total, err := h.mediaService.GetPublicImages(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve images"})
		return
	}

	// Build response with proxy URLs (faster than S3 presigned URLs)
	imageList := make([]gin.H, len(images))
	for i, img := range images {
		fileURL := fmt.Sprintf("/api/v1/user/images/%s/file", img.ID)
		imageList[i] = gin.H{
			"id":            img.ID,
			"title":         img.Title,
			"description":   img.Description,
			"presigned_url": fileURL, // Proxy URL for compatibility (faster than S3)
			"created_at":    img.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"images": imageList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetPublicImage returns a single public image with presigned URL (IMG-USR-02)
// GET /user/images/:id
func (h *MediaHandler) GetPublicImage(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	image, err := h.mediaService.GetImageByID(imageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	// Check visibility
	if image.Visibility != "public" {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	// Generate proxy URL (faster than S3 presigned URL)
	fileURL := fmt.Sprintf("/api/v1/user/images/%s/file", image.ID)

	c.JSON(http.StatusOK, gin.H{
		"id":            image.ID,
		"title":         image.Title,
		"description":   image.Description,
		"presigned_url": fileURL, // Proxy URL for compatibility (faster than S3)
		"created_at":    image.CreatedAt,
	})
}

// ServeImageFile serves the actual image file from local cache
// GET /user/images/:id/file
// This endpoint is faster than presigned URLs because it serves from local disk
// Prefers WebP version if available (auto-converted by background worker)
func (h *MediaHandler) ServeImageFile(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	image, err := h.mediaService.GetImageByID(imageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	// Check visibility - only public images can be served
	if image.Visibility != "public" {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	// Get local file path (downloads from S3 if not present, prefers WebP)
	localPath, err := h.mediaService.GetLocalImagePath(c.Request.Context(), image.Asset.Key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve image"})
		return
	}

	// Determine content type based on actual file (may be WebP)
	contentType := services.GetImageContentType(localPath)

	// Set content type and serve file
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=31536000") // Cache for 1 year
	c.Header("Content-Disposition", "inline; filename=\""+image.Asset.Filename+"\"")
	c.File(localPath)
}

// ServeImageFileAdmin serves any image file for admin (including private)
// GET /admin/images/:id/file
func (h *MediaHandler) ServeImageFileAdmin(c *gin.Context) {
	imageIDStr := c.Param("id")
	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		log.Printf("[ImageAdmin] Invalid image ID: %s", imageIDStr)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	log.Printf("[ImageAdmin] Loading image %s", imageID)
	image, err := h.mediaService.GetImageByID(imageID)
	if err != nil {
		log.Printf("[ImageAdmin] Image not found: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	// Check if asset exists
	if image.Asset == nil {
		log.Printf("[ImageAdmin] Image %s has no asset", imageID)
		c.JSON(http.StatusNotFound, gin.H{"error": "image has no file"})
		return
	}

	log.Printf("[ImageAdmin] Image %s has asset: key=%s", imageID, image.Asset.Key)

	// Get local file path (downloads from S3 if not present, prefers WebP)
	localPath, err := h.mediaService.GetLocalImagePath(c.Request.Context(), image.Asset.Key)
	if err != nil {
		log.Printf("[ImageAdmin] Failed to get local path for %s: %v", image.Asset.Key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve image: " + err.Error()})
		return
	}

	log.Printf("[ImageAdmin] Serving image %s from: %s", imageID, localPath)

	// Determine content type based on actual file (may be WebP)
	contentType := services.GetImageContentType(localPath)

	// Set content type and serve file
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, max-age=3600") // Cache for 1 hour (admin)
	c.Header("Content-Disposition", "inline; filename=\""+image.Asset.Filename+"\"")
	c.File(localPath)
}
