package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/services"
)

type PayPalHandler struct {
	ticketService *services.TicketService
	emailService  *services.EmailService
	cfg           *config.Config
}

func NewPayPalHandler(ticketService *services.TicketService, emailService *services.EmailService, cfg *config.Config) *PayPalHandler {
	return &PayPalHandler{
		ticketService: ticketService,
		emailService:  emailService,
		cfg:           cfg,
	}
}

// PayPalWebhookEvent represents a PayPal webhook event
type PayPalWebhookEvent struct {
	ID         string `json:"id"`
	EventType  string `json:"event_type"`
	CreateTime string `json:"create_time"`
	Resource   struct {
		ID          string `json:"id"`
		OrderID     string `json:"order_id"`
		Status      string `json:"status"`
		Amount      struct {
			Currency string `json:"currency_code"`
			Value    string `json:"value"`
		} `json:"amount"`
		CustomID string `json:"custom_id"` // This is our ticket_id
	} `json:"resource"`
}

// HandleWebhook processes PayPal webhook events
func (h *PayPalHandler) HandleWebhook(c *gin.Context) {
	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("PayPal webhook: failed to read body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Parse the webhook event
	var event PayPalWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("PayPal webhook: failed to parse JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	log.Printf("PayPal webhook received: event_type=%s, id=%s", event.EventType, event.ID)

	// Handle different event types
	switch event.EventType {
	case "PAYMENT.CAPTURE.COMPLETED":
		h.handlePaymentCaptureCompleted(event)
	case "CHECKOUT.ORDER.APPROVED":
		// Order approved, but not yet captured - we can ignore this
		log.Printf("PayPal order approved: %s", event.Resource.ID)
	case "PAYMENT.CAPTURE.DENIED":
		log.Printf("PayPal payment denied: %s", event.Resource.ID)
	case "PAYMENT.CAPTURE.REFUNDED":
		log.Printf("PayPal payment refunded: %s", event.Resource.ID)
	default:
		log.Printf("PayPal webhook: unhandled event type: %s", event.EventType)
	}

	// Always return 200 OK to acknowledge receipt
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Webhook received",
	})
}

// handlePaymentCaptureCompleted handles successful PayPal payment capture
func (h *PayPalHandler) handlePaymentCaptureCompleted(event PayPalWebhookEvent) {
	// Extract ticket ID from custom_id or order_id
	ticketIDStr := event.Resource.CustomID
	if ticketIDStr == "" {
		log.Printf("PayPal webhook: no custom_id found in event")
		return
	}

	ticketID, err := uuid.Parse(ticketIDStr)
	if err != nil {
		log.Printf("PayPal webhook: invalid ticket ID: %s", ticketIDStr)
		return
	}

	// Get the ticket
	ticket, err := h.ticketService.GetTicketByID(ticketID)
	if err != nil {
		// Ticket was deleted (user cancelled) but payment went through
		// This is a critical issue - user paid but has no ticket!
		log.Printf("⚠️ PayPal webhook: CRITICAL - Payment received for deleted ticket %s (capture: %s)", ticketID, event.Resource.ID)
		log.Printf("⚠️ PayPal webhook: User paid but ticket was cancelled. Manual refund required!")

		// TODO: Send alert email to admin
		if h.emailService != nil {
			subject := "🚨 KRITISCH: PayPal-Zahlung ohne Ticket"
			body := fmt.Sprintf(`
KRITISCHE SITUATION:

Ein User hat bei PayPal bezahlt, aber das Ticket wurde bereits gelöscht!

Ticket-ID: %s
Capture-ID: %s
Betrag: %s %s
Zeitpunkt: %s

AKTION ERFORDERLICH:
1. Prüfe PayPal-Transaktion
2. Erstelle Ticket manuell ODER
3. Erstatte Zahlung über PayPal Dashboard

Dies passiert wenn User während der Zahlung das Ticket abbricht.
			`, ticketID, event.Resource.ID, event.Resource.Amount.Value, event.Resource.Amount.Currency, time.Now().Format("02.01.2006 15:04:05"))

			_ = h.emailService.SendGenericTextEmail(h.cfg.AdminAlertEmail, subject, body)
		}
		return
	}

	// Check if already processed
	if ticket.Status == "paid" {
		log.Printf("PayPal webhook: ticket already paid: %s", ticketID)
		return
	}

	// Check if ticket was cancelled or is pending cancellation
	if ticket.Status == "cancelled" || ticket.Status == "pending_cancellation" {
		log.Printf("⚠️ PayPal webhook: Payment received for %s ticket %s (capture: %s)", ticket.Status, ticketID, event.Resource.ID)
		log.Printf("✅ PayPal webhook: Reactivating ticket and marking as paid (payment completed within grace period)")

		// Reactivate the ticket (user paid, so they should get the ticket)
		// This is the GRACE PERIOD in action - payment completed before final cancellation!
		updates := map[string]interface{}{
			"status":             "paid",
			"cancelled_at":       nil,
			"paypal_capture_id":  event.Resource.ID,
		}

		if err := h.ticketService.UpdateTicket(ticketID, updates); err != nil {
			log.Printf("PayPal webhook: failed to reactivate ticket: %v", err)
			return
		}

		log.Printf("✅ PayPal webhook: Ticket %s reactivated and marked as paid (was: %s)", ticketID, ticket.Status)
		return
	}

	// Update ticket status (normal flow)
	ticket.Status = "paid"
	ticket.PayPalCaptureID = event.Resource.ID // Save capture ID for refunds

	if err := h.ticketService.UpdateTicketStatus(ticketID, "paid"); err != nil {
		log.Printf("PayPal webhook: failed to update ticket status: %v", err)
		return
	}

	// Update PayPal capture ID separately
	if err := h.ticketService.UpdatePayPalCaptureID(ticketID, event.Resource.ID); err != nil {
		log.Printf("PayPal webhook: failed to update capture ID: %v", err)
	}

	log.Printf("PayPal webhook: ticket %s marked as paid (capture: %s)", ticketID, event.Resource.ID)

	// Send confirmation email
	if h.emailService != nil {
		// TODO: Send ticket confirmation email
		log.Printf("PayPal webhook: TODO - send confirmation email for ticket %s", ticketID)
	}
}

