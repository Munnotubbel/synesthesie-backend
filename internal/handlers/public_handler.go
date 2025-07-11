package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
)

type PublicHandler struct {
	eventService  *services.EventService
	inviteService *services.InviteService
}

func NewPublicHandler(eventService *services.EventService, inviteService *services.InviteService) *PublicHandler {
	return &PublicHandler{
		eventService:  eventService,
		inviteService: inviteService,
	}
}

// GetUpcomingEvents retrieves upcoming public events
func (h *PublicHandler) GetUpcomingEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	events, total, err := h.eventService.GetUpcomingEvents(offset, limit)
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
			"price":            event.Price,
			"max_participants": event.MaxParticipants,
			"available_spots":  availableSpots,
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

// ViewInviteCode handles the first-time viewing of an invite code
func (h *PublicHandler) ViewInviteCode(c *gin.Context) {
	code := c.Param("code")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invite code is required"})
		return
	}

	invite, err := h.inviteService.ViewInviteCode(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"code":    invite.Code,
		"status":  "viewed",
		"message": "Invite code has been marked as viewed. You can now proceed with registration.",
	})
}

// CheckInviteCode checks if an invite code is valid
func (h *PublicHandler) CheckInviteCode(c *gin.Context) {
	code := c.Param("code")

	invite, err := h.inviteService.GetInviteByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"valid": false,
			"error": "Invite code not found",
		})
		return
	}

	response := gin.H{
		"valid":  true,
		"code":   invite.Code,
		"status": invite.Status,
	}

	// Add appropriate message based on status
	switch invite.Status {
	case models.InviteStatusNew:
		response["message"] = "Invite code is ready to be used"
	case models.InviteStatusViewed:
		response["message"] = "Invite code has been viewed and is ready for registration"
	case models.InviteStatusRegistered:
		response["message"] = "Invite code has already been used for registration"
	case models.InviteStatusInactive:
		response["valid"] = false
		response["message"] = "Invite code has been deactivated"
	}

	c.JSON(http.StatusOK, response)
}
