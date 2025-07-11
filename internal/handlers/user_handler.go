package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
)

type UserHandler struct {
	userService   *services.UserService
	eventService  *services.EventService
	ticketService *services.TicketService
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
		"id":                user.ID,
		"username":          user.Username,
		"email":             user.Email,
		"name":              user.Name,
		"favorite_drink":    user.FavoriteDrink,
		"favorite_cocktail": user.FavoriteCocktail,
		"favorite_shot":     user.FavoriteShot,
		"created_at":        user.CreatedAt,
	})
}

// UpdateProfile updates the current user's profile
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req struct {
		Name             string `json:"name"`
		FavoriteDrink    string `json:"favorite_drink"`
		FavoriteCocktail string `json:"favorite_cocktail"`
		FavoriteShot     string `json:"favorite_shot"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.FavoriteDrink != "" {
		updates["favorite_drink"] = req.FavoriteDrink
	}
	if req.FavoriteCocktail != "" {
		updates["favorite_cocktail"] = req.FavoriteCocktail
	}
	if req.FavoriteShot != "" {
		updates["favorite_shot"] = req.FavoriteShot
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
			"price":            event.Price,
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
