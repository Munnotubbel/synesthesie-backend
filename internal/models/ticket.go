package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Ticket struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID                uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	EventID               uuid.UUID  `gorm:"type:uuid;not null" json:"event_id"`
	Status                string     `gorm:"not null;default:'pending'" json:"status"` // pending, paid, cancelled, refunded
	Price                 float64    `gorm:"not null" json:"price"`
	IncludesPickup        bool       `gorm:"default:false" json:"includes_pickup"`
	PickupPrice           float64    `json:"pickup_price,omitempty"`
	PickupAddress         string     `json:"pickup_address,omitempty"`
	TotalAmount           float64    `gorm:"not null" json:"total_amount"`

	// Payment Provider (stripe or paypal)
	PaymentProvider string `gorm:"type:varchar(20);default:'stripe'" json:"payment_provider"`

	// Stripe Payment Details
	StripeSessionID       string `json:"-"`
	StripePaymentIntentID string `json:"-"`

	// PayPal Payment Details
	PayPalOrderID   string `gorm:"type:varchar(255)" json:"-"`
	PayPalCaptureID string `gorm:"type:varchar(255)" json:"-"`

	RefundedAmount  float64    `json:"refunded_amount,omitempty"`
	RefundedAt      *time.Time `json:"refunded_at,omitempty"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// Relations
	User  User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Event Event `gorm:"foreignKey:EventID" json:"event,omitempty"`
}

func (t *Ticket) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// CalculateTotalAmount calculates the total amount including pickup service
func (t *Ticket) CalculateTotalAmount() {
	t.TotalAmount = t.Price
	if t.IncludesPickup {
		t.TotalAmount += t.PickupPrice
	}
}

// CanBeCancelled checks if the ticket can be cancelled (7 days before event)
func (t *Ticket) CanBeCancelled() bool {
	if t.Status != "paid" {
		return false
	}

	// Load event if not loaded
	if t.Event.ID == uuid.Nil {
		return false
	}

	// Check if event is at least 7 days away
	daysUntilEvent := time.Until(t.Event.DateFrom).Hours() / 24
	return daysUntilEvent >= 7
}

// GetRefundAmount calculates the refund amount (50% if cancelled by user)
func (t *Ticket) GetRefundAmount(fullRefund bool) float64 {
	if fullRefund {
		return t.TotalAmount
	}
	return t.TotalAmount * 0.5
}
