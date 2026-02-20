package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MusicSet represents a DJ set (single audio file, 1-3 hours)
type MusicSet struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Title       string     `gorm:"size:255;not null" json:"title"`
	Description string     `gorm:"size:2000" json:"description"`
	Visibility  string     `gorm:"size:16;default:'private'" json:"visibility"` // private|public
	AssetID     *uuid.UUID `gorm:"type:uuid" json:"asset_id"`                   // Nullable - set when file is uploaded
	Duration    int        `gorm:"default:0" json:"duration"`                    // Duration in seconds

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relation to the audio asset
	Asset *Asset `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
}

// BeforeCreate generates a UUID if not set
func (m *MusicSet) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
