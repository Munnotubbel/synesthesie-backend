package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MusicSet represents a collection of music tracks (album, DJ set, etc.)
type MusicSet struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Title       string    `gorm:"size:255;not null" json:"title"`
	Description string    `gorm:"size:2000" json:"description"`
	Visibility  string    `gorm:"size:16;default:'private'" json:"visibility"` // private|public

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relation to tracks (has-many)
	Tracks []MusicTrack `gorm:"foreignKey:MusicSetID;constraint:OnDelete:CASCADE" json:"tracks,omitempty"`
}

// BeforeCreate generates a UUID if not set
func (m *MusicSet) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// MusicTrack represents an individual audio track within a MusicSet
type MusicTrack struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	MusicSetID uuid.UUID `gorm:"type:uuid;not null;index" json:"music_set_id"`
	AssetID    uuid.UUID `gorm:"type:uuid;not null" json:"asset_id"`
	Title      string    `gorm:"size:255" json:"title"`
	Artist     string    `gorm:"size:255" json:"artist"`
	TrackOrder int       `gorm:"default:0" json:"track_order"` // Order within the set
	Duration   int       `gorm:"default:0" json:"duration"`    // Duration in seconds (optional)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	MusicSet *MusicSet `gorm:"foreignKey:MusicSetID" json:"music_set,omitempty"`
	Asset    *Asset    `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
}

// BeforeCreate generates a UUID if not set
func (t *MusicTrack) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
