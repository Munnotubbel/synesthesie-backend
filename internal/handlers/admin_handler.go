package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
)

type AdminHandler struct {
	adminService  *services.AdminService
	eventService  *services.EventService
	inviteService *services.InviteService
	userService   *services.UserService
}

func NewAdminHandler(adminService *services.AdminService, eventService *services.EventService, inviteService *services.InviteService, userService *services.UserService) *AdminHandler {
	return &AdminHandler{
		adminService:  adminService,
		eventService:  eventService,
		inviteService: inviteService,
		userService:   userService,
	}
}

// GetAllEvents retrieves all events for admin
func (h *AdminHandler) GetAllEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	includeInactive := c.Query("include_inactive") == "true"
	offset := (page - 1) * limit

	events, total, err := h.eventService.GetAllEvents(offset, limit, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve events"})
		return
	}

	eventList := make([]gin.H, len(events))
	for i, event := range events {
		availableSpots := event.GetAvailableSpots(h.eventService.GetDB())
		eventList[i] = gin.H{
			"id":               event.ID,
			"name":             event.Name,
			"description":      event.Description,
			"date_from":        event.DateFrom,
			"date_to":          event.DateTo,
			"time_from":        event.TimeFrom,
			"time_to":          event.TimeTo,
			"max_participants": event.MaxParticipants,
			"price":            event.Price,
			"is_active":        event.IsActive,
			"available_spots":  availableSpots,
			"created_at":       event.CreatedAt,
			"updated_at":       event.UpdatedAt,
		}
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

// CreateEvent creates a new event
func (h *AdminHandler) CreateEvent(c *gin.Context) {
	var req struct {
		Name            string    `json:"name" binding:"required"`
		Description     string    `json:"description"`
		DateFrom        time.Time `json:"date_from" binding:"required"`
		DateTo          time.Time `json:"date_to" binding:"required"`
		TimeFrom        string    `json:"time_from" binding:"required"`
		TimeTo          string    `json:"time_to" binding:"required"`
		MaxParticipants int       `json:"max_participants" binding:"required,min=1"`
		Price           float64   `json:"price" binding:"required,min=0"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	event := &models.Event{
		Name:            req.Name,
		Description:     req.Description,
		DateFrom:        req.DateFrom,
		DateTo:          req.DateTo,
		TimeFrom:        req.TimeFrom,
		TimeTo:          req.TimeTo,
		MaxParticipants: req.MaxParticipants,
		Price:           req.Price,
		IsActive:        true,
	}

	if err := h.eventService.CreateEvent(event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Event created successfully",
		"event":   event,
	})
}

// UpdateEvent updates an existing event
func (h *AdminHandler) UpdateEvent(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	var req struct {
		Name            string    `json:"name"`
		Description     string    `json:"description"`
		DateFrom        time.Time `json:"date_from"`
		DateTo          time.Time `json:"date_to"`
		TimeFrom        string    `json:"time_from"`
		TimeTo          string    `json:"time_to"`
		MaxParticipants int       `json:"max_participants"`
		Price           float64   `json:"price"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if !req.DateFrom.IsZero() {
		updates["date_from"] = req.DateFrom
	}
	if !req.DateTo.IsZero() {
		updates["date_to"] = req.DateTo
	}
	if req.TimeFrom != "" {
		updates["time_from"] = req.TimeFrom
	}
	if req.TimeTo != "" {
		updates["time_to"] = req.TimeTo
	}
	if req.MaxParticipants > 0 {
		updates["max_participants"] = req.MaxParticipants
	}
	if req.Price >= 0 {
		updates["price"] = req.Price
	}

	if err := h.eventService.UpdateEvent(eventID, updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Event updated successfully"})
}

// DeleteEvent deletes an event
func (h *AdminHandler) DeleteEvent(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	if err := h.eventService.DeleteEvent(eventID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Event deleted successfully"})
}

// DeactivateEvent deactivates an event
func (h *AdminHandler) DeactivateEvent(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	if err := h.eventService.DeactivateEvent(eventID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Event deactivated successfully"})
}

// RefundEventTickets refunds all tickets for an event
func (h *AdminHandler) RefundEventTickets(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// This would typically be injected
	ticketService := services.NewTicketService(h.eventService.GetDB(), nil)

	if err := ticketService.RefundEventTickets(eventID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refund tickets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All tickets refunded successfully"})
}

// GetAllInvites retrieves all invite codes
func (h *AdminHandler) GetAllInvites(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	includeUsed := c.Query("include_used") == "true"
	offset := (page - 1) * limit

	invites, total, err := h.inviteService.GetAllInvites(offset, limit, includeUsed)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invites"})
		return
	}

	inviteList := make([]gin.H, len(invites))
	for i, invite := range invites {
		inviteData := gin.H{
			"id":            invite.ID,
			"code":          invite.Code,
			"status":        invite.Status,
			"viewed_at":     invite.ViewedAt,
			"registered_at": invite.RegisteredAt,
			"created_at":    invite.CreatedAt,
		}

		if invite.User != nil {
			inviteData["registered_by"] = gin.H{
				"id":       invite.User.ID,
				"username": invite.User.Username,
				"name":     invite.User.Name,
			}
		}

		inviteList[i] = inviteData
	}

	c.JSON(http.StatusOK, gin.H{
		"invites": inviteList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// CreateInvite creates a new invite code
func (h *AdminHandler) CreateInvite(c *gin.Context) {
	var req struct {
		Count int `json:"count"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		req.Count = 1
	}

	if req.Count <= 0 {
		req.Count = 1
	}

	if req.Count == 1 {
		invite, err := h.inviteService.CreateInviteCode()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invite code"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Invite code created successfully",
			"invite": gin.H{
				"id":   invite.ID,
				"code": invite.Code,
			},
		})
	} else {
		invites, err := h.inviteService.CreateBulkInviteCodes(req.Count)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		inviteList := make([]gin.H, len(invites))
		for i, invite := range invites {
			inviteList[i] = gin.H{
				"id":   invite.ID,
				"code": invite.Code,
			}
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Invite codes created successfully",
			"invites": inviteList,
		})
	}
}

// DeactivateInvite deactivates an invite code
func (h *AdminHandler) DeactivateInvite(c *gin.Context) {
	inviteIDStr := c.Param("id")
	inviteID, err := uuid.Parse(inviteIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invite ID"})
		return
	}

	if err := h.inviteService.DeactivateInvite(inviteID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invite deactivated successfully"})
}

// GetAllUsers retrieves all users
func (h *AdminHandler) GetAllUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")
	offset := (page - 1) * limit

	var users []*models.User
	var total int64
	var err error

	if search != "" {
		users, total, err = h.userService.SearchUsers(search, offset, limit)
	} else {
		users, total, err = h.userService.GetAllUsers(offset, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve users"})
		return
	}

	userList := make([]gin.H, len(users))
	for i, user := range users {
		userList[i] = gin.H{
			"id":                   user.ID,
			"username":             user.Username,
			"email":                user.Email,
			"name":                 user.Name,
			"favorite_drink":       user.FavoriteDrink,
			"favorite_cocktail":    user.FavoriteCocktail,
			"favorite_shot":        user.FavoriteShot,
			"is_active":            user.IsActive,
			"registered_with_code": user.RegisteredWithCode,
			"created_at":           user.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"users": userList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetUserDetails retrieves detailed information about a user
func (h *AdminHandler) GetUserDetails(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	user, err := h.userService.GetUserWithDetails(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Build ticket history
	ticketHistory := make([]gin.H, len(user.Tickets))
	for i, ticket := range user.Tickets {
		ticketHistory[i] = gin.H{
			"id":              ticket.ID,
			"event_name":      ticket.Event.Name,
			"event_date":      ticket.Event.DateFrom,
			"status":          ticket.Status,
			"total_amount":    ticket.TotalAmount,
			"includes_pickup": ticket.IncludesPickup,
			"created_at":      ticket.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":                   user.ID,
			"username":             user.Username,
			"email":                user.Email,
			"name":                 user.Name,
			"favorite_drink":       user.FavoriteDrink,
			"favorite_cocktail":    user.FavoriteCocktail,
			"favorite_shot":        user.FavoriteShot,
			"is_active":            user.IsActive,
			"registered_with_code": user.RegisteredWithCode,
			"created_at":           user.CreatedAt,
		},
		"ticket_history": ticketHistory,
	})
}

// ResetUserPassword resets a user's password
func (h *AdminHandler) ResetUserPassword(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get user details for email
	user, err := h.userService.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Reset password
	newPassword, err := h.adminService.ResetUserPassword(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send email with new password (in production, this should be done asynchronously)
	// emailService would be injected in real implementation
	// go emailService.SendPasswordResetEmail(user.Email, user.Name, newPassword)
	_ = user // Will be used when email service is properly injected

	c.JSON(http.StatusOK, gin.H{
		"message":      "Password reset successfully",
		"new_password": newPassword, // In production, this should only be sent via email
	})
}

// GetPickupServicePrice retrieves the current pickup service price
func (h *AdminHandler) GetPickupServicePrice(c *gin.Context) {
	price, err := h.adminService.GetPickupServicePrice()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve pickup service price"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"price": price})
}

// UpdatePickupServicePrice updates the pickup service price
func (h *AdminHandler) UpdatePickupServicePrice(c *gin.Context) {
	var req struct {
		Price float64 `json:"price" binding:"required,min=0"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.adminService.UpdatePickupServicePrice(req.Price); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Pickup service price updated successfully",
		"price":   req.Price,
	})
}
