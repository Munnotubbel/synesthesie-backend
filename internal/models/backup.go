package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Backup struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Filename     string     `gorm:"not null" json:"filename"`                   // z.B. synesthesie_2025-01-15T12-00-00Z.sql.gz
	S3Key        string     `gorm:"not null" json:"s3_key"`                     // z.B. db/synesthesie/2025-01-15T12-00-00Z.sql.gz
	SizeBytes    int64      `json:"size_bytes"`                                 // Dateigröße in Bytes
	Status       string     `gorm:"not null;default:'completed'" json:"status"` // completed, failed, in_progress
	Type         string     `gorm:"not null;default:'automatic'" json:"type"`   // automatic, manual
	StartedAt    time.Time  `gorm:"not null" json:"started_at"`                 // Backup-Start
	CompletedAt  *time.Time `json:"completed_at,omitempty"`                     // Backup-Ende
	ErrorMessage string     `json:"error_message,omitempty"`                    // Fehlermeldung bei failed
	CreatedBy    *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`      // UserID wenn manuell
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (b *Backup) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.StartedAt.IsZero() {
		b.StartedAt = time.Now()
	}
	return nil
}
