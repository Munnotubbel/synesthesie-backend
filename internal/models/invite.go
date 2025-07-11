package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	InviteStatusNew        = "new"
	InviteStatusViewed     = "viewed"
	InviteStatusRegistered = "registered"
	InviteStatusInactive   = "inactive"
)

type InviteCode struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Code         string     `gorm:"uniqueIndex;not null" json:"code"`
	Status       string     `gorm:"type:varchar(20);not null;default:'new'" json:"status"`
	RegisteredBy *uuid.UUID `gorm:"type:uuid" json:"registered_by,omitempty"`
	ViewedAt     *time.Time `json:"viewed_at,omitempty"`
	RegisteredAt *time.Time `json:"registered_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// Relations
	User *User `gorm:"foreignKey:RegisteredBy" json:"user,omitempty"`
}

func (i *InviteCode) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}

	// Generate secure random code if not provided
	if i.Code == "" {
		code, err := generateSecureCode(32)
		if err != nil {
			return err
		}
		i.Code = code
	}

	return nil
}

// generateSecureCode generates a cryptographically secure random code
func generateSecureCode(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// MarkAsViewed marks the invite code as viewed
func (i *InviteCode) MarkAsViewed() {
	now := time.Now()
	i.Status = InviteStatusViewed
	i.ViewedAt = &now
}

// MarkAsRegistered marks the invite code as used by a specific user
func (i *InviteCode) MarkAsRegistered(userID uuid.UUID) {
	now := time.Now()
	i.Status = InviteStatusRegistered
	i.RegisteredBy = &userID
	i.RegisteredAt = &now
}

// Deactivate marks the invite as inactive
func (i *InviteCode) Deactivate() {
	i.Status = InviteStatusInactive
}

// CanBeViewed checks if the invite code can be viewed
func (i *InviteCode) CanBeViewed() bool {
	return i.Status == InviteStatusNew
}

// CanBeUsedForRegistration checks if the code can be used for registration
func (i *InviteCode) CanBeUsedForRegistration() bool {
	return i.Status == InviteStatusViewed
}
