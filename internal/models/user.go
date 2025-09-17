package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID                 uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Username           string    `gorm:"uniqueIndex;not null" json:"username"`
	Email              string    `gorm:"uniqueIndex;not null" json:"email"`
	Password           string    `gorm:"not null" json:"-"`
	Name               string    `gorm:"not null" json:"name"`
	Mobile             string    `json:"mobile"`
	MobileVerified     bool      `gorm:"default:false" json:"mobile_verified"`
	Drink1             string    `json:"drink1"`
	Drink2             string    `json:"drink2"`
	Drink3             string    `json:"drink3"`
	IsAdmin            bool      `gorm:"default:false" json:"is_admin"`
	IsActive           bool      `gorm:"default:true" json:"is_active"`
	RegisteredWithCode string    `json:"registered_with_code,omitempty"`
	Group              string    `gorm:"type:varchar(20);not null;default:'guests'" json:"group"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Relations
	Tickets []Ticket `gorm:"foreignKey:UserID" json:"tickets,omitempty"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null"`
	Token     string    `gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time

	// Relations
	User User `gorm:"foreignKey:UserID"`
}

func (r *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
