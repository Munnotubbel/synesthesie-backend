package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PhoneVerification struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID     uuid.UUID  `gorm:"type:uuid;index;not null"`
	Mobile     string     `gorm:"not null;index"`
	Code       string     `gorm:"not null"`
	ExpiresAt  time.Time  `gorm:"not null"`
	ConsumedAt *time.Time `gorm:"default:null"`
	Attempts   int        `gorm:"not null;default:0"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (p *PhoneVerification) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
