package services

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/pkg/crypto"
	"gorm.io/gorm"
)

type AdminService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAdminService(db *gorm.DB, cfg *config.Config) *AdminService {
	return &AdminService{db: db, cfg: cfg}
}

func (s *AdminService) GetConfig() *config.Config { return s.cfg }

// CreateAssetRecord stores an asset metadata record
func (s *AdminService) CreateAssetRecord(key, filename string, size int64, checksum string, isImage bool) (*models.Asset, error) {
	visibility := models.AssetVisibilityPrivate
	if isImage {
		visibility = models.AssetVisibilityPublic
	}
	asset := &models.Asset{
		Key:        key,
		Filename:   filename,
		MimeType:   "",
		SizeBytes:  size,
		Checksum:   checksum,
		Visibility: visibility,
	}
	if err := s.db.Create(asset).Error; err != nil {
		return nil, err
	}
	return asset, nil
}

// CreateDefaultAdmin creates the default admin user if it doesn't exist
func (s *AdminService) CreateDefaultAdmin() error {
	// Check if admin already exists
	var count int64
	if err := s.db.Model(&models.User{}).Where("username = ?", s.cfg.AdminUsername).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil // Admin already exists
	}

	// Hash password
	hashedPassword, err := crypto.HashPassword(s.cfg.AdminPassword, s.cfg.BcryptCost)
	if err != nil {
		return err
	}

	// Create admin user
	admin := &models.User{
		Username: s.cfg.AdminUsername,
		Email:    s.cfg.AdminEmail,
		Password: hashedPassword,
		Name:     "Administrator",
		IsAdmin:  true,
		IsActive: true,
	}

	return s.db.Create(admin).Error
}

// ResetUserPassword resets a user's password
func (s *AdminService) ResetUserPassword(userID uuid.UUID) (string, error) {
	// Generate new password
	newPassword := crypto.GenerateRandomPassword(12)

	// Hash password
	hashedPassword, err := crypto.HashPassword(newPassword, s.cfg.BcryptCost)
	if err != nil {
		return "", err
	}

	// Update user password
	result := s.db.Model(&models.User{}).Where("id = ?", userID).Update("password", hashedPassword)
	if result.Error != nil {
		return "", result.Error
	}

	if result.RowsAffected == 0 {
		return "", errors.New("user not found")
	}

	return newPassword, nil
}

// GetPickupServicePrice retrieves the current pickup service price
func (s *AdminService) GetPickupServicePrice() (float64, error) {
	var setting models.SystemSetting
	err := s.db.Where("key = ?", "pickup_service_price").First(&setting).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create default setting
			setting = models.SystemSetting{
				Key:   "pickup_service_price",
				Value: "10.00",
			}
			if err := s.db.Create(&setting).Error; err != nil {
				return 0, err
			}
			return 10.00, nil
		}
		return 0, err
	}

	var price float64
	if _, err := fmt.Sscanf(setting.Value, "%f", &price); err != nil {
		return 0, err
	}

	return price, nil
}

// UpdatePickupServicePrice updates the pickup service price
func (s *AdminService) UpdatePickupServicePrice(price float64) error {
	if price < 0 {
		return errors.New("price cannot be negative")
	}

	value := fmt.Sprintf("%.2f", price)

	// Update or create setting
	var setting models.SystemSetting
	err := s.db.Where("key = ?", "pickup_service_price").First(&setting).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new setting
			setting = models.SystemSetting{
				Key:   "pickup_service_price",
				Value: value,
			}
			return s.db.Create(&setting).Error
		}
		return err
	}

	// Update existing setting
	return s.db.Model(&setting).Update("value", value).Error
}

// GetDashboardStats returns statistics for the admin dashboard
func (s *AdminService) GetDashboardStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total users
	var userCount int64
	if err := s.db.Model(&models.User{}).Where("is_admin = ?", false).Count(&userCount).Error; err != nil {
		return nil, err
	}
	stats["total_users"] = userCount

	// Active events
	var activeEventCount int64
	if err := s.db.Model(&models.Event{}).Where("is_active = ?", true).Count(&activeEventCount).Error; err != nil {
		return nil, err
	}
	stats["active_events"] = activeEventCount

	// Total tickets sold
	var ticketCount int64
	if err := s.db.Model(&models.Ticket{}).Where("status = ?", "paid").Count(&ticketCount).Error; err != nil {
		return nil, err
	}
	stats["tickets_sold"] = ticketCount

	// Total revenue
	var totalRevenue float64
	if err := s.db.Model(&models.Ticket{}).Where("status = ?", "paid").Select("COALESCE(SUM(total_amount), 0)").Scan(&totalRevenue).Error; err != nil {
		return nil, err
	}
	stats["total_revenue"] = totalRevenue

	// Unused invite codes (new status)
	var unusedInvites int64
	if err := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusNew).Count(&unusedInvites).Error; err != nil {
		return nil, err
	}
	stats["unused_invites"] = unusedInvites

	return stats, nil
}

// GetUserStats returns statistics about user preferences
func (s *AdminService) GetUserStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Most popular drinks
	var drinks []struct {
		FavoriteDrink string
		Count         int64
	}
	s.db.Model(&models.User{}).
		Select("favorite_drink, COUNT(*) as count").
		Where("favorite_drink IS NOT NULL AND favorite_drink != ''").
		Group("favorite_drink").
		Order("count DESC").
		Limit(10).
		Scan(&drinks)
	stats["popular_drinks"] = drinks

	// Most popular cocktails
	var cocktails []struct {
		FavoriteCocktail string
		Count            int64
	}
	s.db.Model(&models.User{}).
		Select("favorite_cocktail, COUNT(*) as count").
		Where("favorite_cocktail IS NOT NULL AND favorite_cocktail != ''").
		Group("favorite_cocktail").
		Order("count DESC").
		Limit(10).
		Scan(&cocktails)
	stats["popular_cocktails"] = cocktails

	// Most popular shots
	var shots []struct {
		FavoriteShot string
		Count        int64
	}
	s.db.Model(&models.User{}).
		Select("favorite_shot, COUNT(*) as count").
		Where("favorite_shot IS NOT NULL AND favorite_shot != ''").
		Group("favorite_shot").
		Order("count DESC").
		Limit(10).
		Scan(&shots)
	stats["popular_shots"] = shots

	return stats, nil
}
