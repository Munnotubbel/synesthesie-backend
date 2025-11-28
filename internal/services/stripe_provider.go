package services

import (
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

// StripeProvider implements PaymentProvider for Stripe
type StripeProvider struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewStripeProvider creates a new Stripe payment provider
func NewStripeProvider(cfg *config.Config, db *gorm.DB) *StripeProvider {
	stripe.Key = cfg.StripeSecretKey
	return &StripeProvider{
		cfg: cfg,
		db:  db,
	}
}

// GetProviderName returns "stripe"
func (p *StripeProvider) GetProviderName() string {
	return "stripe"
}

// CreateCheckout creates a Stripe checkout session
func (p *StripeProvider) CreateCheckout(ticket *models.Ticket, event *models.Event, user *models.User, totalAmount float64) (string, error) {
	// Build line items
	lineItems := []*stripe.CheckoutSessionLineItemParams{
		{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("eur"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(event.Name),
					Description: stripe.String(fmt.Sprintf("Ticket für %s", event.Name)),
				},
				UnitAmount: stripe.Int64(int64(ticket.Price * 100)),
			},
			Quantity: stripe.Int64(1),
		},
	}

	// Add pickup service if included
	if ticket.IncludesPickup {
		lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("eur"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String("Abholservice"),
					Description: stripe.String(fmt.Sprintf("Abholung von: %s", ticket.PickupAddress)),
				},
				UnitAmount: stripe.Int64(int64(ticket.PickupPrice * 100)),
			},
			Quantity: stripe.Int64(1),
		})
	}

	// Build URLs with ticket_id
	successURL := fmt.Sprintf("%s?ticket_id=%s&session_id={CHECKOUT_SESSION_ID}", p.cfg.StripeSuccessURL, ticket.ID.String())
	cancelURL := fmt.Sprintf("%s?ticket_id=%s", p.cfg.StripeCancelURL, ticket.ID.String())

	// Create Stripe checkout session
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice(p.cfg.StripePaymentMethods),
		LineItems:          lineItems,
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:         stripe.String(successURL),
		CancelURL:          stripe.String(cancelURL),
		CustomerEmail:      stripe.String(user.Email),
		Metadata: map[string]string{
			"ticket_id": ticket.ID.String(),
			"user_id":   user.ID.String(),
			"event_id":  event.ID.String(),
		},
	}

	// Enable automatic payment methods if configured
	if p.cfg.StripeAutomaticPaymentMethods {
		params.PaymentMethodTypes = nil
		params.PaymentMethodOptions = &stripe.CheckoutSessionPaymentMethodOptionsParams{
			Card: &stripe.CheckoutSessionPaymentMethodOptionsCardParams{},
		}
	}

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create Stripe session: %w", err)
	}

	// Save Stripe session ID to ticket
	ticket.StripeSessionID = sess.ID
	ticket.PaymentProvider = "stripe"
	if err := p.db.Save(ticket).Error; err != nil {
		return "", fmt.Errorf("failed to save ticket: %w", err)
	}

	return sess.URL, nil
}

// ProcessRefund processes a Stripe refund
func (p *StripeProvider) ProcessRefund(ticket *models.Ticket, amount float64) error {
	if ticket.StripePaymentIntentID == "" {
		return fmt.Errorf("no Stripe payment intent ID found")
	}

	_, err := refund.New(&stripe.RefundParams{
		PaymentIntent: stripe.String(ticket.StripePaymentIntentID),
		Amount:        stripe.Int64(int64(amount * 100)),
	})

	if err != nil {
		return fmt.Errorf("failed to process Stripe refund: %w", err)
	}

	return nil
}

// CheckAndCaptureOrder checks if a Stripe payment was completed (for active polling)
// Stripe auto-captures, so we just check the session status
func (p *StripeProvider) CheckAndCaptureOrder(ticket *models.Ticket) bool {
	if ticket.StripeSessionID == "" {
		return false
	}

	// Get Stripe session
	sess, err := session.Get(ticket.StripeSessionID, nil)
	if err != nil {
		return false
	}

	// Check if payment was completed
	if sess.PaymentStatus == "paid" {
		// Get payment intent ID
		paymentIntentID := ""
		if sess.PaymentIntent != nil {
			paymentIntentID = sess.PaymentIntent.ID
		}

		// Update ticket to paid
		updates := map[string]interface{}{
			"status":                   "paid",
			"cancelled_at":             nil,
			"stripe_payment_intent_id": paymentIntentID,
		}

		if err := p.db.Model(&models.Ticket{}).Where("id = ?", ticket.ID).Updates(updates).Error; err != nil {
			return false
		}

		return true
	}

	return false
}

