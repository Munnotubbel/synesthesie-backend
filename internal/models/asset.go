package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AssetVisibility string

const (
	AssetVisibilityPrivate AssetVisibility = "private"
	AssetVisibilityPublic  AssetVisibility = "public"
)

// Asset represents a stored file (image or audio)
type Asset struct {
	ID         uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	OwnerID    *uuid.UUID      `gorm:"type:uuid" json:"owner_id,omitempty"`
	Key        string          `gorm:"size:512;uniqueIndex" json:"key"` // storage path
	Filename   string          `gorm:"size:255" json:"filename"`
	MimeType   string          `gorm:"size:120" json:"mime_type"`
	SizeBytes  int64           `json:"size_bytes"`
	Checksum   string          `gorm:"size:128" json:"checksum"`
	Visibility AssetVisibility `gorm:"size:16;default:private" json:"visibility"`
	Tags       string          `gorm:"size:512" json:"tags"` // optional CSV tags

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (a *Asset) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
