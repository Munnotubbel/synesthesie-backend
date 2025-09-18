package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
	jwtpkg "github.com/synesthesie/backend/pkg/jwt"
)

type PublicHandler struct {
	eventService  *services.EventService
	inviteService *services.InviteService
	cfg           *config.Config
}

func NewPublicHandler(eventService *services.EventService, inviteService *services.InviteService, cfg *config.Config) *PublicHandler {
	return &PublicHandler{
		eventService:  eventService,
		inviteService: inviteService,
		cfg:           cfg,
	}
}

// GetEventICS generates an .ics calendar entry for an event
func (h *PublicHandler) GetEventICS(c *gin.Context) {
	// Accept signed calendar token, short-lived
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token required"})
		return
	}

	claims, err := jwtpkg.ValidateToken(token, h.cfg.JWTSecret)
	if err != nil || claims.TokenType != jwtpkg.CalendarToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	if claims.EventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	eventID, err := uuid.Parse(claims.EventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
		return
	}

	event, err := h.eventService.GetEventByID(eventID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}

	loc, _ := time.LoadLocation("Europe/Berlin")
	dtStart := event.DateFrom.In(loc).Format("20060102T150405")
	dtEnd := event.DateTo.In(loc).Format("20060102T150405")
	uid := fmt.Sprintf("synesthesie-%s@synesthesie.de", event.ID)

	ics := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Synesthesie//EN\r\n" +
		"CALSCALE:GREGORIAN\r\n" +
		"METHOD:PUBLISH\r\n" +
		"BEGIN:VEVENT\r\n" +
		fmt.Sprintf("UID:%s\r\n", uid) +
		fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(event.Name)) +
		fmt.Sprintf("DTSTART;TZID=Europe/Berlin:%s\r\n", dtStart) +
		fmt.Sprintf("DTEND;TZID=Europe/Berlin:%s\r\n", dtEnd) +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=event.ics")
	c.String(http.StatusOK, ics)
}

func escapeICS(s string) string {
	repl := map[string]string{",": "\\,", ";": "\\;", "\n": "\\n"}
	out := s
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
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
		"group":   invite.Group,
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
		"group":  invite.Group,
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
