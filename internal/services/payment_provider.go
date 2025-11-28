package services

import (
	"github.com/synesthesie/backend/internal/models"
)

// PaymentProvider defines the interface for payment providers (Stripe, PayPal, etc.)
type PaymentProvider interface {
	// CreateCheckout creates a checkout session and returns the checkout URL
	CreateCheckout(ticket *models.Ticket, event *models.Event, user *models.User, totalAmount float64) (checkoutURL string, err error)

	// ProcessRefund processes a refund for a ticket
	ProcessRefund(ticket *models.Ticket, amount float64) error

	// CheckAndCaptureOrder checks payment status and captures if approved (for active polling)
	CheckAndCaptureOrder(ticket *models.Ticket) bool

	// GetProviderName returns the name of the provider ("stripe" or "paypal")
	GetProviderName() string
}

