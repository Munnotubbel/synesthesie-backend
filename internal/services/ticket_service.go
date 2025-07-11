package services

import (
	"errors"
	"fmt"
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
	db  *gorm.DB
	cfg *config.Config
}

func NewTicketService(db *gorm.DB, cfg *config.Config) *TicketService {
	stripe.Key = cfg.StripeSecretKey
	return &TicketService{db: db, cfg: cfg}
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

	// Check availability
	availableSpots := event.GetAvailableSpots(s.db)
	if availableSpots <= 0 {
		return nil, nil, errors.New("event is fully booked")
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
		Price:          event.Price,
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
				UnitAmount: stripe.Int64(int64(event.Price * 100)), // Convert to cents
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
				UnitAmount: stripe.Int64(int64(ticket.PickupPrice * 100)), // Convert to cents
			},
			Quantity: stripe.Int64(1),
		})
	}

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems:          lineItems,
		SuccessURL:         stripe.String(fmt.Sprintf("%s?ticket_id=%s", s.cfg.StripeSuccessURL, ticket.ID)),
		CancelURL:          stripe.String(s.cfg.StripeCancelURL),
		ClientReferenceID:  stripe.String(ticket.ID.String()),
		Metadata: map[string]string{
			"ticket_id": ticket.ID.String(),
			"user_id":   ticket.UserID.String(),
			"event_id":  ticket.EventID.String(),
		},
	}

	return session.New(params)
}

// ConfirmPayment confirms a ticket payment after successful Stripe webhook
func (s *TicketService) ConfirmPayment(ticketID uuid.UUID, paymentIntentID string) error {
	result := s.db.Model(&models.Ticket{}).
		Where("id = ? AND status = ?", ticketID, "pending").
		Updates(map[string]interface{}{
			"status":                   "paid",
			"stripe_payment_intent_id": paymentIntentID,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("ticket not found or already paid")
	}

	return nil
}

// CancelTicket cancels a paid ticket and processes a refund, or deletes a pending ticket.
func (s *TicketService) CancelTicket(ticketID, userID uuid.UUID) error {
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
		// We can just delete the ticket record.
		if err := s.db.Delete(&ticket).Error; err != nil {
			return fmt.Errorf("failed to delete pending ticket: %w", err)
		}
		return nil

	case "paid":
		// Existing logic for paid tickets: Check cancellation window and process refund.
		if !ticket.CanBeCancelled() {
			return errors.New("ticket can only be cancelled up to 7 days before the event")
		}

		// Calculate refund amount (50%)
		refundAmount := ticket.GetRefundAmount(false)

		// Process Stripe refund
		if ticket.StripePaymentIntentID != "" {
			_, err := refund.New(&stripe.RefundParams{
				PaymentIntent: stripe.String(ticket.StripePaymentIntentID),
				Amount:        stripe.Int64(int64(refundAmount * 100)), // Convert to cents
			})
			if err != nil {
				return fmt.Errorf("failed to process refund: %w", err)
			}
		}

		// Update ticket status to 'cancelled'
		now := time.Now()
		updates := map[string]interface{}{
			"status":          "cancelled",
			"refunded_amount": refundAmount,
			"refunded_at":     now,
			"cancelled_at":    now,
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

	// Process Stripe refund
	if ticket.StripePaymentIntentID != "" {
		_, err := refund.New(&stripe.RefundParams{
			PaymentIntent: stripe.String(ticket.StripePaymentIntentID),
			Amount:        stripe.Int64(int64(refundAmount * 100)), // Convert to cents
		})
		if err != nil {
			return fmt.Errorf("failed to process refund: %w", err)
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

	err := s.db.Preload("User").
		Where("event_id = ?", eventID).
		Order("created_at DESC").
		Find(&tickets).Error

	return tickets, err
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
