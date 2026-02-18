package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Image represents an image in the gallery with metadata
// References the existing Asset model for S3 storage information
type Image struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	AssetID     uuid.UUID `gorm:"type:uuid;not null" json:"asset_id"`
	Title       string    `gorm:"size:255" json:"title"`
	Description string    `gorm:"size:1000" json:"description"`
	Visibility  string    `gorm:"size:16;default:'private'" json:"visibility"` // private|public

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relation to Asset (existing model)
	Asset *Asset `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
}

// BeforeCreate generates a UUID if not set
func (i *Image) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}
