package services

import (
	"errors"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(userID uuid.UUID) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername retrieves a user by username
func (s *UserService) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

// UpdateUserProfile updates user profile information
func (s *UserService) UpdateUserProfile(userID uuid.UUID, updates map[string]interface{}) error {
	// Only allow updating certain fields
	allowedFields := map[string]bool{
		"drink1": true,
		"drink2": true,
		"drink3": true,
	}

	// Filter updates to only allowed fields
	filteredUpdates := make(map[string]interface{})
	for key, value := range updates {
		if allowedFields[key] {
			filteredUpdates[key] = value
		}
	}

	if len(filteredUpdates) == 0 {
		return errors.New("no valid fields to update")
	}

	result := s.db.Model(&models.User{}).Where("id = ?", userID).Updates(filteredUpdates)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdateUserGroup updates the user's group to 'bubble' or 'guests'
func (s *UserService) UpdateUserGroup(userID uuid.UUID, group string) error {
	if group != "bubble" && group != "guests" {
		return errors.New("invalid group; must be 'bubble' or 'guests'")
	}
	result := s.db.Model(&models.User{}).Where("id = ?", userID).Update("group", group)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}
	return nil
}

// UpdateUserActive sets is_active
func (s *UserService) UpdateUserActive(userID uuid.UUID, isActive bool) error {
	result := s.db.Model(&models.User{}).Where("id = ?", userID).Update("is_active", isActive)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}
	return nil
}

// GetAllUsers retrieves all users with pagination
func (s *UserService) GetAllUsers(offset, limit int) ([]*models.User, int64, error) {
	var users []*models.User
	var total int64

	// Count total users
	if err := s.db.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated users
	if err := s.db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// GetUserWithDetails retrieves a user with all related data
func (s *UserService) GetUserWithDetails(userID uuid.UUID) (*models.User, error) {
	var user models.User

	err := s.db.Preload("Tickets", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at DESC")
	}).Preload("Tickets.Event").First(&user, userID).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return &user, nil
}

// SearchUsers searches for users by name or username
func (s *UserService) SearchUsers(query string, offset, limit int) ([]*models.User, int64, error) {
	var users []*models.User
	var total int64

	searchQuery := "%" + query + "%"

	// Count matching users
	if err := s.db.Model(&models.User{}).Where("username ILIKE ? OR name ILIKE ?", searchQuery, searchQuery).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := s.db.Where("username ILIKE ? OR name ILIKE ?", searchQuery, searchQuery).
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// DeactivateUser deactivates a user account
func (s *UserService) DeactivateUser(userID uuid.UUID) error {
	result := s.db.Model(&models.User{}).Where("id = ?", userID).Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}
	return nil
}

// ReactivateUser reactivates a user account
func (s *UserService) ReactivateUser(userID uuid.UUID) error {
	result := s.db.Model(&models.User{}).Where("id = ?", userID).Update("is_active", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}
	return nil
}
