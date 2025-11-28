package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	paypal "github.com/logpacker/PayPal-Go-SDK"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

// PayPalProvider implements PaymentProvider for PayPal
type PayPalProvider struct {
	client *paypal.Client
	cfg    *config.Config
	db     *gorm.DB
}

// NewPayPalProvider creates a new PayPal payment provider
func NewPayPalProvider(cfg *config.Config, db *gorm.DB) (*PayPalProvider, error) {
	var client *paypal.Client
	var err error

	if cfg.PayPalMode == "live" {
		client, err = paypal.NewClient(cfg.PayPalClientID, cfg.PayPalSecret, paypal.APIBaseLive)
	} else {
		client, err = paypal.NewClient(cfg.PayPalClientID, cfg.PayPalSecret, paypal.APIBaseSandBox)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create PayPal client: %w", err)
	}

	// Get access token
	_, err = client.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get PayPal access token: %w", err)
	}

	return &PayPalProvider{
		client: client,
		cfg:    cfg,
		db:     db,
	}, nil
}

// GetProviderName returns "paypal"
func (p *PayPalProvider) GetProviderName() string {
	return "paypal"
}

// CreateCheckout creates a PayPal order and returns the approval URL
func (p *PayPalProvider) CreateCheckout(ticket *models.Ticket, event *models.Event, user *models.User, totalAmount float64) (string, error) {
	// Build purchase units
	amountStr := fmt.Sprintf("%.2f", totalAmount)
	purchaseUnits := []paypal.PurchaseUnitRequest{
		{
			ReferenceID: ticket.ID.String(),
			Description: fmt.Sprintf("Ticket für %s", event.Name),
			CustomID:    ticket.ID.String(),
			Amount: &paypal.PurchaseUnitAmount{
				Currency: "EUR",
				Value:    amountStr,
			},
		},
	}

	// Application context with ticket_id in URLs
	successURL := fmt.Sprintf("%s?ticket_id=%s", p.cfg.PayPalSuccessURL, ticket.ID.String())
	cancelURL := fmt.Sprintf("%s?ticket_id=%s", p.cfg.PayPalCancelURL, ticket.ID.String())

	appContext := &paypal.ApplicationContext{
		BrandName:          "Synesthesie",
		LandingPage:        "LOGIN", // "LOGIN" or "BILLING" or "NO_PREFERENCE"
		ShippingPreference: "NO_SHIPPING", // "NO_SHIPPING", "SET_PROVIDED_ADDRESS", or "GET_FROM_FILE"
		UserAction:         "PAY_NOW", // "PAY_NOW" or "CONTINUE"
		ReturnURL:          successURL,
		CancelURL:          cancelURL,
	}

	// Debug logging
	fmt.Printf("[PayPal DEBUG] Creating order:\n")
	fmt.Printf("  - Amount: %s EUR\n", amountStr)
	fmt.Printf("  - Ticket ID: %s\n", ticket.ID.String())
	fmt.Printf("  - Event: %s\n", event.Name)
	fmt.Printf("  - Return URL: %s\n", successURL)
	fmt.Printf("  - Cancel URL: %s\n", cancelURL)

	// Create the order using the correct API signature
	// Use empty payer object instead of nil
	payer := &paypal.CreateOrderPayer{}
	createdOrder, err := p.client.CreateOrder(paypal.OrderIntentCapture, purchaseUnits, payer, appContext)
	if err != nil {
		fmt.Printf("[PayPal ERROR] Failed to create order: %v\n", err)
		return "", fmt.Errorf("failed to create PayPal order: %w", err)
	}

	fmt.Printf("[PayPal DEBUG] Order created successfully: %s\n", createdOrder.ID)

	// Save PayPal order ID to ticket
	ticket.PayPalOrderID = createdOrder.ID
	ticket.PaymentProvider = "paypal"
	if err := p.db.Save(ticket).Error; err != nil {
		return "", fmt.Errorf("failed to save ticket: %w", err)
	}

	// Extract approval URL
	var approvalURL string
	for _, link := range createdOrder.Links {
		if link.Rel == "approve" {
			approvalURL = link.Href
			break
		}
	}

	if approvalURL == "" {
		return "", fmt.Errorf("no approval URL found in PayPal order response")
	}

	// Start background polling to check order status (fallback if webhook fails)
	go p.pollOrderStatus(ticket.ID, createdOrder.ID)

	return approvalURL, nil
}

// pollOrderStatus polls PayPal order status in background (fallback if webhook fails)
func (p *PayPalProvider) pollOrderStatus(ticketID uuid.UUID, orderID string) {
	fmt.Printf("[PayPal Polling] Starting status polling for ticket %s, order %s\n", ticketID, orderID)

	// Poll for up to 2 minutes (24 attempts x 5 seconds = 2 minutes)
	maxAttempts := 24
	pollInterval := 5 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		time.Sleep(pollInterval)

		// Check if ticket still exists and its status
		var ticket models.Ticket
		if err := p.db.First(&ticket, ticketID).Error; err != nil {
			// Ticket was deleted (user cancelled)
			fmt.Printf("⚠️ [PayPal Polling] Ticket %s was deleted, but checking if payment went through...\n", ticketID)

			// Continue polling to check if payment was completed
			// If payment completed, we'll handle it below
			ticket.ID = ticketID // Set ID for potential recreation
		} else {
			// Ticket exists - check status
			if ticket.Status == "paid" {
				fmt.Printf("[PayPal Polling] Ticket %s already paid (webhook arrived), stopping polling\n", ticketID)
				return
			}

			if ticket.Status == "cancelled" || ticket.Status == "pending_cancellation" {
				fmt.Printf("⚠️ [PayPal Polling] Ticket %s is %s, but checking if payment went through...\n", ticketID, ticket.Status)
				// Continue polling - if payment completed, we'll reactivate it (GRACE PERIOD)
			}
		}

		// Get order details from PayPal
		order, err := p.client.GetOrder(orderID)
		if err != nil {
			fmt.Printf("[PayPal Polling] Failed to get order details (attempt %d/%d): %v\n", attempt, maxAttempts, err)
			continue
		}

		fmt.Printf("[PayPal Polling] Order %s status: %s (attempt %d/%d)\n", orderID, order.Status, attempt, maxAttempts)

		// Check if order is approved (user completed payment)
		if order.Status == "APPROVED" {
			// Order is approved, now we need to capture it
			captureResp, err := p.client.CaptureOrder(orderID, paypal.CaptureOrderRequest{})
			if err != nil {
				fmt.Printf("[PayPal Polling] Failed to capture order: %v\n", err)
				continue
			}

			// Extract the real capture ID from the response
			// The capture ID is nested: captureResp.PurchaseUnits[0].Payments.Captures[0].ID
			var captureID string
			if len(captureResp.PurchaseUnits) > 0 &&
				captureResp.PurchaseUnits[0].Payments != nil &&
				len(captureResp.PurchaseUnits[0].Payments.Captures) > 0 {
				captureID = captureResp.PurchaseUnits[0].Payments.Captures[0].ID
			}

			// Fallback to order ID if capture ID not found (shouldn't happen)
			if captureID == "" {
				captureID = orderID
				fmt.Printf("[PayPal Polling] Warning: Could not extract capture ID, using order ID as fallback\n")
			}

			// Update or reactivate ticket
			if ticket.Status == "cancelled" || ticket.Status == "pending_cancellation" || ticket.ID == uuid.Nil {
				// Ticket was cancelled/pending_cancellation/deleted but payment went through
				// GRACE PERIOD in action - payment completed within 5 minutes!
				fmt.Printf("⚠️ [PayPal Polling] Payment completed for %s ticket %s\n", ticket.Status, ticketID)
				fmt.Printf("✅ [PayPal Polling] Reactivating ticket (GRACE PERIOD - payment within 5 min)\n")

				// Try to reactivate or update
				updates := map[string]interface{}{
					"status":             "paid",
					"paypal_capture_id":  captureID,
					"cancelled_at":       nil,
				}

				// Use Unscoped to update even soft-deleted records
				result := p.db.Unscoped().Model(&models.Ticket{}).Where("id = ?", ticketID).Updates(updates)
				if result.Error != nil {
					fmt.Printf("⚠️ [PayPal Polling] CRITICAL - Failed to reactivate ticket %s: %v\n", ticketID, result.Error)
					fmt.Printf("⚠️ [PayPal Polling] User paid but ticket lost! Manual intervention required!\n")
					// TODO: Send admin alert email
					return
				}

				if result.RowsAffected == 0 {
					fmt.Printf("⚠️ [PayPal Polling] CRITICAL - Ticket %s not found in DB but payment completed!\n", ticketID)
					fmt.Printf("⚠️ [PayPal Polling] Capture ID: %s - Manual refund or ticket recreation required!\n", captureID)
					// TODO: Send admin alert email
					return
				}

				fmt.Printf("✅ [PayPal Polling] Ticket %s reactivated and marked as paid (was: %s, capture: %s)\n", ticketID, ticket.Status, captureID)
			} else {
				// Normal flow - ticket is pending
				ticket.Status = "paid"
				ticket.PayPalCaptureID = captureID
				if err := p.db.Save(&ticket).Error; err != nil {
					fmt.Printf("[PayPal Polling] Failed to update ticket: %v\n", err)
					continue
				}

				fmt.Printf("[PayPal Polling] ✅ Ticket %s marked as paid (capture ID: %s)\n", ticketID, captureID)
			}
			return
		}

		// Check if order is already completed (captured by webhook)
		if order.Status == "COMPLETED" {
			// Webhook already processed this, just update ticket if needed
			if ticket.Status != "paid" {
				ticket.Status = "paid"
				ticket.PayPalCaptureID = orderID // Use order ID as fallback
				p.db.Save(&ticket)
				fmt.Printf("[PayPal Polling] ✅ Ticket %s marked as paid (order already completed)\n", ticketID)
			}
			return
		}

		// If order is cancelled or expired, stop polling
		if order.Status == "VOIDED" || order.Status == "EXPIRED" || order.Status == "CANCELLED" {
			fmt.Printf("[PayPal Polling] Order %s is %s, stopping polling\n", orderID, order.Status)
			return
		}
	}

	fmt.Printf("[PayPal Polling] ⏱️ Polling timeout for ticket %s after %d attempts\n", ticketID, maxAttempts)
}

// ProcessRefund processes a PayPal refund
func (p *PayPalProvider) ProcessRefund(ticket *models.Ticket, amount float64) error {
	if ticket.PayPalCaptureID == "" {
		return fmt.Errorf("no PayPal capture ID found")
	}

	// Get access token
	accessToken, err := p.client.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get PayPal access token: %w", err)
	}

	// Build refund request
	amountStr := fmt.Sprintf("%.2f", amount)
	refundRequest := map[string]interface{}{
		"amount": map[string]string{
			"currency_code": "EUR",
			"value":         amountStr,
		},
	}

	// Call PayPal Refund Capture API directly
	// https://developer.paypal.com/docs/api/payments/v2/#captures_refund
	apiBase := "https://api.sandbox.paypal.com"
	if p.cfg.PayPalMode == "live" {
		apiBase = "https://api.paypal.com"
	}

	url := fmt.Sprintf("%s/v2/payments/captures/%s/refund", apiBase, ticket.PayPalCaptureID)

	reqBody, _ := json.Marshal(refundRequest)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create refund request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken.Token))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send refund request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PayPal refund failed (status %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("[PayPal Refund] Successfully refunded %.2f EUR for ticket %s (capture: %s)\n", amount, ticket.ID, ticket.PayPalCaptureID)
	return nil
}

// CheckAndCaptureOrder checks if a PayPal order is approved/completed and captures it if needed
// Used by active polling to find completed payments even if webhooks fail
func (p *PayPalProvider) CheckAndCaptureOrder(ticket *models.Ticket) bool {
	if ticket.PayPalOrderID == "" {
		return false
	}

	// Get order details from PayPal
	order, err := p.client.GetOrder(ticket.PayPalOrderID)
	if err != nil {
		log.Printf("⚠️ Payment check: Failed to get PayPal order %s: %v", ticket.PayPalOrderID, err)
		return false
	}

	log.Printf("🔍 Payment check: PayPal order %s status: %s", ticket.PayPalOrderID, order.Status)

	// Check if order is approved and needs capturing
	if order.Status == "APPROVED" {
		log.Printf("✅ Payment check: Found approved PayPal order %s, capturing...", ticket.PayPalOrderID)

		// Capture the order
		captureResp, err := p.client.CaptureOrder(ticket.PayPalOrderID, paypal.CaptureOrderRequest{})
		if err != nil {
			log.Printf("⚠️ Payment check: Failed to capture PayPal order %s: %v", ticket.PayPalOrderID, err)
			return false
		}

		// Extract capture ID
		var captureID string
		if len(captureResp.PurchaseUnits) > 0 &&
			captureResp.PurchaseUnits[0].Payments != nil &&
			len(captureResp.PurchaseUnits[0].Payments.Captures) > 0 {
			captureID = captureResp.PurchaseUnits[0].Payments.Captures[0].ID
		} else {
			captureID = ticket.PayPalOrderID // Fallback
		}

		// Update ticket to paid
		updates := map[string]interface{}{
			"status":             "paid",
			"cancelled_at":       nil,
			"paypal_capture_id":  captureID,
		}

		if err := p.db.Model(&models.Ticket{}).Where("id = ?", ticket.ID).Updates(updates).Error; err != nil {
			log.Printf("⚠️ Payment check: Failed to update PayPal ticket %s: %v", ticket.ID, err)
			return false
		}

		log.Printf("✅ Payment check: PayPal ticket %s confirmed as paid (capture: %s)", ticket.ID, captureID)
		return true
	}

	// Check if already completed (captured by webhook or previous poll)
	if order.Status == "COMPLETED" {
		log.Printf("✅ Payment check: PayPal order %s already completed", ticket.PayPalOrderID)

		// For completed orders, we need to get the capture ID
		// The SDK doesn't expose it directly, so we use the order ID as fallback
		captureID := ticket.PayPalOrderID

		// If ticket already has capture ID, use it
		if ticket.PayPalCaptureID != "" {
			captureID = ticket.PayPalCaptureID
		}

		// Update ticket to paid if not already
		if ticket.Status != "paid" {
			updates := map[string]interface{}{
				"status":             "paid",
				"cancelled_at":       nil,
				"paypal_capture_id":  captureID,
			}

			if err := p.db.Model(&models.Ticket{}).Where("id = ?", ticket.ID).Updates(updates).Error; err != nil {
				log.Printf("⚠️ Payment check: Failed to update completed PayPal ticket %s: %v", ticket.ID, err)
				return false
			}

			log.Printf("✅ Payment check: Completed PayPal ticket %s confirmed as paid", ticket.ID)
			return true
		}
	}

	return false
}
