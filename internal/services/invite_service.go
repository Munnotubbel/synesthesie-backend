package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InviteService struct {
	db *gorm.DB
}

func NewInviteService(db *gorm.DB) *InviteService {
	return &InviteService{db: db}
}

func generateSecureCode(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateInviteCode creates a new invite code (default group: guests)
func (s *InviteService) CreateInviteCode() (*models.InviteCode, error) {
	code, err := generateSecureCode(32)
	if err != nil {
		return nil, err
	}
	invite := &models.InviteCode{
		Status: models.InviteStatusNew,
		Group:  "guests",
		Code:   code,
		// PublicID nil for guests
	}
	if err := s.db.Create(invite).Error; err != nil {
		return nil, err
	}
	return invite, nil
}

// CreateInviteCodeWithGroup creates a new invite code for a specific group (bubble|guests)
// guests → PublicID NULL; bubble → PublicID sequential numeric (1..1000)
func (s *InviteService) CreateInviteCodeWithGroup(group string) (*models.InviteCode, error) {
	if group != "bubble" && group != "guests" {
		return nil, errors.New("invalid group; must be 'bubble' or 'guests'")
	}
	if group == "guests" {
		return s.CreateInviteCode()
	}
	// bubble: allocate next sequential PublicID
	var created *models.InviteCode
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var setting models.SystemSetting
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key = ?", "bubble_public_id_counter").First(&setting).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			setting = models.SystemSetting{Key: "bubble_public_id_counter", Value: "0"}
			if err := tx.Create(&setting).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		current, _ := strconv.Atoi(setting.Value)
		next := current + 1
		if next > 1000 {
			return errors.New("bubble public id limit reached (max 1000)")
		}
		pub := strconv.Itoa(next)
		code, err := generateSecureCode(32)
		if err != nil {
			return err
		}
		invite := &models.InviteCode{Status: models.InviteStatusNew, Group: "bubble", Code: code, PublicID: &pub}
		if err := tx.Create(invite).Error; err != nil {
			return err
		}
		setting.Value = strconv.Itoa(next)
		if err := tx.Model(&models.SystemSetting{}).Where("id = ?", setting.ID).Update("value", setting.Value).Error; err != nil {
			return err
		}
		created = invite
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// CreateBulkInviteCodes creates multiple invite codes at once (guests)
func (s *InviteService) CreateBulkInviteCodes(count int) ([]*models.InviteCode, error) {
	if count <= 0 || count > 100 {
		return nil, errors.New("count must be between 1 and 100")
	}
	invites := make([]*models.InviteCode, count)
	for i := 0; i < count; i++ {
		code, err := generateSecureCode(32)
		if err != nil {
			return nil, err
		}
		invites[i] = &models.InviteCode{Status: models.InviteStatusNew, Group: "guests", Code: code}
	}
	if err := s.db.Create(&invites).Error; err != nil {
		return nil, err
	}
	return invites, nil
}

// CreateBulkInviteCodesWithGroup creates multiple invite codes for a group
func (s *InviteService) CreateBulkInviteCodesWithGroup(count int, group string) ([]*models.InviteCode, error) {
	if count <= 0 || count > 100 {
		return nil, errors.New("count must be between 1 and 100")
	}
	if group != "bubble" && group != "guests" {
		return nil, errors.New("invalid group; must be 'bubble' or 'guests'")
	}
	if group == "guests" {
		return s.CreateBulkInviteCodes(count)
	}
	var invites []*models.InviteCode
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var setting models.SystemSetting
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key = ?", "bubble_public_id_counter").First(&setting).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			setting = models.SystemSetting{Key: "bubble_public_id_counter", Value: "0"}
			if err := tx.Create(&setting).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		current, _ := strconv.Atoi(setting.Value)
		start := current + 1
		end := current + count
		if end > 1000 {
			return errors.New("bubble public id limit reached (max 1000)")
		}
		invites = make([]*models.InviteCode, 0, count)
		for n := start; n <= end; n++ {
			pub := strconv.Itoa(n)
			code, err := generateSecureCode(32)
			if err != nil {
				return err
			}
			invites = append(invites, &models.InviteCode{Status: models.InviteStatusNew, Group: "bubble", Code: code, PublicID: &pub})
		}
		if err := tx.Create(&invites).Error; err != nil {
			return err
		}
		setting.Value = strconv.Itoa(end)
		if err := tx.Model(&models.SystemSetting{}).Where("id = ?", setting.ID).Update("value", setting.Value).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return invites, nil
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
func (s *InviteService) GetAllInvites(offset, limit int, includeUsed bool, group string, status string) ([]*models.InviteCode, int64, error) {
	var invites []*models.InviteCode
	var total int64

	query := s.db.Model(&models.InviteCode{})
	if !includeUsed {
		query = query.Where("status != ?", models.InviteStatusRegistered)
	}
	if group != "" {
		if group != "bubble" && group != "guests" {
			return nil, 0, errors.New("invalid group; must be 'bubble' or 'guests'")
		}
		query = query.Where("\"group\" = ?", group)
	}
	if status != "" {
		switch status {
		case models.InviteStatusNew, models.InviteStatusViewed, models.InviteStatusRegistered, models.InviteStatusInactive:
			query = query.Where("status = ?", status)
		default:
			return nil, 0, errors.New("invalid status; must be new|viewed|registered|inactive")
		}
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

// GetActiveInvitesByGroup retrieves active invites filtered by group
func (s *InviteService) GetActiveInvitesByGroup(offset, limit int, group string) ([]*models.InviteCode, int64, error) {
	if group != "bubble" && group != "guests" {
		return nil, 0, errors.New("invalid group; must be 'bubble' or 'guests'")
	}
	var invites []*models.InviteCode
	var total int64

	query := s.db.Model(&models.InviteCode{}).Where("status = ? AND \"group\" = ?", models.InviteStatusNew, group)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
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

// ListUnexportedInvites returns invites which have not been exported yet
func (s *InviteService) ListUnexportedInvites(limit int) ([]*models.InviteCode, error) {
	var invites []*models.InviteCode
	q := s.db.Model(&models.InviteCode{}).Where("exported_at IS NULL")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Order("created_at ASC").Find(&invites).Error; err != nil {
		return nil, err
	}
	return invites, nil
}

// MarkInvitesExported sets exported_at for the given invite IDs
func (s *InviteService) MarkInvitesExported(ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.Model(&models.InviteCode{}).Where("id IN ?", ids).Update("exported_at", now).Error
}
