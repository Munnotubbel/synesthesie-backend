package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type BackupService struct {
	db    *gorm.DB
	cfg   *config.Config
	s3Svc *S3Service
}

func NewBackupService(db *gorm.DB, cfg *config.Config, s3Svc *S3Service) *BackupService {
	return &BackupService{
		db:    db,
		cfg:   cfg,
		s3Svc: s3Svc,
	}
}

// ListBackups retrieves all backups with pagination, ordered by most recent first
func (s *BackupService) ListBackups(offset, limit int) ([]*models.Backup, int64, error) {
	var backups []*models.Backup
	var total int64

	// Count total backups
	if err := s.db.Model(&models.Backup{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated backups, ordered by created_at descending
	if err := s.db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&backups).Error; err != nil {
		return nil, 0, err
	}

	return backups, total, nil
}

// SyncBackupsFromS3 synchronizes backup records from S3 bucket
// This fetches all backup files from S3 and creates missing database records
func (s *BackupService) SyncBackupsFromS3() (int, error) {
	if s.cfg.BackupBucket == "" {
		return 0, errors.New("backup bucket not configured")
	}

	ctx := context.Background()
	prefix := fmt.Sprintf("db/%s/", s.cfg.DBName)

	// List objects from S3
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.BackupBucket),
		Prefix: aws.String(prefix),
	}

	client, err := s.s3Svc.GetBackupClient()
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 client: %w", err)
	}

	result, err := client.ListObjectsV2(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to list S3 objects: %w", err)
	}

	synced := 0
	for _, obj := range result.Contents {
		if obj.Key == nil || obj.Size == nil || obj.LastModified == nil {
			continue
		}

		s3Key := *obj.Key
		// Extract filename from key
		parts := strings.Split(s3Key, "/")
		filename := parts[len(parts)-1]

		// Skip if not a .sql.gz file
		if !strings.HasSuffix(filename, ".sql.gz") {
			continue
		}

		// Check if backup already exists in DB
		var existing models.Backup
		err := s.db.Where("s3_key = ?", s3Key).First(&existing).Error
		if err == nil {
			// Already exists
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Real error
			return synced, err
		}

		// Create new backup record
		backup := &models.Backup{
			Filename:    filename,
			S3Key:       s3Key,
			SizeBytes:   *obj.Size,
			Status:      "completed",
			Type:        "automatic",
			StartedAt:   *obj.LastModified,
			CompletedAt: obj.LastModified,
		}

		if err := s.db.Create(backup).Error; err != nil {
			return synced, fmt.Errorf("failed to create backup record: %w", err)
		}
		synced++
	}

	return synced, nil
}

// GetBackupByID retrieves a backup by ID
func (s *BackupService) GetBackupByID(backupID uuid.UUID) (*models.Backup, error) {
	var backup models.Backup
	if err := s.db.First(&backup, backupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("backup not found")
		}
		return nil, err
	}
	return &backup, nil
}

// DeleteBackup is DISABLED for security reasons
// Backups are disaster recovery and should only be deleted via S3 lifecycle policies
func (s *BackupService) DeleteBackup(backupID uuid.UUID, deleteFromS3 bool) error {
	return errors.New("deleting backups via API is disabled for security reasons - use S3 lifecycle policies instead")
}

// CreateManualBackup creates a backup record for a manually triggered backup
func (s *BackupService) CreateManualBackup(userID uuid.UUID) (*models.Backup, error) {
	backup := &models.Backup{
		Filename:  fmt.Sprintf("%s_%s_manual.sql.gz", s.cfg.DBName, time.Now().UTC().Format("2006-01-02T15-04-05Z")),
		S3Key:     "", // Will be set after actual backup is created
		Status:    "in_progress",
		Type:      "manual",
		StartedAt: time.Now(),
		CreatedBy: &userID,
	}

	if err := s.db.Create(backup).Error; err != nil {
		return nil, err
	}

	return backup, nil
}

// UpdateBackupStatus updates the status of a backup
func (s *BackupService) UpdateBackupStatus(backupID uuid.UUID, status string, s3Key string, sizeBytes int64, errorMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if s3Key != "" {
		updates["s3_key"] = s3Key
	}
	if sizeBytes > 0 {
		updates["size_bytes"] = sizeBytes
	}
	if errorMsg != "" {
		updates["error_message"] = errorMsg
	}
	if status == "completed" || status == "failed" {
		now := time.Now()
		updates["completed_at"] = now
	}

	return s.db.Model(&models.Backup{}).Where("id = ?", backupID).Updates(updates).Error
}

// GetLatestBackup returns the most recent completed backup
func (s *BackupService) GetLatestBackup() (*models.Backup, error) {
	var backup models.Backup
	err := s.db.Where("status = ?", "completed").Order("completed_at DESC").First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no completed backups found")
		}
		return nil, err
	}
	return &backup, nil
}

// GetBackupStats returns statistics about backups
func (s *BackupService) GetBackupStats() (map[string]interface{}, error) {
	var totalCount int64
	var completedCount int64
	var failedCount int64
	var totalSize int64

	s.db.Model(&models.Backup{}).Count(&totalCount)
	s.db.Model(&models.Backup{}).Where("status = ?", "completed").Count(&completedCount)
	s.db.Model(&models.Backup{}).Where("status = ?", "failed").Count(&failedCount)
	s.db.Model(&models.Backup{}).Where("status = ?", "completed").Select("COALESCE(SUM(size_bytes), 0)").Scan(&totalSize)

	// Get latest backup
	latest, _ := s.GetLatestBackup()
	var latestDate *time.Time
	if latest != nil && latest.CompletedAt != nil {
		latestDate = latest.CompletedAt
	}

	stats := map[string]interface{}{
		"total_backups":     totalCount,
		"completed_backups": completedCount,
		"failed_backups":    failedCount,
		"total_size_bytes":  totalSize,
		"latest_backup":     latestDate,
	}

	return stats, nil
}
