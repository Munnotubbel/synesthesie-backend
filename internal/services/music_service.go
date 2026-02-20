package services

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/pkg/audio"
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

	// Process Audio via FFmpeg (Duration, Peaks, HLS Files)
	log.Printf("[Upload] Processing audio file %s for HLS and Peaks...", filename)
	result, err := audio.ProcessAudio(ctx, data, ext)
	if err != nil {
		return nil, fmt.Errorf("failed to process audio: %w", err)
	}

	// Detect MIME type for the original file, though now we only care about the HLS files
	mimeType := getMimeTypeFromExtension(ext)
	if mimeType == "application/octet-stream" {
		sniffed := http.DetectContentType(data)
		if strings.HasPrefix(sniffed, "audio/") {
			mimeType = sniffed
		}
	}

	// A unique directory prefix for this set's HLS files
	baseKey := fmt.Sprintf("music/%s", uuid.New().String())

	// Upload all generated HLS files to S3 under the baseKey
	log.Printf("[Upload] Uploading %d HLS segments to S3...", len(result.HLSFiles))
	for name, fileData := range result.HLSFiles {
		hlsKey := fmt.Sprintf("%s/%s", baseKey, name)

		contentType := "application/octet-stream"
		if strings.HasSuffix(name, ".m3u8") {
			contentType = "application/vnd.apple.mpegurl"
		} else if strings.HasSuffix(name, ".ts") {
			contentType = "video/MP2T"
		}

		if err := s.s3Service.UploadMedia(ctx, s.cfg.MediaAudioBucket, hlsKey, fileData, contentType); err != nil {
			return nil, fmt.Errorf("failed to upload HLS segment %s to S3: %w", name, err)
		}
	}

	// Delete old asset if it exists
	if musicSet.AssetID != nil {
		var oldAsset models.Asset
		if err := s.db.First(&oldAsset, "id = ?", *musicSet.AssetID).Error; err == nil {
			// Extract old base key (e.g., from "music/uuid/master.m3u8" to "music/uuid")
			// We ideally want to list and delete all files in that prefix, but for now
			// we at least try to delete the exact key logged. If it was a raw file, this deletes it.
			// HLS directories will need prefix deletion, but S3Service.DeleteMedia only takes exact keys right now.

			// Try deleting old asset key
			_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, oldAsset.Key)

			// Delete old asset record
			s.db.Delete(&oldAsset)
		}
	}

	// Create Asset record for the master.m3u8
	masterKey := fmt.Sprintf("%s/master.m3u8", baseKey)
	asset := &models.Asset{
		Key:        masterKey,
		Filename:   "master.m3u8",
		MimeType:   "application/vnd.apple.mpegurl",
		SizeBytes:  int64(len(result.HLSFiles["master.m3u8"])), // Just the playlist size
		Visibility: models.AssetVisibilityPrivate,
	}
	if err := s.db.Create(asset).Error; err != nil {
		// Attempt cleanup on DB failure (not exhaustive prefix deletion, but best effort)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, masterKey)
		return nil, fmt.Errorf("failed to create asset record: %w", err)
	}

	// Upload original source file for downloads (SourceAsset)
	sourceKey := fmt.Sprintf("%s/source%s", baseKey, ext)
	log.Printf("[Upload] Uploading original source file for downloads: %s", sourceKey)
	if err := s.s3Service.UploadMedia(ctx, s.cfg.MediaAudioBucket, sourceKey, data, mimeType); err != nil {
		// Cleanup the previously created HLS asset record
		s.db.Delete(asset)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, masterKey)
		return nil, fmt.Errorf("failed to upload original source file: %w", err)
	}

	// Create Asset record for the original source file
	sourceAsset := &models.Asset{
		Key:        sourceKey,
		Filename:   filename, // Use the user's original filename (e.g., "05 Let It Go.flac")
		MimeType:   mimeType,
		SizeBytes:  int64(len(data)),
		Visibility: models.AssetVisibilityPrivate,
	}
	if err := s.db.Create(sourceAsset).Error; err != nil {
		s.db.Delete(asset)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, masterKey)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, sourceKey)
		return nil, fmt.Errorf("failed to create source asset record: %w", err)
	}

	// Update music set with asset ID, source asset ID, duration, and peak data
	musicSet.AssetID = &asset.ID
	musicSet.SourceAssetID = &sourceAsset.ID
	musicSet.Duration = result.Duration
	musicSet.PeakData = result.PeakData

	if err := s.db.Save(&musicSet).Error; err != nil {
		// Cleanup
		s.db.Delete(asset)
		s.db.Delete(sourceAsset)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, masterKey)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, sourceKey)
		return nil, fmt.Errorf("failed to update music set: %w", err)
	}

	// Load asset relations
	musicSet.Asset = asset
	musicSet.SourceAsset = sourceAsset

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

	if err := s.db.Preload("Asset").Preload("SourceAsset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&sets).Error; err != nil {
		return nil, 0, err
	}

	return sets, total, nil
}

// GetMusicSetByID returns a single music set
func (s *MusicService) GetMusicSetByID(setID uuid.UUID) (*models.MusicSet, error) {
	var musicSet models.MusicSet
	if err := s.db.Preload("Asset").Preload("SourceAsset").First(&musicSet, "id = ?", setID).Error; err != nil {
		return nil, err
	}
	return &musicSet, nil
}

// DeleteMusicSet deletes a music set and its audio file
// Follows SEC-04: Delete S3 objects first, then DB records
func (s *MusicService) DeleteMusicSet(ctx context.Context, setID uuid.UUID) error {
	var musicSet models.MusicSet
	if err := s.db.Preload("Asset").Preload("SourceAsset").First(&musicSet, "id = ?", setID).Error; err != nil {
		return fmt.Errorf("music set not found: %w", err)
	}

	// Delete S3 object FIRST (SEC-04)
	if musicSet.Asset != nil {
		if err := s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, musicSet.Asset.Key); err != nil {
			log.Printf("[DeleteMusic] Warning: failed to delete S3 asset %s: %v", musicSet.Asset.Key, err)
			// Continue with deletion even if S3 fails (orphaned file is better than broken state)
		}
	}

	if musicSet.SourceAsset != nil {
		if err := s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, musicSet.SourceAsset.Key); err != nil {
			log.Printf("[DeleteMusic] Warning: failed to delete S3 source asset %s: %v", musicSet.SourceAsset.Key, err)
		}
	}

	// Delete DB record
	// Note: GORM handles deleting the Asset records due to foreign key constraints if set up,
	// but explicitly deleting the Asset records is safer since we use pointer relations.
	if err := s.db.Delete(&musicSet).Error; err != nil {
		return fmt.Errorf("failed to delete music set from db: %w", err)
	}

	if musicSet.AssetID != nil {
		s.db.Delete(&models.Asset{}, *musicSet.AssetID)
	}
	if musicSet.SourceAssetID != nil {
		s.db.Delete(&models.Asset{}, *musicSet.SourceAssetID)
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

// IncrementPlayCount increments the play count for a music set
// Called when a user starts playing a track
func (s *MusicService) IncrementPlayCount(setID uuid.UUID) error {
	return s.db.Model(&models.MusicSet{}).
		Where("id = ?", setID).
		UpdateColumn("play_count", gorm.Expr("play_count + 1")).Error
}

// IncrementDownloadCount increments the download count for a music set
// Called when a user downloads a track (via ?download=true parameter)
func (s *MusicService) IncrementDownloadCount(setID uuid.UUID) error {
	return s.db.Model(&models.MusicSet{}).
		Where("id = ?", setID).
		UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error
}
