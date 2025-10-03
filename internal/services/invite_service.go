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

// generateRandomPublicID generates a random public ID with format: P + 4 alphanumeric characters (e.g., PA12, P3X9)
func generateRandomPublicID() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	result := make([]byte, 5)
	result[0] = 'P'
	for i := 0; i < 4; i++ {
		result[i+1] = chars[int(b[i])%len(chars)]
	}
	return string(result), nil
}

// generateUniqueRandomPublicID generates a unique random public ID for the plus group
func (s *InviteService) generateUniqueRandomPublicID(tx *gorm.DB, maxAttempts int) (string, error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		pubID, err := generateRandomPublicID()
		if err != nil {
			return "", err
		}
		// Check if this public_id already exists
		var count int64
		if err := tx.Model(&models.InviteCode{}).Where("public_id = ?", pubID).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return pubID, nil
		}
	}
	return "", errors.New("failed to generate unique public_id after max attempts")
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

// CreateInviteCodeWithGroup creates a new invite code for a specific group (bubble|guests|plus)
// guests → PublicID NULL; bubble → PublicID sequential numeric (1..1000); plus → PublicID random (B + 4 chars)
func (s *InviteService) CreateInviteCodeWithGroup(group string) (*models.InviteCode, error) {
	if group != "bubble" && group != "guests" && group != "plus" {
		return nil, errors.New("invalid group; must be 'bubble', 'guests' or 'plus'")
	}
	if group == "guests" {
		return s.CreateInviteCode()
	}
	if group == "plus" {
		// plus: generate random PublicID (B + 4 alphanumeric)
		var created *models.InviteCode
		err := s.db.Transaction(func(tx *gorm.DB) error {
			pubID, err := s.generateUniqueRandomPublicID(tx, 100)
			if err != nil {
				return err
			}
			code, err := generateSecureCode(32)
			if err != nil {
				return err
			}
			invite := &models.InviteCode{Status: models.InviteStatusNew, Group: "plus", Code: code, PublicID: &pubID}
			if err := tx.Create(invite).Error; err != nil {
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
	if group != "bubble" && group != "guests" && group != "plus" {
		return nil, errors.New("invalid group; must be 'bubble', 'guests' or 'plus'")
	}
	if group == "guests" {
		return s.CreateBulkInviteCodes(count)
	}
	if group == "plus" {
		// plus: generate random PublicID for each
		var invites []*models.InviteCode
		err := s.db.Transaction(func(tx *gorm.DB) error {
			invites = make([]*models.InviteCode, 0, count)
			for i := 0; i < count; i++ {
				pubID, err := s.generateUniqueRandomPublicID(tx, 100)
				if err != nil {
					return err
				}
				code, err := generateSecureCode(32)
				if err != nil {
					return err
				}
				invites = append(invites, &models.InviteCode{Status: models.InviteStatusNew, Group: "plus", Code: code, PublicID: &pubID})
			}
			if err := tx.Create(&invites).Error; err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return invites, nil
	}
	// bubble: sequential
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

	// Status filter takes precedence over includeUsed
	if status != "" {
		switch status {
		case models.InviteStatusNew, models.InviteStatusViewed, models.InviteStatusRegistered, models.InviteStatusInactive:
			query = query.Where("status = ?", status)
		default:
			return nil, 0, errors.New("invalid status; must be new|viewed|registered|inactive")
		}
	} else if !includeUsed {
		// Only apply includeUsed filter if no explicit status filter is set
		query = query.Where("status != ?", models.InviteStatusRegistered)
	}

	if group != "" {
		if group != "bubble" && group != "guests" && group != "plus" {
			return nil, 0, errors.New("invalid group; must be 'bubble', 'guests' or 'plus'")
		}
		query = query.Where("\"group\" = ?", group)
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
	if group != "bubble" && group != "guests" && group != "plus" {
		return nil, 0, errors.New("invalid group; must be 'bubble', 'guests' or 'plus'")
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

// GetInviteStats returns statistics about invite codes with registered users
func (s *InviteService) GetInviteStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

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

	// Viewed invites (considered "used" in UI context)
	var viewed int64
	if err := s.db.Model(&models.InviteCode{}).Where("status = ?", models.InviteStatusViewed).Count(&viewed).Error; err != nil {
		return nil, err
	}
	stats["viewed"] = viewed
	stats["used"] = viewed // Alias: viewed codes are "used" in the sense that they've been accessed

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

	// Get all registered users (with invite code preloaded)
	var registeredInvites []*models.InviteCode
	if err := s.db.Preload("User").Where("status = ?", models.InviteStatusRegistered).Find(&registeredInvites).Error; err != nil {
		return nil, err
	}

	registeredUsers := make([]map[string]interface{}, 0, len(registeredInvites))
	for _, invite := range registeredInvites {
		if invite.User != nil {
			registeredUsers = append(registeredUsers, map[string]interface{}{
				"id":         invite.User.ID,
				"username":   invite.User.Username,
				"name":       invite.User.Name,
				"email":      invite.User.Email,
				"group":      invite.User.Group,
				"invite_id":  invite.ID,
				"public_id":  invite.PublicID,
				"created_at": invite.User.CreatedAt,
			})
		}
	}
	stats["registered_users"] = registeredUsers

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
