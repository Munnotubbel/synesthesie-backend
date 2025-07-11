package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/services"
)

type StripeHandler struct {
	ticketService *services.TicketService
	cfg           *config.Config
}

func NewStripeHandler(ticketService *services.TicketService, cfg *config.Config) *StripeHandler {
	return &StripeHandler{
		ticketService: ticketService,
		cfg:           cfg,
	}
}

// HandleWebhook handles Stripe webhook events
func (h *StripeHandler) HandleWebhook(c *gin.Context) {
	const MaxBodyBytes = int64(65536)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("ERROR: Failed to read Stripe webhook request body: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Error reading request body"})
		return
	}

	// Get Stripe signature header
	signatureHeader := c.GetHeader("Stripe-Signature")

	// Verify webhook signature
	event, err := webhook.ConstructEvent(payload, signatureHeader, h.cfg.StripeWebhookSecret)
	if err != nil {
		log.Printf("ERROR: Webhook signature verification failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid signature"})
		return
	}

	log.Printf("INFO: Received Stripe event type: %s, ID: %s", event.Type, event.ID)

	// Handle the event
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Printf("ERROR: Failed to parse webhook JSON for checkout.session.completed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Error parsing webhook JSON"})
			return
		}

		// Get ticket ID from metadata
		ticketIDStr, ok := session.Metadata["ticket_id"]
		if !ok {
			log.Printf("ERROR: ticket_id not found in metadata for session %s", session.ID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Ticket ID not found in metadata"})
			return
		}

		ticketID, err := uuid.Parse(ticketIDStr)
		if err != nil {
			log.Printf("ERROR: Invalid ticket_id format in metadata: %s", ticketIDStr)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ticket ID"})
			return
		}

		paymentIntentID := ""
		if session.PaymentIntent != nil {
			paymentIntentID = session.PaymentIntent.ID
		}

		log.Printf("INFO: Processing payment confirmation for TicketID: %s, PaymentIntentID: %s", ticketID, paymentIntentID)

		// Confirm payment
		if err := h.ticketService.ConfirmPayment(ticketID, paymentIntentID); err != nil {
			log.Printf("ERROR: Failed to confirm payment for ticket %s: %v", ticketID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to confirm payment"})
			return
		}

		log.Printf("SUCCESS: Payment confirmed for TicketID: %s", ticketID)
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Payment confirmed"})
		return

	case "payment_intent.succeeded":
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			log.Printf("ERROR: Failed to parse webhook JSON for payment_intent.succeeded: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Error parsing webhook JSON"})
			return
		}
		log.Printf("INFO: Received payment_intent.succeeded for %s", paymentIntent.ID)
		// Usually handled by checkout.session.completed, but good for logging.

	case "payment_intent.payment_failed":
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			log.Printf("ERROR: Failed to parse webhook JSON for payment_intent.payment_failed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Error parsing webhook JSON"})
			return
		}
		var reason string
		if paymentIntent.LastPaymentError != nil {
			reason = paymentIntent.LastPaymentError.Msg
		}
		log.Printf("WARN: Payment failed for PaymentIntent %s. Reason: %s", paymentIntent.ID, reason)
		// Optionally: Update ticket status to 'failed'

	default:
		log.Printf("INFO: Unhandled Stripe event type: %s", event.Type)
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Unhandled event type"})
	}
}
