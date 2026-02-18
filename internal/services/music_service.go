package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type MusicService struct {
	db        *gorm.DB
	cfg       *config.Config
	s3Service *S3Service
}

func NewMusicService(db *gorm.DB, cfg *config.Config, s3Service *S3Service) *MusicService {
	return &MusicService{
		db:        db,
		cfg:       cfg,
		s3Service: s3Service,
	}
}

// CreateMusicSet creates a new music set (MSC-ADM-01)
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

// GetAllMusicSets returns all music sets for admin (with pagination)
func (s *MusicService) GetAllMusicSets(limit, offset int) ([]models.MusicSet, int64, error) {
	var sets []models.MusicSet
	var total int64

	if err := s.db.Model(&models.MusicSet{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Preload tracks with their assets, ordered by track_order
	if err := s.db.Preload("Tracks", func(db *gorm.DB) *gorm.DB {
		return db.Order("track_order ASC")
	}).Preload("Tracks.Asset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&sets).Error; err != nil {
		return nil, 0, err
	}

	return sets, total, nil
}

// GetMusicSetByID returns a single music set with all tracks
func (s *MusicService) GetMusicSetByID(setID uuid.UUID) (*models.MusicSet, error) {
	var musicSet models.MusicSet
	if err := s.db.Preload("Tracks", func(db *gorm.DB) *gorm.DB {
		return db.Order("track_order ASC")
	}).Preload("Tracks.Asset").First(&musicSet, "id = ?", setID).Error; err != nil {
		return nil, err
	}
	return &musicSet, nil
}

// DeleteMusicSet deletes a music set and all its tracks (MSC-ADM-04)
// Follows SEC-04: Delete S3 objects first, then DB records
func (s *MusicService) DeleteMusicSet(ctx context.Context, setID uuid.UUID) error {
	var musicSet models.MusicSet
	if err := s.db.Preload("Tracks.Asset").First(&musicSet, "id = ?", setID).Error; err != nil {
		return fmt.Errorf("music set not found: %w", err)
	}

	// Delete all track S3 objects FIRST (SEC-04)
	for _, track := range musicSet.Tracks {
		if track.Asset != nil {
			if err := s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, track.Asset.Key); err != nil {
				// Log but continue - S3 might already be gone
			}
		}
	}

	// Delete DB records (MusicSet CASCADE deletes Tracks, then delete Assets manually)
	// First collect asset IDs to delete after
	assetIDs := make([]uuid.UUID, 0)
	for _, track := range musicSet.Tracks {
		if track.Asset != nil {
			assetIDs = append(assetIDs, track.Asset.ID)
		}
	}

	// Delete the music set (CASCADE will delete tracks)
	if err := s.db.Delete(&musicSet).Error; err != nil {
		return fmt.Errorf("failed to delete music set: %w", err)
	}

	// Delete asset records
	if len(assetIDs) > 0 {
		s.db.Delete(&models.Asset{}, "id IN ?", assetIDs)
	}

	return nil
}

// UpdateMusicSetVisibility changes the visibility of a music set (MSC-ADM-05)
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

// UploadTrack uploads a track to a music set (MSC-ADM-02)
func (s *MusicService) UploadTrack(ctx context.Context, setID uuid.UUID, filename string, data []byte, title, artist string) (*models.MusicTrack, error) {
	// Verify music set exists
	var musicSet models.MusicSet
	if err := s.db.First(&musicSet, "id = ?", setID).Error; err != nil {
		return nil, fmt.Errorf("music set not found: %w", err)
	}

	// Get current max track order for this set
	var maxOrder int
	s.db.Model(&models.MusicTrack{}).Where("music_set_id = ?", setID).Select("COALESCE(MAX(track_order), 0)").Scan(&maxOrder)

	// Generate S3 key
	key := fmt.Sprintf("music/%s/%s%s", setID.String(), uuid.New().String(), getFileExtension(filename))

	// Upload to S3 (audio bucket)
	if err := s.s3Service.UploadMedia(ctx, s.cfg.MediaAudioBucket, key, data, "audio/flac"); err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Create Asset record
	asset := &models.Asset{
		Key:        key,
		Filename:   filename,
		MimeType:   "audio/flac",
		SizeBytes:  int64(len(data)),
		Visibility: models.AssetVisibilityPrivate,
	}
	if err := s.db.Create(asset).Error; err != nil {
		// Attempt S3 cleanup on DB failure (SEC-04)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, key)
		return nil, fmt.Errorf("failed to create asset record: %w", err)
	}

	// Create MusicTrack record
	track := &models.MusicTrack{
		MusicSetID: setID,
		AssetID:    asset.ID,
		Title:      title,
		Artist:     artist,
		TrackOrder: maxOrder + 1,
	}
	if err := s.db.Create(track).Error; err != nil {
		// Cleanup: delete asset and S3 object
		s.db.Delete(asset)
		_ = s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, key)
		return nil, fmt.Errorf("failed to create track record: %w", err)
	}

	// Load asset relation
	track.Asset = asset

	return track, nil
}

// DeleteTrack deletes a single track from a music set (MSC-ADM-03)
func (s *MusicService) DeleteTrack(ctx context.Context, trackID uuid.UUID) error {
	var track models.MusicTrack
	if err := s.db.Preload("Asset").First(&track, "id = ?", trackID).Error; err != nil {
		return fmt.Errorf("track not found: %w", err)
	}

	// Delete from S3 FIRST (SEC-04)
	if track.Asset != nil {
		if err := s.s3Service.DeleteMedia(ctx, s.cfg.MediaAudioBucket, track.Asset.Key); err != nil {
			// Log but continue - S3 might already be gone
		}
	}

	// Delete track record
	if err := s.db.Delete(&track).Error; err != nil {
		return fmt.Errorf("failed to delete track: %w", err)
	}

	// Delete asset record
	if track.Asset != nil {
		s.db.Delete(track.Asset)
	}

	return nil
}

// UpdateTrackMetadata updates track title and artist (MSC-ADM-06)
func (s *MusicService) UpdateTrackMetadata(trackID uuid.UUID, title, artist string) error {
	updates := map[string]interface{}{}
	if title != "" {
		updates["title"] = title
	}
	if artist != "" {
		updates["artist"] = artist
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&models.MusicTrack{}).Where("id = ?", trackID).Updates(updates).Error
}

// UpdateTrackOrder updates the track order
func (s *MusicService) UpdateTrackOrder(trackID uuid.UUID, order int) error {
	return s.db.Model(&models.MusicTrack{}).Where("id = ?", trackID).Update("track_order", order).Error
}

// GetPresignedTrackURL generates a presigned URL for a track
func (s *MusicService) GetPresignedTrackURL(ctx context.Context, key string) (string, error) {
	ttl := time.Duration(s.cfg.PresignedURLTTLMinutes) * time.Minute
	if ttl == 0 {
		ttl = 15 * time.Minute
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

	if err := query.Preload("Tracks", func(db *gorm.DB) *gorm.DB {
		return db.Order("track_order ASC")
	}).Preload("Tracks.Asset").Order("created_at DESC").Limit(limit).Offset(offset).Find(&sets).Error; err != nil {
		return nil, 0, err
	}

	return sets, total, nil
}

// Helper function to get file extension
func getFileExtension(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
	}
	return ""
}
