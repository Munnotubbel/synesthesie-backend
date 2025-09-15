package services

import (
	"errors"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type InviteService struct {
	db *gorm.DB
}

func NewInviteService(db *gorm.DB) *InviteService {
	return &InviteService{db: db}
}

// CreateInviteCode creates a new invite code
func (s *InviteService) CreateInviteCode() (*models.InviteCode, error) {
	invite := &models.InviteCode{
		Status: models.InviteStatusNew,
	}

	// The code will be generated automatically in BeforeCreate hook
	if err := s.db.Create(invite).Error; err != nil {
		return nil, err
	}

	return invite, nil
}

// GetInviteByCode retrieves an invite by its code
func (s *InviteService) GetInviteByCode(code string) (*models.InviteCode, error) {
	var invite models.InviteCode
	if err := s.db.Where("code = ?", code).First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invite code not found")
		}
		return nil, err
	}
	return &invite, nil
}

// GetInviteByID retrieves an invite by ID
func (s *InviteService) GetInviteByID(inviteID uuid.UUID) (*models.InviteCode, error) {
	var invite models.InviteCode
	if err := s.db.First(&invite, inviteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}
	return &invite, nil
}

// ViewInviteCode marks an invite code as viewed (first time access)
func (s *InviteService) ViewInviteCode(code string) (*models.InviteCode, error) {
	var invite models.InviteCode
	if err := s.db.Where("code = ?", code).First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invite code not found")
		}
		return nil, err
	}

	// Check if code can be viewed
	if !invite.CanBeViewed() {
		return nil, errors.New("invite code has already been viewed or is no longer available")
	}

	// Mark as viewed
	invite.MarkAsViewed()
	if err := s.db.Save(&invite).Error; err != nil {
		return nil, err
	}

	return &invite, nil
}

// DeactivateInvite deactivates an invite code
func (s *InviteService) DeactivateInvite(inviteID uuid.UUID) error {
	result := s.db.Model(&models.InviteCode{}).Where("id = ?", inviteID).Update("status", models.InviteStatusInactive)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("invite not found")
	}

	return nil
}

// GetAllInvites retrieves all invites with pagination
func (s *InviteService) GetAllInvites(offset, limit int, includeUsed bool) ([]*models.InviteCode, int64, error) {
	var invites []*models.InviteCode
	var total int64

	query := s.db.Model(&models.InviteCode{})
	if !includeUsed {
		query = query.Where("status != ?", models.InviteStatusRegistered)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results with user preloaded
	if err := query.Preload("User").Offset(offset).Limit(limit).Order("created_at DESC").Find(&invites).Error; err != nil {
		return nil, 0, err
	}

	return invites, total, nil
}

// GetActiveInvites retrieves all active and unused invites
func (s *InviteService) GetActiveInvites(offset, limit int) ([]*models.InviteCode, int64, error) {
	var invites []*models.InviteCode
	var total int64

	query := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusNew)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&invites).Error; err != nil {
		return nil, 0, err
	}

	return invites, total, nil
}

// ValidateInviteCodeForRegistration checks if an invite code can be used for registration
func (s *InviteService) ValidateInviteCodeForRegistration(code string) (*models.InviteCode, error) {
	invite, err := s.GetInviteByCode(code)
	if err != nil {
		return nil, err
	}

	if !invite.CanBeUsedForRegistration() {
		return nil, errors.New("invite code must be viewed first before registration")
	}

	return invite, nil
}

// MarkInviteAsRegistered marks an invite code as used for registration
func (s *InviteService) MarkInviteAsRegistered(code string, userID uuid.UUID) error {
	var invite models.InviteCode
	if err := s.db.Where("code = ?", code).First(&invite).Error; err != nil {
		return err
	}

	invite.MarkAsRegistered(userID)
	return s.db.Save(&invite).Error
}

// CreateBulkInviteCodes creates multiple invite codes at once
func (s *InviteService) CreateBulkInviteCodes(count int) ([]*models.InviteCode, error) {
	if count <= 0 || count > 100 {
		return nil, errors.New("count must be between 1 and 100")
	}

	invites := make([]*models.InviteCode, count)
	for i := 0; i < count; i++ {
		invites[i] = &models.InviteCode{
			Status: models.InviteStatusNew,
		}
	}

	if err := s.db.Create(&invites).Error; err != nil {
		return nil, err
	}

	return invites, nil
}

// GetInviteStats returns statistics about invite codes
func (s *InviteService) GetInviteStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	// Total invites
	var total int64
	if err := s.db.Model(&models.InviteCode{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total

	// New invites
	var newInvites int64
	if err := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusNew).Count(&newInvites).Error; err != nil {
		return nil, err
	}
	stats["new"] = newInvites

	// Viewed invites
	var viewed int64
	if err := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusViewed).Count(&viewed).Error; err != nil {
		return nil, err
	}
	stats["viewed"] = viewed

	// Registered invites
	var registered int64
	if err := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusRegistered).Count(&registered).Error; err != nil {
		return nil, err
	}
	stats["registered"] = registered

	// Inactive invites
	var inactive int64
	if err := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusInactive).Count(&inactive).Error; err != nil {
		return nil, err
	}
	stats["inactive"] = inactive

	return stats, nil
}

// SetInviteQRGenerated marks the invite as QR generated
func (s *InviteService) SetInviteQRGenerated(inviteID uuid.UUID) error {
	return s.db.Model(&models.InviteCode{}).Where("id = ?", inviteID).Update("qr_generated", true).Error
}
