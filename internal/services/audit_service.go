package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type AuditService struct {
	db           *gorm.DB
	emailService *EmailService
	cfg          *config.Config
}

func NewAuditService(db *gorm.DB, emailService *EmailService, cfg *config.Config) *AuditService {
	return &AuditService{
		db:           db,
		emailService: emailService,
		cfg:          cfg,
	}
}

// LogAction logs an admin action to the audit log
func (s *AuditService) LogAction(adminID uuid.UUID, action, targetType string, targetID uuid.UUID, details map[string]interface{}, ipAddress, userAgent string) error {
	detailsJSON := ""
	if details != nil {
		if jsonBytes, err := json.Marshal(details); err == nil {
			detailsJSON = string(jsonBytes)
		}
	}

	log := &models.AuditLog{
		AdminID:    adminID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    detailsJSON,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
	}

	if err := s.db.Create(log).Error; err != nil {
		return err
	}

	// Check for suspicious activity (rate limiting check)
	go s.checkSuspiciousActivity(adminID, action)

	return nil
}

// checkSuspiciousActivity checks if an admin is performing too many actions
func (s *AuditService) checkSuspiciousActivity(adminID uuid.UUID, action string) {
	// Check last 5 minutes for ticket cancellations
	if action == "cancel_ticket" {
		fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
		var count int64
		s.db.Model(&models.AuditLog{}).
			Where("admin_id = ? AND action = ? AND created_at > ?", adminID, "cancel_ticket", fiveMinutesAgo).
			Count(&count)

		// Alert if more than 5 cancellations in 5 minutes
		if count >= 5 && s.emailService != nil && s.cfg != nil && s.cfg.AdminAlertEmail != "" {
			var admin models.User
			if err := s.db.First(&admin, adminID).Error; err == nil {
				subject := "⚠️ Verdächtige Admin-Aktivität erkannt"
				body := fmt.Sprintf(`
Warnung: Der Administrator %s (%s) hat in den letzten 5 Minuten %d Tickets storniert.

Dies könnte auf einen kompromittierten Account hinweisen.

Bitte überprüfen Sie die Aktivität im Admin-Dashboard unter "Audit Log".
				`, admin.Name, admin.Email, count)

				_ = s.emailService.SendGenericTextEmail(s.cfg.AdminAlertEmail, subject, body)
			}
		}
	}
}

// GetRecentActions retrieves recent admin actions with pagination
func (s *AuditService) GetRecentActions(page, limit int, adminID *uuid.UUID, action string) ([]*models.AuditLog, int64, error) {
	var logs []*models.AuditLog
	var total int64

	query := s.db.Model(&models.AuditLog{}).Preload("Admin")

	// Filter by admin if provided
	if adminID != nil {
		query = query.Where("admin_id = ?", *adminID)
	}

	// Filter by action if provided
	if action != "" {
		query = query.Where("action = ?", action)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetActionCount returns the count of actions in a time window
func (s *AuditService) GetActionCount(adminID uuid.UUID, action string, since time.Time) (int64, error) {
	var count int64
	err := s.db.Model(&models.AuditLog{}).
		Where("admin_id = ? AND action = ? AND created_at > ?", adminID, action, since).
		Count(&count).Error
	return count, err
}

// GetStats returns audit log statistics
func (s *AuditService) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total actions
	var totalActions int64
	if err := s.db.Model(&models.AuditLog{}).Count(&totalActions).Error; err != nil {
		return nil, err
	}
	stats["total_actions"] = totalActions

	// Actions by type
	var actionCounts []struct {
		Action string
		Count  int64
	}
	if err := s.db.Model(&models.AuditLog{}).
		Select("action, COUNT(*) as count").
		Group("action").
		Order("count DESC").
		Scan(&actionCounts).Error; err != nil {
		return nil, err
	}
	stats["actions_by_type"] = actionCounts

	// Most active admins (last 30 days)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	var adminCounts []struct {
		AdminID uuid.UUID
		Count   int64
	}
	if err := s.db.Model(&models.AuditLog{}).
		Select("admin_id, COUNT(*) as count").
		Where("created_at > ?", thirtyDaysAgo).
		Group("admin_id").
		Order("count DESC").
		Limit(10).
		Scan(&adminCounts).Error; err != nil {
		return nil, err
	}
	stats["most_active_admins_30d"] = adminCounts

	// Recent activity (last 24 hours)
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	var recentCount int64
	if err := s.db.Model(&models.AuditLog{}).
		Where("created_at > ?", twentyFourHoursAgo).
		Count(&recentCount).Error; err != nil {
		return nil, err
	}
	stats["actions_last_24h"] = recentCount

	return stats, nil
}

