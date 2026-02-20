package services

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type MediaService struct {
	db             *gorm.DB
	cfg            *config.Config
	s3Service      *S3Service
	storageService *StorageService
}

func NewMediaService(db *gorm.DB, cfg *config.Config, s3Service *S3Service, storageService *StorageService) *MediaService {
	return &MediaService{
		db:             db,
		cfg:            cfg,
		s3Service:      s3Service,
		storageService: storageService,
	}
}

// UploadImage uploads an image to S3 and creates DB records
// Returns the created Image and Asset, or error
func (s *MediaService) UploadImage(ctx context.Context, filename string, data []byte, title, description string) (*models.Image, error) {
	// Validate MIME type using content detection (IMG-ADM-06)
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, fmt.Errorf("invalid content type: expected image, got %s", mimeType)
	}

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(filename))
	allowedExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	if !allowedExts[ext] {
		return nil, fmt.Errorf("unsupported image extension: %s", ext)
	}

	// Validate size
	if int64(len(data)) > s.cfg.UploadMaxImageSize {
		return nil, fmt.Errorf("image too large: %d bytes (max: %d)", len(data), s.cfg.UploadMaxImageSize)
	}

	// Generate S3 key
	key := fmt.Sprintf("images/%s%s", uuid.New().String(), ext)

	// Upload to S3 (mediaClient -> MediaImagesBucket)
	if err := s.s3Service.UploadMedia(ctx, s.cfg.MediaImagesBucket, key, bytes.NewReader(data), mimeType); err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Also save to local cache for fast serving
	if s.storageService != nil {
		if _, _, _, err := s.storageService.SaveStream(ctx, key, bytes.NewReader(data)); err != nil {
			// Log but don't fail - S3 is the source of truth
			fmt.Printf("Warning: failed to cache image locally: %v\n", err)
		}
	}

	// Create Asset record
	asset := &models.Asset{
		Key:        key,
		Filename:   filename,
		MimeType:   mimeType,
		SizeBytes:  int64(len(data)),
		Visibility: models.AssetVisibilityPrivate, // Always private - images use Image.Visibility
	}
	if err := s.db.Create(asset).Error; err != nil {
		// Attempt S3 cleanup on DB failure (SEC-04)
		_ = s.DeleteS3Object(ctx, key)
		return nil, fmt.Errorf("failed to create asset record: %w", err)
	}

	// Create Image record
	image := &models.Image{
		AssetID:     asset.ID,
		Title:       title,
		Description: description,
		Visibility:  "private", // Default to private
	}
	if err := s.db.Create(image).Error; err != nil {
		// Cleanup: delete asset and S3 object
		s.db.Delete(asset)
		_ = s.DeleteS3Object(ctx, key)
		return nil, fmt.Errorf("failed to create image record: %w", err)
	}

	// Load asset relation
	image.Asset = asset

	return image, nil
}

// DeleteImage deletes an image (S3 + DB) - IMG-ADM-03
// Delete S3 first to avoid orphaned objects (SEC-04)
func (s *MediaService) DeleteImage(ctx context.Context, imageID uuid.UUID) error {
	var image models.Image
	if err := s.db.Preload("Asset").First(&image, "id = ?", imageID).Error; err != nil {
		return fmt.Errorf("image not found: %w", err)
	}

	// Delete from S3 FIRST (SEC-04: avoid orphaned S3 objects)
	if err := s.DeleteS3Object(ctx, image.Asset.Key); err != nil {
		// Log but continue - S3 might already be gone
	}

	// Delete DB records (Image first, then Asset)
	if err := s.db.Delete(&image).Error; err != nil {
		return fmt.Errorf("failed to delete image record: %w", err)
	}
	if err := s.db.Delete(&image.Asset).Error; err != nil {
		return fmt.Errorf("failed to delete asset record: %w", err)
	}

	return nil
}

// DeleteS3Object deletes an object from the media images bucket
func (s *MediaService) DeleteS3Object(ctx context.Context, key string) error {
	return s.s3Service.DeleteMedia(ctx, s.cfg.MediaImagesBucket, key)
}

// GetPresignedImageURL generates a presigned URL for an image (SEC-01)
func (s *MediaService) GetPresignedImageURL(ctx context.Context, key string) (string, error) {
	ttl := time.Duration(s.cfg.PresignedURLTTLMinutes) * time.Minute
	return s.s3Service.PresignMediaGet(ctx, s.cfg.MediaImagesBucket, key, ttl)
}

// UpdateImageVisibility changes the visibility of an image (IMG-ADM-04)
func (s *MediaService) UpdateImageVisibility(imageID uuid.UUID, visibility string) error {
	if visibility != "private" && visibility != "public" {
		return fmt.Errorf("invalid visibility: must be 'private' or 'public'")
	}
	return s.db.Model(&models.Image{}).Where("id = ?", imageID).Update("visibility", visibility).Error
}

// UpdateImageMetadata updates title and description (IMG-ADM-05)
func (s *MediaService) UpdateImageMetadata(imageID uuid.UUID, title, description string) error {
	updates := map[string]interface{}{}
	if title != "" {
		updates["title"] = title
	}
	if description != "" {
		updates["description"] = description
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&models.Image{}).Where("id = ?", imageID).Updates(updates).Error
}

// GetPublicImages returns all public images for user viewing (IMG-USR-01)
func (s *MediaService) GetPublicImages(limit, offset int) ([]models.Image, int64, error) {
	var images []models.Image
	var total int64

	query := s.db.Model(&models.Image{}).Where("visibility = ?", "public")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Asset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&images).Error; err != nil {
		return nil, 0, err
	}

	return images, total, nil
}

// GetImageByID returns a single image by ID
func (s *MediaService) GetImageByID(imageID uuid.UUID) (*models.Image, error) {
	var image models.Image
	if err := s.db.Preload("Asset").First(&image, "id = ?", imageID).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

// GetAllImages returns all images for admin (with pagination)
func (s *MediaService) GetAllImages(limit, offset int) ([]models.Image, int64, error) {
	var images []models.Image
	var total int64

	if err := s.db.Model(&models.Image{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := s.db.Preload("Asset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&images).Error; err != nil {
		return nil, 0, err
	}

	return images, total, nil
}

// GetLocalImagePath returns the local file path for an image
// Downloads from S3 if not present locally
// Prefers WebP version if available
func (s *MediaService) GetLocalImagePath(ctx context.Context, key string) (string, error) {
	localPath := filepath.Join(s.cfg.LocalAssetsPath, filepath.FromSlash(key))
	webpPath := strings.TrimSuffix(localPath, filepath.Ext(localPath)) + ".webp"

	// Check if WebP version exists (preferred)
	if _, err := os.Stat(webpPath); err == nil {
		return webpPath, nil
	}

	// Check if original exists locally
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	// Not local - download from S3
	if s.storageService == nil {
		return "", fmt.Errorf("image not available locally and no storage service configured")
	}

	data, err := s.s3Service.DownloadMedia(ctx, s.cfg.MediaImagesBucket, key)
	if err != nil {
		return "", fmt.Errorf("failed to download from S3: %w", err)
	}

	// Save to local cache
	absPath, _, _, err := s.storageService.SaveStream(ctx, key, bytes.NewReader(data.Bytes()))
	if err != nil {
		return "", fmt.Errorf("failed to cache image locally: %w", err)
	}

	return absPath, nil
}

// ConvertToWebP converts an image to WebP format using cwebp CLI tool
// Returns the path to the WebP file, or error
func (s *MediaService) ConvertToWebP(originalPath string) (string, error) {
	// Skip if already WebP
	if strings.HasSuffix(strings.ToLower(originalPath), ".webp") {
		return originalPath, nil
	}

	// Check if WebP already exists
	webpPath := strings.TrimSuffix(originalPath, filepath.Ext(originalPath)) + ".webp"
	if _, err := os.Stat(webpPath); err == nil {
		return webpPath, nil
	}

	// Use cwebp CLI tool for conversion (no CGO required)
	// -q 90 = 90% quality
	// -quiet = suppress output
	cmd := exec.Command("cwebp", "-q", "90", "-quiet", originalPath, "-o", webpPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cwebp conversion failed: %w, output: %s", err, string(output))
	}

	// Verify the file was created
	if _, err := os.Stat(webpPath); err != nil {
		return "", fmt.Errorf("webp file not created: %w", err)
	}

	return webpPath, nil
}

// GetPendingWebPConversions returns images that need WebP conversion
// Returns original paths that don't have a corresponding .webp file
func (s *MediaService) GetPendingWebPConversions() ([]string, error) {
	imagesPath := filepath.Join(s.cfg.LocalAssetsPath, "images")

	var pending []string
	err := filepath.Walk(imagesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		// Check for convertible formats
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			webpPath := strings.TrimSuffix(path, ext) + ".webp"
			if _, err := os.Stat(webpPath); os.IsNotExist(err) {
				pending = append(pending, path)
			}
		}
		return nil
	})

	return pending, err
}

// GetImageContentType returns the content type based on file extension
func GetImageContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}
