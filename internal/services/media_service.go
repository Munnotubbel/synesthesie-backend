package services

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type MediaService struct {
	db        *gorm.DB
	cfg       *config.Config
	s3Service *S3Service
}

func NewMediaService(db *gorm.DB, cfg *config.Config, s3Service *S3Service) *MediaService {
	return &MediaService{
		db:        db,
		cfg:       cfg,
		s3Service: s3Service,
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
