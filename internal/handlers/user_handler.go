package handlers

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
)

type UserHandler struct {
	userService    *services.UserService
	eventService   *services.EventService
	ticketService  *services.TicketService
	AssetService   *services.AssetService
	StorageService *services.StorageService
	S3Service      *services.S3Service
}

func NewUserHandler(userService *services.UserService, eventService *services.EventService, ticketService *services.TicketService) *UserHandler {
	return &UserHandler{
		userService:   userService,
		eventService:  eventService,
		ticketService: ticketService,
	}
}

// GetProfile retrieves the current user's profile
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	user, err := h.userService.GetUserByID(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"name":       user.Name,
		"drink1":     user.Drink1,
		"drink2":     user.Drink2,
		"drink3":     user.Drink3,
		"group":      user.Group,
		"created_at": user.CreatedAt,
	})
}

// UpdateProfile updates the current user's profile (only drink1-3)
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req struct {
		Drink1 string `json:"drink1"`
		Drink2 string `json:"drink2"`
		Drink3 string `json:"drink3"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Drink1 != "" {
		updates["drink1"] = req.Drink1
	}
	if req.Drink2 != "" {
		updates["drink2"] = req.Drink2
	}
	if req.Drink3 != "" {
		updates["drink3"] = req.Drink3
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid fields to update"})
		return
	}

	if err := h.userService.UpdateUserProfile(userID.(uuid.UUID), updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}

// GetUserEvents retrieves upcoming events with user's ticket status
func (h *UserHandler) GetUserEvents(c *gin.Context) {
	userID, _ := c.Get("userID")

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	// Get upcoming events
	events, total, err := h.eventService.GetUpcomingEvents(offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve events"})
		return
	}

	// Get user's tickets
	userTickets, err := h.ticketService.GetUserTickets(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tickets"})
		return
	}

	// Get user to determine group
	user, err := h.userService.GetUserByID(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user"})
		return
	}
	userPrice := 200.0
	if user.Group == "bubble" {
		userPrice = 35.0
	}

	// Create a map of event IDs to tickets
	ticketMap := make(map[uuid.UUID]*models.Ticket)
	for _, ticket := range userTickets {
		if ticket.Status == "paid" || ticket.Status == "pending" {
			ticketMap[ticket.EventID] = ticket
		}
	}

	// Build response
	eventList := make([]gin.H, len(events))
	for i, event := range events {
		availableSpots := event.GetAvailableSpots(h.eventService.GetDB())

		eventData := gin.H{
			"id":               event.ID,
			"name":             event.Name,
			"description":      event.Description,
			"date_from":        event.DateFrom,
			"date_to":          event.DateTo,
			"time_from":        event.TimeFrom,
			"time_to":          event.TimeTo,
			"price":            userPrice,
			"max_participants": event.MaxParticipants,
			"available_spots":  availableSpots,
			"has_ticket":       false,
		}

		// Check if user has ticket for this event
		if ticket, exists := ticketMap[event.ID]; exists {
			eventData["has_ticket"] = true
			eventData["ticket"] = gin.H{
				"id":              ticket.ID,
				"status":          ticket.Status,
				"includes_pickup": ticket.IncludesPickup,
			}
		}

		eventList[i] = eventData
	}

	c.JSON(http.StatusOK, gin.H{
		"events": eventList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetUserTickets retrieves all tickets for the current user
func (h *UserHandler) GetUserTickets(c *gin.Context) {
	userID, _ := c.Get("userID")

	tickets, err := h.ticketService.GetUserTickets(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tickets"})
		return
	}

	ticketList := make([]gin.H, len(tickets))
	for i, ticket := range tickets {
		ticketList[i] = gin.H{
			"id":              ticket.ID,
			"status":          ticket.Status,
			"price":           ticket.Price,
			"includes_pickup": ticket.IncludesPickup,
			"pickup_price":    ticket.PickupPrice,
			"pickup_address":  ticket.PickupAddress,
			"total_amount":    ticket.TotalAmount,
			"refunded_amount": ticket.RefundedAmount,
			"created_at":      ticket.CreatedAt,
			"cancelled_at":    ticket.CancelledAt,
			"refunded_at":     ticket.RefundedAt,
			"event": gin.H{
				"id":        ticket.Event.ID,
				"name":      ticket.Event.Name,
				"date_from": ticket.Event.DateFrom,
				"date_to":   ticket.Event.DateTo,
				"time_from": ticket.Event.TimeFrom,
				"time_to":   ticket.Event.TimeTo,
			},
		}
	}

	c.JSON(http.StatusOK, gin.H{"tickets": ticketList})
}

// BookTicket creates a new ticket booking
func (h *UserHandler) BookTicket(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req struct {
		EventID        string `json:"event_id" binding:"required"`
		IncludesPickup bool   `json:"includes_pickup"`
		PickupAddress  string `json:"pickup_address"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate pickup address if pickup is included
	if req.IncludesPickup && req.PickupAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pickup address is required when pickup service is selected"})
		return
	}

	// Parse event ID
	eventID, err := uuid.Parse(req.EventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Create ticket
	ticket, checkoutSession, err := h.ticketService.CreateTicket(
		userID.(uuid.UUID),
		eventID,
		req.IncludesPickup,
		req.PickupAddress,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ticket_id":    ticket.ID,
		"checkout_url": checkoutSession.URL,
	})
}

// CancelTicket cancels a user's ticket
func (h *UserHandler) CancelTicket(c *gin.Context) {
	userID, _ := c.Get("userID")
	ticketIDStr := c.Param("id")

	ticketID, err := uuid.Parse(ticketIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ticket ID"})
		return
	}

	if err := h.ticketService.CancelTicket(ticketID, userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Ticket cancelled successfully"})
}

// DownloadAsset streams an asset if the user is authenticated and allowed
func (h *UserHandler) DownloadAsset(c *gin.Context) {
	assetIDStr := c.Param("id")
	assetID, err := uuid.Parse(assetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid asset id"})
		return
	}

	asset, err := h.AssetService.GetByID(assetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}

	// Audio: 302 presign + optional Cache
	if strings.HasPrefix(asset.Key, "audio/") {
		cfg := h.S3Service.GetConfig()
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "download service not available"})
			return
		}
		// Cache-Hit
		if cfg.MediaCacheAudio {
			cachePath := filepath.Join(cfg.AudioCachePath, filepath.FromSlash(strings.TrimPrefix(asset.Key, "audio/")))
			if _, statErr := os.Stat(cachePath); statErr == nil {
				name := asset.Filename
				if name == "" {
					name = filepath.Base(asset.Key)
				}
				_ = h.StorageService.ServeFileWithRange(c.Writer, c.Request, cachePath, name)
				return
			}
		}
		url, err := h.S3Service.PresignMediaGet(c, cfg.MediaAudioBucket, asset.Key, 15*60*1e9)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize download"})
			return
		}
		if cfg.MediaCacheAudio {
			go func() {
				cachePath := filepath.Join(cfg.AudioCachePath, filepath.FromSlash(strings.TrimPrefix(asset.Key, "audio/")))
				_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
				_ = h.S3Service.DownloadMediaToFile(c, cfg.MediaAudioBucket, asset.Key, cachePath)
			}()
		}
		c.Redirect(http.StatusFound, url)
		return
	}

	// Images: ensure local exists (fallback from S3) and stream
	abs := h.AssetService.GetAbsolutePath(asset)
	if strings.HasPrefix(asset.Key, "images/") && h.S3Service != nil {
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			cfg := h.S3Service.GetConfig()
			if cfg != nil {
				buf, derr := h.S3Service.DownloadMedia(c, cfg.MediaImagesBucket, asset.Key)
				if derr == nil {
					_, _, _, _ = h.StorageService.SaveStream(c, asset.Key, bytes.NewReader(buf.Bytes()))
				}
			}
		}
	}
	name := asset.Filename
	if name == "" {
		name = filepath.Base(asset.Key)
	}
	if err := h.StorageService.ServeFileWithRange(c.Writer, c.Request, abs, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stream file"})
		return
	}
}
