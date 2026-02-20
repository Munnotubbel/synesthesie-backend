package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MusicSet represents a DJ set (single audio file, 1-3 hours)
type MusicSet struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Title         string     `gorm:"size:255;not null" json:"title"`
	Description   string     `gorm:"size:2000" json:"description"`
	Visibility    string     `gorm:"size:16;default:'private'" json:"visibility"`           // private|public
	AssetID       *uuid.UUID `gorm:"type:uuid" json:"asset_id"`                             // Nullable - points to the HLS master.m3u8 asset
	SourceAssetID *uuid.UUID `gorm:"type:uuid" json:"source_asset_id"`                      // Nullable - points to the original raw audio file (for downloads)
	Duration      int        `gorm:"default:0" json:"duration"`                             // Duration in seconds
	PlayCount     int        `gorm:"default:0" json:"play_count"`                           // Number of plays
	DownloadCount int        `gorm:"default:0" json:"download_count"`                       // Number of downloads
	PeakData      []float32  `gorm:"type:jsonb;serializer:json" json:"peak_data,omitempty"` // 1000 normalized data points for waveform

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	Asset       *Asset `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	SourceAsset *Asset `gorm:"foreignKey:SourceAssetID" json:"source_asset,omitempty"`
}

// BeforeCreate generates a UUID if not set
func (m *MusicSet) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
