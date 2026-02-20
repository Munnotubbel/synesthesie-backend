package services

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type MusicService struct {
	db             *gorm.DB
	cfg            *config.Config
	s3Service      *S3Service
	storageService *StorageService
}

func NewMusicService(db *gorm.DB, cfg *config.Config, s3Service *S3Service, storageService *StorageService) *MusicService {
	return &MusicService{
		db:             db,
		cfg:            cfg,
		s3Service:      s3Service,
		storageService: storageService,
	}
}

// CreateMusicSet creates a new music set (without file, use UploadMusicSetFile later)
func (s *MusicService) CreateMusicSet(title, description string) (*models.MusicSet, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	musicSet := &models.MusicSet{
		Title:       title,
		Description: description,
		Visibility:  "private", // Default to private
	}

	if err := s.db.Create(musicSet).Error; err != nil {
		return nil, fmt.Errorf("failed to create music set: %w", err)
	}

	return musicSet, nil
}

// UploadMusicSetFile uploads the audio file for a music set
// This replaces any existing file
func (s *MusicService) UploadMusicSetFile(ctx context.Context, setID uuid.UUID, filename string, data []byte) (*models.MusicSet, error) {
	// Verify music set exists
	var musicSet models.MusicSet
	if err := s.db.First(&musicSet, "id = ?", setID).Error; err != nil {
		return nil, fmt.Errorf("music set not found: %w", err)
	}

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(filename))
	allowedExts := map[string]bool{
		".flac": true,
		".mp3":  true,
		".wav":  true,
		".aac":  true,
		".m4a":  true,
		".ogg":  true,
		".oga":  true,
	}
	if !allowedExts[ext] {
		return nil, fmt.Errorf("unsupported audio format: %s (allowed: flac, mp3, wav, aac, m4a, ogg)", ext)
	}

	// Detect MIME type: always prefer extension (content sniffing misidentifies FLAC as audio/mpeg)
	mimeType := getMimeTypeFromExtension(ext)
	if mimeType == "application/octet-stream" {
		// Extension unknown — try content sniffing
		sniffed := http.DetectContentType(data)
		if strings.HasPrefix(sniffed, "audio/") {
			mimeType = sniffed
		}
	}

	// Generate S3 key
	key := fmt.Sprintf("music/%s%s", uuid.New().String(), ext)

	// Upload to S3 (audio bucket)
	if err := s.s3Service.UploadMedia(ctx, s.cfg.MediaAudioBucket, key, data, mimeType); err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// If set already has an asset, delete the old one
	if musicSet.AssetID != nil {
		var oldAsset models.Asset
		if err := s.db.First(&oldAsset, "id = ?", *musicSet.AssetID).Error; err == nil {
			// Delete old S3 object
			_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, oldAsset.Key)
			// Delete old asset record
			s.db.Delete(&oldAsset)
		}
	}

	// Create Asset record
	asset := &models.Asset{
		Key:        key,
		Filename:   filename,
		MimeType:   mimeType,
		SizeBytes:  int64(len(data)),
		Visibility: models.AssetVisibilityPrivate,
	}
	if err := s.db.Create(asset).Error; err != nil {
		// Attempt S3 cleanup on DB failure (SEC-04)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, key)
		return nil, fmt.Errorf("failed to create asset record: %w", err)
	}

	// Update music set with asset ID
	musicSet.AssetID = &asset.ID
	if err := s.db.Save(&musicSet).Error; err != nil {
		// Cleanup
		s.db.Delete(asset)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, key)
		return nil, fmt.Errorf("failed to update music set: %w", err)
	}

	// Load asset relation
	musicSet.Asset = asset

	return &musicSet, nil
}

// getMimeTypeFromExtension returns MIME type based on file extension
func getMimeTypeFromExtension(ext string) string {
	switch ext {
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

// GetAllMusicSets returns all music sets for admin (with pagination)
func (s *MusicService) GetAllMusicSets(limit, offset int) ([]models.MusicSet, int64, error) {
	var sets []models.MusicSet
	var total int64

	if err := s.db.Model(&models.MusicSet{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := s.db.Preload("Asset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&sets).Error; err != nil {
		return nil, 0, err
	}

	return sets, total, nil
}

// GetMusicSetByID returns a single music set
func (s *MusicService) GetMusicSetByID(setID uuid.UUID) (*models.MusicSet, error) {
	var musicSet models.MusicSet
	if err := s.db.Preload("Asset").First(&musicSet, "id = ?", setID).Error; err != nil {
		return nil, err
	}
	return &musicSet, nil
}

// DeleteMusicSet deletes a music set and its audio file
// Follows SEC-04: Delete S3 objects first, then DB records
func (s *MusicService) DeleteMusicSet(ctx context.Context, setID uuid.UUID) error {
	var musicSet models.MusicSet
	if err := s.db.Preload("Asset").First(&musicSet, "id = ?", setID).Error; err != nil {
		return fmt.Errorf("music set not found: %w", err)
	}

	// Delete S3 object FIRST (SEC-04)
	if musicSet.Asset != nil {
		if err := s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, musicSet.Asset.Key); err != nil {
			// Log but continue - S3 might already be gone
		}
		// Delete asset record
		s.db.Delete(musicSet.Asset)
	}

	// Delete the music set
	if err := s.db.Delete(&musicSet).Error; err != nil {
		return fmt.Errorf("failed to delete music set: %w", err)
	}

	return nil
}

// UpdateMusicSetVisibility changes the visibility of a music set
func (s *MusicService) UpdateMusicSetVisibility(setID uuid.UUID, visibility string) error {
	if visibility != "private" && visibility != "public" {
		return fmt.Errorf("invalid visibility: must be 'private' or 'public'")
	}
	return s.db.Model(&models.MusicSet{}).Where("id = ?", setID).Update("visibility", visibility).Error
}

// UpdateMusicSetMetadata updates title and description
func (s *MusicService) UpdateMusicSetMetadata(setID uuid.UUID, title, description string) error {
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
	return s.db.Model(&models.MusicSet{}).Where("id = ?", setID).Updates(updates).Error
}

// GetPresignedMusicSetURL generates a presigned URL for the audio file
// Uses longer TTL for audio (default 2 hours) to support long DJ sets
func (s *MusicService) GetPresignedMusicSetURL(ctx context.Context, key string) (string, error) {
	ttl := time.Duration(s.cfg.AudioURLTTLMinutes) * time.Minute
	if ttl == 0 {
		ttl = 120 * time.Minute // 2 hours default for long sets
	}
	return s.s3Service.PresignMediaGet(ctx, s.cfg.MediaAudioBucket, key, ttl)
}

// GetPublicMusicSets returns all public music sets for users
func (s *MusicService) GetPublicMusicSets(limit, offset int) ([]models.MusicSet, int64, error) {
	var sets []models.MusicSet
	var total int64

	query := s.db.Model(&models.MusicSet{}).Where("visibility = ?", "public")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Asset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&sets).Error; err != nil {
		return nil, 0, err
	}

	return sets, total, nil
}

// GetLocalMusicPath returns the local file path for a music file
// Downloads from S3 if not present locally
func (s *MusicService) GetLocalMusicPath(ctx context.Context, key string) (string, error) {
	localPath := filepath.Join(s.cfg.LocalAssetsPath, filepath.FromSlash(key))

	// Check if file exists locally
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	// Not local - download from S3
	if s.storageService == nil {
		return "", fmt.Errorf("audio not available locally and no storage service configured")
	}

	data, err := s.s3Service.DownloadMedia(ctx, s.cfg.MediaAudioBucket, key)
	if err != nil {
		return "", fmt.Errorf("failed to download from S3: %w", err)
	}

	// Save to local cache
	absPath, _, _, err := s.storageService.SaveStream(ctx, key, bytes.NewReader(data.Bytes()))
	if err != nil {
		return "", fmt.Errorf("failed to cache audio locally: %w", err)
	}

	return absPath, nil
}

// GetLocalMusicPathIfExists returns the local file path if it exists, empty string otherwise
// Does NOT download from S3 - use for checking cache without blocking
func (s *MusicService) GetLocalMusicPathIfExists(key string) string {
	localPath := filepath.Join(s.cfg.LocalAssetsPath, filepath.FromSlash(key))

	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	return ""
}
