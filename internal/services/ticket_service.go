package services

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type TicketService struct {
	db              *gorm.DB
	cfg             *config.Config
	stripeProvider  PaymentProvider
	paypalProvider  PaymentProvider
}

func NewTicketService(db *gorm.DB, cfg *config.Config) *TicketService {
	if cfg != nil {
		stripe.Key = cfg.StripeSecretKey
	}

	service := &TicketService{
		db:  db,
		cfg: cfg,
	}

	// Initialize Stripe provider (always available)
	service.stripeProvider = NewStripeProvider(cfg, db)

	// Initialize PayPal provider (if enabled)
	if cfg != nil && cfg.PayPalEnabled && cfg.PayPalClientID != "" {
		paypalProvider, err := NewPayPalProvider(cfg, db)
		if err == nil {
			service.paypalProvider = paypalProvider
		}
	}

	return service
}

// ListPickupTickets returns tickets that include pickup service.
// statusFilter: "paid" (default) | "all" (includes pending & paid)
func (s *TicketService) ListPickupTickets(eventID *uuid.UUID, statusFilter string) ([]*models.Ticket, error) {
	query := s.db.Preload("User").Where("includes_pickup = ?", true)
	if eventID != nil {
		query = query.Where("event_id = ?", *eventID)
	}
	switch statusFilter {
	case "all":
		query = query.Where("status IN ?", []string{"pending", "paid"})
	default:
		query = query.Where("status = ?", "paid")
	}
	// Exclude cancelled/refunded implicitly via status filter above
	var tickets []*models.Ticket
	if err := query.Order("created_at DESC").Find(&tickets).Error; err != nil {
		return nil, err
	}
	return tickets, nil
}

// CreateTicket creates a new ticket for a user
func (s *TicketService) CreateTicket(userID, eventID uuid.UUID, includesPickup bool, pickupAddress string) (*models.Ticket, *stripe.CheckoutSession, error) {
	// Check if user already has a ticket for this event
	var existingTicket models.Ticket
	err := s.db.Where("user_id = ? AND event_id = ? AND status IN ?", userID, eventID, []string{"pending", "paid"}).First(&existingTicket).Error
	if err == nil {
		return nil, nil, errors.New("user already has a ticket for this event")
	}

	// Get event details
	var event models.Event
	if err := s.db.First(&event, eventID).Error; err != nil {
		return nil, nil, errors.New("event not found")
	}

	// Get user for group
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, nil, errors.New("user not found")
	}

	// Enforce allowed_group
	if event.AllowedGroup == "guests" && user.Group != "guests" {
		return nil, nil, errors.New("event not available for your group")
	}
	if event.AllowedGroup == "bubble" && user.Group != "bubble" {
		return nil, nil, errors.New("event not available for your group")
	}
	if event.AllowedGroup == "plus" && user.Group != "plus" {
		return nil, nil, errors.New("event not available for your group")
	}

	// Check availability
	availableSpots := event.GetAvailableSpots(s.db)
	if availableSpots <= 0 {
		return nil, nil, errors.New("event is fully booked")
	}

	// Determine base price based on user group and event prices
	basePrice := event.GuestsPrice
	if user.Group == "bubble" {
		basePrice = event.BubblePrice
	}
	if user.Group == "plus" {
		basePrice = event.PlusPrice
	}

	// Get pickup service price
	pickupPrice := 0.0
	if includesPickup {
		var setting models.SystemSetting
		if err := s.db.Where("key = ?", "pickup_service_price").First(&setting).Error; err == nil {
			fmt.Sscanf(setting.Value, "%f", &pickupPrice)
		}
	}

	// Create ticket
	ticket := &models.Ticket{
		UserID:         userID,
		EventID:        eventID,
		Status:         "pending",
		Price:          basePrice,
		IncludesPickup: includesPickup,
		PickupPrice:    pickupPrice,
		PickupAddress:  pickupAddress,
	}
	ticket.CalculateTotalAmount()

	// Save ticket
	if err := s.db.Create(ticket).Error; err != nil {
		return nil, nil, err
	}

	// Create Stripe checkout session
	checkoutSession, err := s.createStripeCheckoutSession(ticket, &event)
	if err != nil {
		// Delete ticket if Stripe session creation fails
		s.db.Delete(ticket)
		return nil, nil, err
	}

	// Update ticket with Stripe session ID
	ticket.StripeSessionID = checkoutSession.ID
	if err := s.db.Save(ticket).Error; err != nil {
		return nil, nil, err
	}

	return ticket, checkoutSession, nil
}

// GetPickupServicePrice returns current pickup service price for user-facing endpoints
func (s *TicketService) GetPickupServicePrice() (float64, error) {
	var setting models.SystemSetting
	if err := s.db.Where("key = ?", "pickup_service_price").First(&setting).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 10.00, nil
		}
		return 0, err
	}
	var price float64
	_, _ = fmt.Sscanf(setting.Value, "%f", &price)
	return price, nil
}

// createStripeCheckoutSession creates a Stripe checkout session
func (s *TicketService) createStripeCheckoutSession(ticket *models.Ticket, event *models.Event) (*stripe.CheckoutSession, error) {
	lineItems := []*stripe.CheckoutSessionLineItemParams{
		{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("eur"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(fmt.Sprintf("Ticket für %s", event.Name)),
					Description: stripe.String(fmt.Sprintf("Event am %s", event.DateFrom.Format("02.01.2006"))),
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
					Name:        stripe.String("Abhol- und Bringservice"),
					Description: stripe.String(fmt.Sprintf("Abholadresse: %s", ticket.PickupAddress)),
				},
				UnitAmount: stripe.Int64(int64(ticket.PickupPrice * 100)),
			},
			Quantity: stripe.Int64(1),
		})
	}

	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems:         lineItems,
		SuccessURL:        stripe.String(fmt.Sprintf("%s?ticket_id=%s", s.cfg.StripeSuccessURL, ticket.ID)),
		CancelURL:         stripe.String(fmt.Sprintf("%s?ticket_id=%s", s.cfg.StripeCancelURL, ticket.ID)),
		ClientReferenceID: stripe.String(ticket.ID.String()),
		Metadata: map[string]string{
			"ticket_id": ticket.ID.String(),
			"user_id":   ticket.UserID.String(),
			"event_id":  ticket.EventID.String(),
		},
	}

	// Configure allowed payment methods explicitly from config
	if len(s.cfg.StripePaymentMethods) > 0 {
		params.PaymentMethodTypes = stripe.StringSlice(s.cfg.StripePaymentMethods)
	}

	return session.New(params)
}

// ConfirmPayment confirms a ticket payment after successful Stripe webhook
// Also handles Grace Period: reactivates tickets in "pending_cancellation" status
func (s *TicketService) ConfirmPayment(ticketID uuid.UUID, paymentIntentID string) error {
	// First try normal pending tickets
	result := s.db.Model(&models.Ticket{}).
		Where("id = ? AND status = ?", ticketID, "pending").
		Updates(map[string]interface{}{
			"status":                   "paid",
			"stripe_payment_intent_id": paymentIntentID,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		return nil // Success - normal flow
	}

	// No rows affected - check if ticket is in pending_cancellation (GRACE PERIOD)
	result = s.db.Model(&models.Ticket{}).
		Where("id = ? AND status = ?", ticketID, "pending_cancellation").
		Updates(map[string]interface{}{
			"status":                   "paid",
			"stripe_payment_intent_id": paymentIntentID,
			"cancelled_at":             nil, // Clear cancellation timestamp
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		log.Printf("✅ Stripe webhook: Ticket %s reactivated from pending_cancellation to paid (GRACE PERIOD)", ticketID)
		return nil
	}

	// Ticket not found in pending or pending_cancellation
	return errors.New("ticket not found or already paid")
}

// CancelPendingBySystem cancels a pending ticket (e.g., after Stripe session expiration)
func (s *TicketService) CancelPendingBySystem(ticketID uuid.UUID, reason string) error {
	updates := map[string]interface{}{
		"status":       "cancelled",
		"cancelled_at": time.Now(),
	}
	return s.db.Model(&models.Ticket{}).Where("id = ? AND status = ?", ticketID, "pending").Updates(updates).Error
}

// RetryPendingCheckout generates a new checkout URL for a pending ticket
func (s *TicketService) RetryPendingCheckout(ticketID, userID uuid.UUID) (string, string, error) {
	var ticket models.Ticket

	// Get ticket with event and user
	if err := s.db.Preload("Event").Preload("User").
		Where("id = ? AND user_id = ?", ticketID, userID).
		First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", errors.New("ticket not found")
		}
		return "", "", err
	}

	// Only allow retry for pending tickets
	if ticket.Status != "pending" {
		return "", "", fmt.Errorf("ticket is not pending (status: %s)", ticket.Status)
	}

	// Determine payment provider
	paymentProvider := ticket.PaymentProvider
	if paymentProvider == "" {
		paymentProvider = "stripe" // Default fallback
	}

	var checkoutURL string
	var err error

	// Generate new checkout URL based on provider
	switch paymentProvider {
	case "stripe":
		if s.stripeProvider == nil {
			return "", "", errors.New("Stripe provider not available")
		}
		checkoutURL, err = s.stripeProvider.CreateCheckout(&ticket, &ticket.Event, &ticket.User, ticket.TotalAmount)
	case "paypal":
		if s.paypalProvider == nil {
			return "", "", errors.New("PayPal is not enabled")
		}
		checkoutURL, err = s.paypalProvider.CreateCheckout(&ticket, &ticket.Event, &ticket.User, ticket.TotalAmount)
	default:
		return "", "", fmt.Errorf("unsupported payment provider: %s", paymentProvider)
	}

	if err != nil {
		return "", "", fmt.Errorf("failed to create checkout: %w", err)
	}

	return checkoutURL, paymentProvider, nil
}

// CleanupStalePending cancels tickets that stayed pending longer than the configured TTL
func (s *TicketService) CleanupStalePending() (int64, error) {
	if s.cfg == nil || s.cfg.PendingTicketTTLMinutes <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(s.cfg.PendingTicketTTLMinutes) * time.Minute)
	res := s.db.Model(&models.Ticket{}).
		Where("status = ? AND created_at < ?", "pending", cutoff).
		Updates(map[string]interface{}{
			"status":       "cancelled",
			"cancelled_at": time.Now(),
		})
	return res.RowsAffected, res.Error
}

// CleanupPendingCancellations finalizes tickets in "pending_cancellation" after grace period
// Grace period: 5 minutes to allow PayPal/Stripe payments to complete
func (s *TicketService) CleanupPendingCancellations() (int64, error) {
	gracePeriodMinutes := 5 // 5 minutes grace period
	cutoff := time.Now().Add(-time.Duration(gracePeriodMinutes) * time.Minute)

	res := s.db.Model(&models.Ticket{}).
		Where("status = ? AND cancelled_at < ?", "pending_cancellation", cutoff).
		Update("status", "cancelled")

	if res.RowsAffected > 0 {
		log.Printf("CleanupPendingCancellations: Finalized %d cancelled tickets after grace period", res.RowsAffected)
	}

	return res.RowsAffected, res.Error
}

// CheckPendingCancellations actively polls payment status for tickets in pending_cancellation
// This ensures payments are captured even if webhooks fail (Grace Period active polling)
func (s *TicketService) CheckPendingCancellations() (int64, error) {
	var tickets []models.Ticket

	// Get all tickets in pending_cancellation that are less than 5 minutes old
	gracePeriodMinutes := 5
	cutoff := time.Now().Add(-time.Duration(gracePeriodMinutes) * time.Minute)

	err := s.db.Where("status = ? AND cancelled_at > ?", "pending_cancellation", cutoff).
		Find(&tickets).Error

	if err != nil {
		return 0, err
	}

	if len(tickets) == 0 {
		return 0, nil
	}

	log.Printf("🔍 CheckPendingCancellations: Checking %d tickets in grace period", len(tickets))

	var reactivated int64

	for _, ticket := range tickets {
		// Check payment status based on provider
		switch ticket.PaymentProvider {
		case "stripe":
			if ticket.StripeSessionID != "" {
				if s.checkStripePaymentStatus(&ticket) {
					reactivated++
				}
			}
		case "paypal":
			if ticket.PayPalOrderID != "" {
				if s.checkPayPalPaymentStatus(&ticket) {
					reactivated++
				}
			}
		}
	}

	if reactivated > 0 {
		log.Printf("✅ CheckPendingCancellations: Reactivated %d tickets after finding completed payments", reactivated)
	}

	return reactivated, nil
}

// FastCheckRecentPending checks very recent pending tickets frequently (0-30 seconds old)
// This provides fast feedback for users actively waiting at checkout
func (s *TicketService) FastCheckRecentPending() (int64, error) {
	var tickets []models.Ticket

	// Get pending tickets less than 30 seconds old
	cutoff := time.Now().Add(-30 * time.Second)

	err := s.db.Where("status = ? AND created_at > ?", "pending", cutoff).
		Find(&tickets).Error

	if err != nil {
		return 0, err
	}

	if len(tickets) == 0 {
		return 0, nil
	}

	log.Printf("🔍 Fast-poll: Checking %d recent pending tickets", len(tickets))

	var confirmed int64

	for _, ticket := range tickets {
		switch ticket.PaymentProvider {
		case "stripe":
			if ticket.StripeSessionID != "" {
				if s.checkStripePaymentStatus(&ticket) {
					confirmed++
				}
			}
		case "paypal":
			if ticket.PayPalOrderID != "" {
				if s.checkPayPalPaymentStatus(&ticket) {
					confirmed++
				}
			}
		}
	}

	if confirmed > 0 {
		log.Printf("✅ Fast-poll: Confirmed %d payments", confirmed)
	}

	return confirmed, nil
}

// CheckPendingPayments actively polls payment status for pending tickets (30 sec - 30 min old)
// Uses exponential backoff to reduce API calls over time
func (s *TicketService) CheckPendingPayments() (int64, error) {
	var tickets []models.Ticket

	// Get all pending tickets between 30 seconds and 30 minutes old
	maxAge := 30 * time.Minute
	minAge := 30 * time.Second
	cutoffMax := time.Now().Add(-maxAge)
	cutoffMin := time.Now().Add(-minAge)

	err := s.db.Where("status = ? AND created_at > ? AND created_at < ?", "pending", cutoffMax, cutoffMin).
		Find(&tickets).Error

	if err != nil {
		return 0, err
	}

	if len(tickets) == 0 {
		return 0, nil
	}

	var confirmed int64

	for _, ticket := range tickets {
		// Check payment status based on provider
		switch ticket.PaymentProvider {
		case "stripe":
			if ticket.StripeSessionID != "" {
				if s.checkStripePaymentStatus(&ticket) {
					confirmed++
				}
			}
		case "paypal":
			if ticket.PayPalOrderID != "" {
				if s.checkPayPalPaymentStatus(&ticket) {
					confirmed++
				}
			}
		}
	}

	if confirmed > 0 {
		log.Printf("✅ Pending payment check: Confirmed %d payments via active polling", confirmed)
	}

	return confirmed, nil
}

// checkStripePaymentStatus checks if a Stripe payment was completed
func (s *TicketService) checkStripePaymentStatus(ticket *models.Ticket) bool {
	// Get Stripe session
	sess, err := session.Get(ticket.StripeSessionID, nil)
	if err != nil {
		log.Printf("⚠️ Payment check: Failed to get Stripe session for ticket %s: %v", ticket.ID, err)
		return false
	}

	// Check if payment was completed
	if sess.PaymentStatus == "paid" {
		log.Printf("✅ Payment check: Found completed Stripe payment for ticket %s", ticket.ID)

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

		if err := s.db.Model(&models.Ticket{}).Where("id = ?", ticket.ID).Updates(updates).Error; err != nil {
			log.Printf("⚠️ Payment check: Failed to update Stripe ticket %s: %v", ticket.ID, err)
			return false
		}

		log.Printf("✅ Payment check: Stripe ticket %s confirmed as paid", ticket.ID)
		return true
	}

	return false
}

// checkPayPalPaymentStatus checks if a PayPal payment was completed
func (s *TicketService) checkPayPalPaymentStatus(ticket *models.Ticket) bool {
	if s.paypalProvider == nil {
		return false
	}

	// Use the PayPal provider to check and capture order
	return s.paypalProvider.CheckAndCaptureOrder(ticket)
}

// ProactiveConfirmPayment proactively confirms a payment when user returns from payment provider
// This provides instant confirmation without waiting for webhooks (like Shopify, Airbnb)
func (s *TicketService) ProactiveConfirmPayment(ticketID, userID uuid.UUID, paypalToken, paypalPayerID, stripeSessionID string) (bool, string, error) {
	var ticket models.Ticket

	// Get ticket with ownership check
	if err := s.db.Where("id = ? AND user_id = ?", ticketID, userID).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", errors.New("ticket not found")
		}
		return false, "", err
	}

	// Only confirm pending or pending_cancellation tickets
	if ticket.Status != "pending" && ticket.Status != "pending_cancellation" {
		// Already paid or cancelled
		return false, ticket.Status, nil
	}

	log.Printf("🔍 Proactive confirm: Checking payment for ticket %s (status: %s)", ticketID, ticket.Status)

	// Check based on payment provider
	switch ticket.PaymentProvider {
	case "paypal":
		if paypalToken != "" && s.paypalProvider != nil {
			// Proactively check PayPal order status
			if s.paypalProvider.CheckAndCaptureOrder(&ticket) {
				// Reload ticket to get updated status
				s.db.First(&ticket, ticketID)
				log.Printf("✅ Proactive confirm: PayPal payment confirmed for ticket %s (was: %s, now: %s)", ticketID, ticket.Status, "paid")
				return true, "paid", nil
			}
		}

	case "stripe":
		if stripeSessionID != "" {
			// Proactively check Stripe session status
			if s.checkStripePaymentStatus(&ticket) {
				// Reload ticket to get updated status
				s.db.First(&ticket, ticketID)
				log.Printf("✅ Proactive confirm: Stripe payment confirmed for ticket %s", ticketID)
				return true, "paid", nil
			}
		}
	}

	// Payment not yet confirmed
	log.Printf("⏱️ Proactive confirm: Payment not yet confirmed for ticket %s, will retry via polling", ticketID)
	return false, ticket.Status, nil
}

// CancelTicket cancels a paid ticket and processes a refund, or deletes a pending ticket.
// CancelTicket keeps backward compatibility with default 'auto' behaviour
func (s *TicketService) CancelTicket(ticketID, userID uuid.UUID) error {
	return s.CancelTicketWithMode(ticketID, userID, "auto")
}

// AdminCancelTicket cancels a ticket by admin without user ownership check
func (s *TicketService) AdminCancelTicket(ticketID uuid.UUID, mode string) error {
	var ticket models.Ticket

	// Get ticket without user ownership check
	if err := s.db.Preload("Event").Where("id = ?", ticketID).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("ticket not found")
		}
		return err
	}

	switch ticket.Status {
	case "pending":
		// Delete pending ticket
		if err := s.db.Delete(&ticket).Error; err != nil {
			return fmt.Errorf("failed to delete pending ticket: %w", err)
		}
		return nil

	case "paid":
		// Admin cancellation: always allow, refund handling based on mode
		days := 14
		percent := 50
		policyEnabled := false
		if s.cfg != nil {
			policyEnabled = s.cfg.TicketCancellationEnabled
			if s.cfg.TicketCancellationDays > 0 {
				days = s.cfg.TicketCancellationDays
			}
			if s.cfg.TicketCancellationRefundPercent > 0 {
				percent = s.cfg.TicketCancellationRefundPercent
			}
		}

		refundAmount := 0.0
		if policyEnabled && mode != "no_refund" {
			// Check eligibility window
			daysUntilEvent := time.Until(ticket.Event.DateFrom).Hours() / 24
			if int(daysUntilEvent) >= days {
				refundAmount = ticket.TotalAmount * float64(percent) / 100.0

				// Process refund based on payment provider
				if refundAmount > 0 {
					if ticket.PaymentProvider == "paypal" && s.paypalProvider != nil && ticket.PayPalCaptureID != "" {
						// PayPal refund
						if err := s.paypalProvider.ProcessRefund(&ticket, refundAmount); err != nil {
							return fmt.Errorf("failed to process PayPal refund: %w", err)
						}
					} else if ticket.StripePaymentIntentID != "" {
						// Stripe refund (default)
						if _, err := refund.New(&stripe.RefundParams{
							PaymentIntent: stripe.String(ticket.StripePaymentIntentID),
							Amount:        stripe.Int64(int64(refundAmount * 100)),
						}); err != nil {
							return fmt.Errorf("failed to process Stripe refund: %w", err)
						}
					}
				}
			} else if mode == "refund" {
				// Requested refund but not eligible
				return errors.New("refund_not_eligible")
			}
		}

		// Always cancel ticket; only set refund fields if > 0
		now := time.Now()
		updates := map[string]interface{}{
			"status":       "cancelled",
			"cancelled_at": now,
		}
		if refundAmount > 0 {
			updates["refunded_amount"] = refundAmount
			updates["refunded_at"] = now
		}

		if err := s.db.Model(&ticket).Updates(updates).Error; err != nil {
			return err
		}
		return nil

	default:
		return fmt.Errorf("ticket cannot be cancelled with current status: %s", ticket.Status)
	}
}

// CancelTicketWithMode cancels a ticket with an explicit mode:
// mode = 'auto' (default policy), 'refund' (require refund eligibility), 'no_refund' (force cancel without refund)
func (s *TicketService) CancelTicketWithMode(ticketID, userID uuid.UUID, mode string) error {
	var ticket models.Ticket

	// Get ticket with event for paid tickets, or just the ticket for pending ones
	if err := s.db.Preload("Event").Where("id = ? AND user_id = ?", ticketID, userID).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("ticket not found")
		}
		return err
	}

	switch ticket.Status {
	case "pending":
		// If the ticket is pending, the user is cancelling the checkout process.
		// IMPORTANT: We use a GRACE PERIOD to prevent race conditions!
		//
		// Grace Period: 5 minutes
		// - User cancels → Status: "pending_cancellation"
		// - If PayPal/Stripe payment completes within 5 min → Ticket becomes "paid" ✅
		// - After 5 min → Cleanup job sets to "cancelled" permanently
		//
		// This prevents: User cancels → Payment completes → User paid but no ticket!
		now := time.Now()
		ticket.Status = "pending_cancellation"
		ticket.CancelledAt = &now
		if err := s.db.Save(&ticket).Error; err != nil {
			return fmt.Errorf("failed to mark ticket for cancellation: %w", err)
		}
		return nil

	case "paid":
		// Stornierung ist grundsätzlich erlaubt. Refund ist optional per ENV + Frist.
		days := 14
		percent := 50
		policyEnabled := false
		if s.cfg != nil {
			policyEnabled = s.cfg.TicketCancellationEnabled
			if s.cfg.TicketCancellationDays > 0 {
				days = s.cfg.TicketCancellationDays
			}
			if s.cfg.TicketCancellationRefundPercent > 0 {
				percent = s.cfg.TicketCancellationRefundPercent
			}
		}

		refundAmount := 0.0
		if policyEnabled && mode != "no_refund" {
			// Prüfe Fristfenster für Refund
			daysUntilEvent := time.Until(ticket.Event.DateFrom).Hours() / 24
			if int(daysUntilEvent) >= days {
				refundAmount = ticket.TotalAmount * float64(percent) / 100.0

				// Process refund based on payment provider
				if refundAmount > 0 {
					if ticket.PaymentProvider == "paypal" && s.paypalProvider != nil && ticket.PayPalCaptureID != "" {
						// PayPal refund
						if err := s.paypalProvider.ProcessRefund(&ticket, refundAmount); err != nil {
							return fmt.Errorf("failed to process PayPal refund: %w", err)
						}
					} else if ticket.StripePaymentIntentID != "" {
						// Stripe refund (default)
						if _, err := refund.New(&stripe.RefundParams{
							PaymentIntent: stripe.String(ticket.StripePaymentIntentID),
							Amount:        stripe.Int64(int64(refundAmount * 100)),
						}); err != nil {
							return fmt.Errorf("failed to process Stripe refund: %w", err)
						}
					}
				}
			} else if mode == "refund" {
				// Gewünschter Refund, aber nicht eligible → Fehler zurück
				return errors.New("refund_not_eligible")
			}
		}

		// Ticket immer stornieren; Refund-Felder nur setzen, wenn > 0
		now := time.Now()
		updates := map[string]interface{}{
			"status":       "cancelled",
			"cancelled_at": now,
		}
		if refundAmount > 0 {
			updates["refunded_amount"] = refundAmount
			updates["refunded_at"] = now
		}

		if err := s.db.Model(&ticket).Updates(updates).Error; err != nil {
			return err
		}
		return nil

	default:
		// For other statuses like 'cancelled' or 'refunded', no action is allowed.
		return fmt.Errorf("ticket cannot be cancelled with current status: %s", ticket.Status)
	}
}

// RefundTicket processes a full refund for a ticket (admin action)
func (s *TicketService) RefundTicket(ticketID uuid.UUID, fullRefund bool) error {
	var ticket models.Ticket

	// Get ticket
	if err := s.db.First(&ticket, ticketID).Error; err != nil {
		return errors.New("ticket not found")
	}

	if ticket.Status != "paid" {
		return errors.New("only paid tickets can be refunded")
	}

	// Calculate refund amount
	refundAmount := ticket.GetRefundAmount(fullRefund)

	// Process refund based on payment provider
	if ticket.PaymentProvider == "paypal" && s.paypalProvider != nil {
		// PayPal refund
		if err := s.paypalProvider.ProcessRefund(&ticket, refundAmount); err != nil {
			return fmt.Errorf("failed to process PayPal refund: %w", err)
		}
	} else {
		// Stripe refund (default/fallback)
		if ticket.StripePaymentIntentID != "" {
			_, err := refund.New(&stripe.RefundParams{
				PaymentIntent: stripe.String(ticket.StripePaymentIntentID),
				Amount:        stripe.Int64(int64(refundAmount * 100)), // Convert to cents
			})
			if err != nil {
				return fmt.Errorf("failed to process Stripe refund: %w", err)
			}
		}
	}

	// Update ticket status
	now := time.Now()
	updates := map[string]interface{}{
		"status":          "refunded",
		"refunded_amount": refundAmount,
		"refunded_at":     now,
	}

	if err := s.db.Model(&ticket).Updates(updates).Error; err != nil {
		return err
	}

	return nil
}

// GetUserTickets retrieves all tickets for a user
func (s *TicketService) GetUserTickets(userID uuid.UUID) ([]*models.Ticket, error) {
	var tickets []*models.Ticket

	err := s.db.Preload("Event").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&tickets).Error

	return tickets, err
}

// GetTicketByID retrieves a ticket by ID
func (s *TicketService) GetTicketByID(ticketID uuid.UUID) (*models.Ticket, error) {
	var ticket models.Ticket

	if err := s.db.Preload("Event").Preload("User").First(&ticket, ticketID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("ticket not found")
		}
		return nil, err
	}

	return &ticket, nil
}

// GetEventTickets retrieves all tickets for an event
func (s *TicketService) GetEventTickets(eventID uuid.UUID) ([]*models.Ticket, error) {
	var tickets []*models.Ticket

	err := s.db.Preload("User").Preload("Event").
		Where("event_id = ?", eventID).
		Order("created_at DESC").
		Find(&tickets).Error

	return tickets, err
}

// CancelAllTicketsForEvent cancels all tickets of an event.
// If refundPaid is true, paid tickets are fully refunded and marked refunded; otherwise they are marked cancelled without refund.
func (s *TicketService) CancelAllTicketsForEvent(eventID uuid.UUID, refundPaid bool) error {
	var tickets []*models.Ticket
	if err := s.db.Where("event_id = ? AND status IN ?", eventID, []string{"pending", "paid"}).Find(&tickets).Error; err != nil {
		return err
	}

	now := time.Now()
	for _, t := range tickets {
		switch t.Status {
		case "pending":
			// historisieren statt löschen
			updates := map[string]interface{}{
				"status":       "cancelled",
				"cancelled_at": now,
			}
			if err := s.db.Model(&models.Ticket{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
				return err
			}
		case "paid":
			if refundPaid {
				// full refund - support both Stripe and PayPal
				if t.PaymentProvider == "paypal" && s.paypalProvider != nil && t.PayPalCaptureID != "" {
					// PayPal refund
					if err := s.paypalProvider.ProcessRefund(t, t.TotalAmount); err != nil {
						return fmt.Errorf("failed to refund PayPal ticket %s: %w", t.ID, err)
					}
				} else if t.StripePaymentIntentID != "" {
					// Stripe refund (default)
					_, err := refund.New(&stripe.RefundParams{
						PaymentIntent: stripe.String(t.StripePaymentIntentID),
						Amount:        stripe.Int64(int64(t.TotalAmount * 100)),
					})
					if err != nil {
						return fmt.Errorf("failed to refund Stripe ticket %s: %w", t.ID, err)
					}
				}
				updates := map[string]interface{}{
					"status":          "refunded",
					"refunded_amount": t.TotalAmount,
					"refunded_at":     now,
				}
				if err := s.db.Model(&models.Ticket{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
					return err
				}
			} else {
				// cancel without refund
				updates := map[string]interface{}{
					"status":       "cancelled",
					"cancelled_at": now,
				}
				if err := s.db.Model(&models.Ticket{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// DeleteTicketsForEvent removes all tickets for the event (after refunds)
func (s *TicketService) DeleteTicketsForEvent(eventID uuid.UUID) error {
	return s.db.Where("event_id = ?", eventID).Delete(&models.Ticket{}).Error
}

// RefundEventTickets refunds all tickets for an event (when event is cancelled)
func (s *TicketService) RefundEventTickets(eventID uuid.UUID) error {
	// Get all paid tickets for the event
	var tickets []*models.Ticket
	if err := s.db.Where("event_id = ? AND status = ?", eventID, "paid").Find(&tickets).Error; err != nil {
		return err
	}

	// Process refunds for each ticket
	for _, ticket := range tickets {
		if err := s.RefundTicket(ticket.ID, true); err != nil {
			// Log error but continue with other refunds
			fmt.Printf("Failed to refund ticket %s: %v\n", ticket.ID, err)
		}
	}

	return nil
}

// CreateTicketWithProvider creates a ticket with a specific payment provider (stripe or paypal)
// This is the NEW function that supports both providers in parallel
func (s *TicketService) CreateTicketWithProvider(userID, eventID uuid.UUID, includesPickup bool, pickupAddress, paymentProvider string) (*models.Ticket, string, error) {
	// Validate payment provider
	if paymentProvider != "stripe" && paymentProvider != "paypal" {
		return nil, "", errors.New("invalid payment provider; must be 'stripe' or 'paypal'")
	}

	// Check if PayPal is requested but not enabled
	if paymentProvider == "paypal" && s.paypalProvider == nil {
		return nil, "", errors.New("PayPal is not enabled")
	}

	// Check if user already has a ticket for this event
	var existingTicket models.Ticket
	err := s.db.Where("user_id = ? AND event_id = ? AND status IN ?", userID, eventID, []string{"pending", "paid"}).First(&existingTicket).Error
	if err == nil {
		return nil, "", errors.New("user already has a ticket for this event")
	}

	// Get event details
	var event models.Event
	if err := s.db.First(&event, eventID).Error; err != nil {
		return nil, "", errors.New("event not found")
	}

	// Get user for group
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, "", errors.New("user not found")
	}

	// Enforce allowed_group
	if event.AllowedGroup == "guests" && user.Group != "guests" {
		return nil, "", errors.New("event not available for your group")
	}
	if event.AllowedGroup == "bubble" && user.Group != "bubble" {
		return nil, "", errors.New("event not available for your group")
	}
	if event.AllowedGroup == "plus" && user.Group != "plus" {
		return nil, "", errors.New("event not available for your group")
	}

	// Check availability
	availableSpots := event.GetAvailableSpots(s.db)
	if availableSpots <= 0 {
		return nil, "", errors.New("event is fully booked")
	}

	// Determine base price based on user group and event prices
	basePrice := event.GuestsPrice
	if user.Group == "bubble" {
		basePrice = event.BubblePrice
	}
	if user.Group == "plus" {
		basePrice = event.PlusPrice
	}

	// Get pickup service price
	pickupPrice := 0.0
	if includesPickup {
		var setting models.SystemSetting
		if err := s.db.Where("key = ?", "pickup_service_price").First(&setting).Error; err == nil {
			fmt.Sscanf(setting.Value, "%f", &pickupPrice)
		}
	}

	// Create ticket
	ticket := &models.Ticket{
		UserID:          userID,
		EventID:         eventID,
		Status:          "pending",
		Price:           basePrice,
		IncludesPickup:  includesPickup,
		PickupPrice:     pickupPrice,
		PickupAddress:   pickupAddress,
		PaymentProvider: paymentProvider,
	}
	ticket.CalculateTotalAmount()

	// Save ticket
	if err := s.db.Create(ticket).Error; err != nil {
		return nil, "", err
	}

	// Create checkout session with selected provider
	var checkoutURL string
	var provider PaymentProvider

	if paymentProvider == "paypal" {
		provider = s.paypalProvider
	} else {
		provider = s.stripeProvider
	}

	checkoutURL, err = provider.CreateCheckout(ticket, &event, &user, ticket.TotalAmount)
	if err != nil {
		// Delete ticket if checkout creation fails
		s.db.Delete(ticket)
		return nil, "", fmt.Errorf("failed to create checkout: %w", err)
	}

	return ticket, checkoutURL, nil
}

// UpdateTicketStatus updates the status of a ticket
func (s *TicketService) UpdateTicketStatus(ticketID uuid.UUID, status string) error {
	return s.db.Model(&models.Ticket{}).Where("id = ?", ticketID).Update("status", status).Error
}

// UpdateTicket updates multiple fields of a ticket
func (s *TicketService) UpdateTicket(ticketID uuid.UUID, updates map[string]interface{}) error {
	return s.db.Model(&models.Ticket{}).Where("id = ?", ticketID).Updates(updates).Error
}

// UpdatePayPalCaptureID updates the PayPal capture ID for a ticket
func (s *TicketService) UpdatePayPalCaptureID(ticketID uuid.UUID, captureID string) error {
	return s.db.Model(&models.Ticket{}).Where("id = ?", ticketID).Update("paypal_capture_id", captureID).Error
}
