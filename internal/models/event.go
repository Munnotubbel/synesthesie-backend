package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Event struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name            string    `gorm:"not null" json:"name"`
	Description     string    `gorm:"type:text" json:"description"`
	DateFrom        time.Time `gorm:"not null" json:"date_from"`
	DateTo          time.Time `gorm:"not null" json:"date_to"`
	TimeFrom        string    `gorm:"not null" json:"time_from"` // Format: "HH:MM"
	TimeTo          string    `gorm:"not null" json:"time_to"`   // Format: "HH:MM"
	MaxParticipants int       `gorm:"not null" json:"max_participants"`
	Price           float64   `gorm:"not null" json:"price"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Relations
	Tickets []Ticket `gorm:"foreignKey:EventID" json:"tickets,omitempty"`
}

func (e *Event) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// GetAvailableSpots returns the number of available spots for the event
func (e *Event) GetAvailableSpots(db *gorm.DB) int {
	var bookedCount int64
	db.Model(&Ticket{}).Where("event_id = ? AND status IN ?", e.ID, []string{"paid", "pending"}).Count(&bookedCount)
	return e.MaxParticipants - int(bookedCount)
}

type SystemSetting struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Key       string    `gorm:"uniqueIndex;not null"`
	Value     string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *SystemSetting) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
