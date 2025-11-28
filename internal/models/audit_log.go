package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog represents an admin action log entry
type AuditLog struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AdminID    uuid.UUID `gorm:"type:uuid;not null" json:"admin_id"`
	Admin      *User     `gorm:"foreignKey:AdminID" json:"admin,omitempty"`
	Action     string    `gorm:"type:varchar(100);not null" json:"action"` // e.g., "cancel_ticket", "delete_user"
	TargetType string    `gorm:"type:varchar(50);not null" json:"target_type"` // e.g., "ticket", "user", "event"
	TargetID   uuid.UUID `gorm:"type:uuid;not null" json:"target_id"`
	Details    string    `gorm:"type:text" json:"details,omitempty"` // JSON string with additional info
	IPAddress  string    `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	UserAgent  string    `gorm:"type:text" json:"user_agent,omitempty"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName specifies the table name for AuditLog
func (AuditLog) TableName() string {
	return "audit_logs"
}

