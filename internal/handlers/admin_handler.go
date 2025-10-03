package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
)

type AdminHandler struct {
	adminService   *services.AdminService
	eventService   *services.EventService
	inviteService  *services.InviteService
	userService    *services.UserService
	ticketService  *services.TicketService
	storageService *services.StorageService
	s3Service      *services.S3Service
	qrService      *services.QRService
	backupService  *services.BackupService
}

func NewAdminHandler(adminService *services.AdminService, eventService *services.EventService, inviteService *services.InviteService, userService *services.UserService, ticketService *services.TicketService, storageService *services.StorageService, s3Service *services.S3Service, qrService *services.QRService, backupService *services.BackupService) *AdminHandler {
	return &AdminHandler{
		adminService:   adminService,
		eventService:   eventService,
		ticketService:  ticketService,
		inviteService:  inviteService,
		userService:    userService,
		storageService: storageService,
		s3Service:      s3Service,
		qrService:      qrService,
		backupService:  backupService,
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

	// Compute turnover for listed events
	ids := make([]uuid.UUID, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	turnoverMap, _ := h.eventService.GetTurnoverByEventIDs(ids)

	eventList := make([]gin.H, len(events))
	for i, event := range events {
		availableSpots := event.GetAvailableSpots(h.eventService.GetDB())
		turnover := turnoverMap[event.ID]
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
			"guests_price":     event.GuestsPrice,
			"bubble_price":     event.BubblePrice,
			"plus_price":       event.PlusPrice,
			"allowed_group":    event.AllowedGroup,
			"is_active":        event.IsActive,
			"available_spots":  availableSpots,
			"turnover":         turnover,
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
		AllowedGroup    string    `json:"allowed_group"` // all|guests|bubble|plus, default all
		GuestsPrice     float64   `json:"guests_price"`  // default 200
		BubblePrice     float64   `json:"bubble_price"`  // default 35
		PlusPrice       float64   `json:"plus_price"`    // default 50
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
		AllowedGroup:    req.AllowedGroup,
		GuestsPrice:     req.GuestsPrice,
		BubblePrice:     req.BubblePrice,
		PlusPrice:       req.PlusPrice,
	}

	if err := h.eventService.CreateEvent(event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Optional: E-Mail-Ankündigung an berechtigte Gruppen
	go func() {
		// We will load users by allowed_group and send a short announcement using event_reminder template
		// This uses a lightweight direct DB call through services for simplicity
		// Fetch recipients
		var users []*models.User
		group := strings.ToLower(event.AllowedGroup)
		q := h.eventService.GetDB().Model(&models.User{}).Where("is_active = ?", true)
		if group == "guests" || group == "bubble" || group == "plus" {
			q = q.Where("\"group\" = ?", group)
		}
		if err := q.Find(&users).Error; err != nil {
			return
		}
		// Build URL
		eventsURL := strings.TrimRight(h.adminService.GetConfig().FrontendURL, "/") + "/events"
		email := services.NewEmailService(h.adminService.GetConfig())
		for _, u := range users {
			data := map[string]interface{}{
				"EventName": event.Name,
				"EventsURL": eventsURL,
			}
			_ = email.SendEventAnnouncement(u.Email, data)
		}
	}()

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
		AllowedGroup    string    `json:"allowed_group"`
		GuestsPrice     *float64  `json:"guests_price"`
		BubblePrice     *float64  `json:"bubble_price"`
		PlusPrice       *float64  `json:"plus_price"`
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
	if req.AllowedGroup != "" {
		updates["allowed_group"] = req.AllowedGroup
	}
	if req.GuestsPrice != nil {
		updates["guests_price"] = *req.GuestsPrice
	}
	if req.BubblePrice != nil {
		updates["bubble_price"] = *req.BubblePrice
	}
	if req.PlusPrice != nil {
		updates["plus_price"] = *req.PlusPrice
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

	// Anforderung „Event löschen“: Statt Hard-Delete setzen wir Event auf inactive,
	// erstatten alle Tickets voll und informieren die Nutzer per E-Mail.
	ts := services.NewTicketService(h.eventService.GetDB(), h.adminService.GetConfig())
	if err := ts.CancelAllTicketsForEvent(eventID, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refund/cancel tickets"})
		return
	}
	if err := h.eventService.DeactivateEvent(eventID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// E-Mails an Ticketinhaber
	// Lade betroffene Tickets (auch bereits pending/paid vor Statuswechsel) und sende Info-Mail
	tsList, _ := services.NewTicketService(h.eventService.GetDB(), h.adminService.GetConfig()).GetEventTickets(eventID)
	email := services.NewEmailService(h.adminService.GetConfig())
	loc, _ := time.LoadLocation("Europe/Berlin")
	cancelledAt := time.Now().In(loc).Format("02.01.2006 15:04")
	for _, t := range tsList {
		data := map[string]interface{}{
			"UserName":       t.User.Name,
			"EventName":      t.Event.Name,
			"EventDate":      t.Event.DateFrom.In(loc).Format("02.01.2006"),
			"EventTime":      t.Event.TimeFrom,
			"TicketID":       t.ID,
			"RefundAmount":   fmt.Sprintf("%.2f", t.TotalAmount),
			"FullRefund":     true,
			"PartialRefund":  false,
			"CancellationBy": "abgesagt durch die Veranstalter:innen",
			"CancellationAt": cancelledAt,
		}
		_ = email.SendEventCancelled(t.User.Email, data)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Event cancelled and deactivated successfully"})
}

// DeactivateEvent deactivates an event
func (h *AdminHandler) DeactivateEvent(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Optional: set all tickets of event to cancelled (no refund) when deactivating
	ts := services.NewTicketService(h.eventService.GetDB(), h.adminService.GetConfig())
	_ = ts.CancelAllTicketsForEvent(eventID, false)

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

	// Use configured TicketService to ensure Stripe key is set
	ticketService := services.NewTicketService(h.eventService.GetDB(), h.adminService.GetConfig())

	if err := ticketService.RefundEventTickets(eventID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refund tickets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tickets refunded successfully"})
}

// GetEventDetails retrieves detailed information about an event including participant list
func (h *AdminHandler) GetEventDetails(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Get event
	event, err := h.eventService.GetEventByID(eventID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	// Get all tickets for the event (paid only)
	tickets, err := h.ticketService.GetEventTickets(eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tickets"})
		return
	}

	// Group participants by user group and sort alphabetically
	type Participant struct {
		Name   string `json:"name"`
		Email  string `json:"email"`
		Drink1 string `json:"drink1"`
		Drink2 string `json:"drink2"`
		Drink3 string `json:"drink3"`
		Group  string `json:"group"`
	}

	groupedParticipants := make(map[string][]Participant)
	groupedParticipants["guests"] = []Participant{}
	groupedParticipants["bubble"] = []Participant{}
	groupedParticipants["plus"] = []Participant{}

	for _, ticket := range tickets {
		if ticket.Status != "paid" {
			continue
		}
		if ticket.User.ID == uuid.Nil {
			continue
		}

		p := Participant{
			Name:   ticket.User.Name,
			Email:  ticket.User.Email,
			Drink1: ticket.User.Drink1,
			Drink2: ticket.User.Drink2,
			Drink3: ticket.User.Drink3,
			Group:  ticket.User.Group,
		}

		group := ticket.User.Group
		if group != "guests" && group != "bubble" && group != "plus" {
			group = "guests" // fallback
		}
		groupedParticipants[group] = append(groupedParticipants[group], p)
	}

	// Sort each group alphabetically by name
	for group := range groupedParticipants {
		participants := groupedParticipants[group]
		// Simple bubble sort by name
		for i := 0; i < len(participants); i++ {
			for j := i + 1; j < len(participants); j++ {
				if strings.ToLower(participants[i].Name) > strings.ToLower(participants[j].Name) {
					participants[i], participants[j] = participants[j], participants[i]
				}
			}
		}
		groupedParticipants[group] = participants
	}

	// Calculate total participants count
	totalParticipants := len(groupedParticipants["guests"]) + len(groupedParticipants["bubble"]) + len(groupedParticipants["plus"])

	// Calculate available spots
	availableSpots := event.GetAvailableSpots(h.eventService.GetDB())

	// Get turnover
	turnoverMap, _ := h.eventService.GetTurnoverByEventIDs([]uuid.UUID{event.ID})
	turnover := turnoverMap[event.ID]

	c.JSON(http.StatusOK, gin.H{
		"event": gin.H{
			"id":                 event.ID,
			"name":               event.Name,
			"description":        event.Description,
			"date_from":          event.DateFrom,
			"date_to":            event.DateTo,
			"time_from":          event.TimeFrom,
			"time_to":            event.TimeTo,
			"max_participants":   event.MaxParticipants,
			"guests_price":       event.GuestsPrice,
			"bubble_price":       event.BubblePrice,
			"plus_price":         event.PlusPrice,
			"allowed_group":      event.AllowedGroup,
			"is_active":          event.IsActive,
			"available_spots":    availableSpots,
			"total_participants": totalParticipants,
			"turnover":           turnover,
			"created_at":         event.CreatedAt,
			"updated_at":         event.UpdatedAt,
		},
		"participants": groupedParticipants,
	})
}

// ExportEventParticipantsCSV exports event participants as CSV grouped by user group
func (h *AdminHandler) ExportEventParticipantsCSV(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Get event
	event, err := h.eventService.GetEventByID(eventID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	// Get all tickets for the event
	tickets, err := h.ticketService.GetEventTickets(eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tickets"})
		return
	}

	// Group participants by user group
	type Participant struct {
		Name   string
		Email  string
		Drink1 string
		Drink2 string
		Drink3 string
		Group  string
	}

	var participants []Participant
	for _, ticket := range tickets {
		// Only include paid tickets
		if ticket.Status != "paid" {
			continue
		}

		// Check if user data is loaded
		if ticket.UserID == uuid.Nil || ticket.User.ID == uuid.Nil {
			continue
		}

		p := Participant{
			Name:   ticket.User.Name,
			Email:  ticket.User.Email,
			Drink1: ticket.User.Drink1,
			Drink2: ticket.User.Drink2,
			Drink3: ticket.User.Drink3,
			Group:  ticket.User.Group,
		}
		participants = append(participants, p)
	}

	// Sort by group, then by name (even if empty, we still create the CSV)
	for i := 0; i < len(participants); i++ {
		for j := i + 1; j < len(participants); j++ {
			// First sort by group
			if participants[i].Group > participants[j].Group {
				participants[i], participants[j] = participants[j], participants[i]
			} else if participants[i].Group == participants[j].Group {
				// Then sort alphabetically by name within same group
				if strings.ToLower(participants[i].Name) > strings.ToLower(participants[j].Name) {
					participants[i], participants[j] = participants[j], participants[i]
				}
			}
		}
	}

	// Create CSV buffer
	buf := &bytes.Buffer{}
	// UTF-8 BOM for Excel compatibility
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(buf)

	// Write separator hint for Excel
	_ = w.Write([]string{"sep=,"})

	// Write header
	_ = w.Write([]string{"Gruppe", "Name", "Email", "Lieblingsgetraenk 1", "Lieblingsgetraenk 2", "Lieblingsgetraenk 3"})

	// Write participants
	for _, p := range participants {
		groupName := p.Group
		if groupName == "" {
			groupName = "guests"
		}
		_ = w.Write([]string{
			groupName,
			p.Name,
			p.Email,
			p.Drink1,
			p.Drink2,
			p.Drink3,
		})
	}

	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate csv"})
		return
	}

	// Create filename with event name and date
	eventDate := event.DateFrom.Format("02-01-2006")
	safe := strings.ReplaceAll(event.Name, " ", "_")
	safe = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, safe)
	filename := fmt.Sprintf("Teilnehmer_%s_%s.csv", eventDate, safe)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportEventDrinksXLSX exports favorite drinks statistics for event participants as an Excel file
func (h *AdminHandler) ExportEventDrinksXLSX(c *gin.Context) {
	eventIDStr := c.Param("id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Load tickets with users for the event (only paid)
	ts := services.NewTicketService(h.eventService.GetDB(), h.adminService.GetConfig())
	tickets, err := ts.GetEventTickets(eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load event tickets"})
		return
	}

	// Aggregate drinks across participants with paid tickets
	// Map: drink name -> count and list of users
	type DrinkInfo struct {
		Count int
		Users []string
	}
	drinkMap := make(map[string]*DrinkInfo)

	for _, t := range tickets {
		if t.Status != "paid" {
			continue
		}
		userName := t.User.Name
		if userName == "" {
			userName = t.User.Username
		}

		if t.User.Drink1 != "" {
			drink := strings.TrimSpace(t.User.Drink1)
			if drinkMap[drink] == nil {
				drinkMap[drink] = &DrinkInfo{Count: 0, Users: []string{}}
			}
			drinkMap[drink].Count++
			drinkMap[drink].Users = append(drinkMap[drink].Users, userName)
		}
		if t.User.Drink2 != "" {
			drink := strings.TrimSpace(t.User.Drink2)
			if drinkMap[drink] == nil {
				drinkMap[drink] = &DrinkInfo{Count: 0, Users: []string{}}
			}
			drinkMap[drink].Count++
			drinkMap[drink].Users = append(drinkMap[drink].Users, userName)
		}
		if t.User.Drink3 != "" {
			drink := strings.TrimSpace(t.User.Drink3)
			if drinkMap[drink] == nil {
				drinkMap[drink] = &DrinkInfo{Count: 0, Users: []string{}}
			}
			drinkMap[drink].Count++
			drinkMap[drink].Users = append(drinkMap[drink].Users, userName)
		}
	}

	// If no paid participants, return a clear message (like pickup CSV behavior)
	hasPaid := false
	for _, t := range tickets {
		if t.Status == "paid" {
			hasPaid = true
			break
		}
	}
	if !hasPaid {
		c.JSON(http.StatusOK, gin.H{"status": "no_participants"})
		return
	}

	// Prepare CSV (Excel-compatible)
	buf := &bytes.Buffer{}
	// UTF-8 BOM for Excel compatibility
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(buf)

	// Write separator hint for Excel
	_ = w.Write([]string{"sep=,"})

	// Header with 3 columns
	_ = w.Write([]string{"Getränk", "Anzahl", "Gewählt von"})

	// Sort drinks alphabetically
	keys := make([]string, 0, len(drinkMap))
	for k := range drinkMap {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	// Write rows with users
	for _, drink := range keys {
		info := drinkMap[drink]
		userList := strings.Join(info.Users, ", ")
		_ = w.Write([]string{drink, fmt.Sprintf("%d", info.Count), userList})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to render CSV"})
		return
	}

	// Build filename: Getränke_DD-MM-YYYY_EVENTNAME.csv (Europe/Berlin)
	ev, _ := h.eventService.GetEventByID(eventID)
	dateStr := ""
	if ev != nil {
		loc, _ := time.LoadLocation("Europe/Berlin")
		dateStr = ev.DateFrom.In(loc).Format("02-01-2006")
	}
	name := "event"
	if ev != nil && strings.TrimSpace(ev.Name) != "" {
		name = strings.TrimSpace(ev.Name)
	}
	safe := strings.Map(func(r rune) rune {
		if r == ' ' {
			return '_'
		}
		if r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, name)
	filename := fmt.Sprintf("Getränke_%s_%s.csv", dateStr, safe)

	// Response headers
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// GetAllInvites retrieves all invite codes
func (h *AdminHandler) GetAllInvites(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	includeUsed := c.Query("include_used") == "true"
	groupFilter := strings.TrimSpace(c.Query("group"))   // optional: bubble|guests
	statusFilter := strings.TrimSpace(c.Query("status")) // optional: new|viewed|registered|inactive
	offset := (page - 1) * limit

	invites, total, err := h.inviteService.GetAllInvites(offset, limit, includeUsed, groupFilter, statusFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invites"})
		return
	}

	inviteList := make([]gin.H, len(invites))
	for i, invite := range invites {
		inviteData := gin.H{
			"id":            invite.ID,
			"public_id":     invite.PublicID,
			"code":          invite.Code,
			"status":        invite.Status,
			"group":         invite.Group,
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

// GetInviteStats retrieves statistics about invite codes
func (h *AdminHandler) GetInviteStats(c *gin.Context) {
	stats, err := h.inviteService.GetInviteStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invite statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// CreateInvite creates a new invite code
func (h *AdminHandler) CreateInvite(c *gin.Context) {
	var req struct {
		Count int    `json:"count"`
		Group string `json:"group"` // optional: "bubble"|"guests"|"plus"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		req.Count = 1
	}
	if req.Count <= 0 {
		req.Count = 1
	}

	if req.Group != "" && req.Group != "bubble" && req.Group != "guests" && req.Group != "plus" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group; must be 'bubble', 'guests' or 'plus'"})
		return
	}

	if req.Count == 1 {
		var invite *models.InviteCode
		var err error
		if req.Group == "" {
			invite, err = h.inviteService.CreateInviteCode()
		} else {
			invite, err = h.inviteService.CreateInviteCodeWithGroup(req.Group)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invite code"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Invite code created successfully",
			"invite": gin.H{
				"id":    invite.ID,
				"code":  invite.Code,
				"group": invite.Group,
			},
		})
		return
	}

	var invites []*models.InviteCode
	var err error
	if req.Group == "" {
		invites, err = h.inviteService.CreateBulkInviteCodes(req.Count)
	} else {
		invites, err = h.inviteService.CreateBulkInviteCodesWithGroup(req.Count, req.Group)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inviteList := make([]gin.H, len(invites))
	for i, invite := range invites {
		inviteList[i] = gin.H{
			"id":    invite.ID,
			"code":  invite.Code,
			"group": invite.Group,
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Invite codes created successfully",
		"invites": inviteList,
	})
}

// ReassignUserGroup allows admin to change a user's group
func (h *AdminHandler) ReassignUserGroup(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	var req struct {
		Group string `json:"group" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Group != "bubble" && req.Group != "guests" && req.Group != "plus" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group; must be 'bubble', 'guests' or 'plus'"})
		return
	}

	if err := h.userService.UpdateUserGroup(userID, req.Group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User group updated successfully", "group": req.Group})
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
			"group":                user.Group,
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
			"mobile":               user.Mobile,
			"drink1":               user.Drink1,
			"drink2":               user.Drink2,
			"drink3":               user.Drink3,
			"group":                user.Group,
			"is_active":            user.IsActive,
			"registered_with_code": user.RegisteredWithCode,
			"created_at":           user.CreatedAt,
		},
		"ticket_history": ticketHistory,
	})
}

// ResetUserPassword resets a user's password
func (h *AdminHandler) ResetUserPassword(c *gin.Context) {
	if h.adminService == nil || h.adminService.GetConfig() == nil || !h.adminService.GetConfig().AdminPasswordResetEnabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint disabled"})
		return
	}
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

// UploadAsset allows admin to upload images or flac via multipart/form-data
// kind=images|audio
func (h *AdminHandler) UploadAsset(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	kind := c.DefaultPostForm("kind", "images")
	if kind != "images" && kind != "audio" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid kind"})
		return
	}
	lower := strings.ToLower(file.Filename)
	if kind == "images" {
		if !(strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".webp")) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported image type"})
			return
		}
		if file.Size > 25*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image too large"})
			return
		}
	} else {
		if !strings.HasSuffix(lower, ".flac") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "only .flac allowed"})
			return
		}
		if file.Size > 4*1024*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "audio too large"})
			return
		}
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open upload"})
		return
	}
	defer src.Close()

	key := h.storageService.BuildObjectKey(kind, filepath.Base(file.Filename))

	// Determine content-type
	ctype := "application/octet-stream"
	if ext := strings.ToLower(filepath.Ext(file.Filename)); ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			ctype = m
		}
	}
	if kind == "audio" {
		// Upload audio directly to S3 (media audio bucket), no local copy
		if err := h.s3Service.UploadMedia(c, h.adminService.GetConfig().MediaAudioBucket, key, src, ctype); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload audio to storage"})
			return
		}
		asset, err := h.adminService.CreateAssetRecord(key, file.Filename, file.Size, "", false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist asset"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": asset.ID, "key": key})
		return
	}

	// Images: save locally then upload copy to S3 images bucket
	absPath, size, checksum, err := h.storageService.SaveStream(c, key, src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store file"})
		return
	}
	// Re-open saved file for upload
	f, err := os.Open(absPath)
	if err == nil {
		defer f.Close()
		_ = h.s3Service.UploadMedia(c, h.adminService.GetConfig().MediaImagesBucket, key, f, ctype)
	}

	asset, err := h.adminService.CreateAssetRecord(key, file.Filename, size, checksum, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist asset"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       asset.ID,
		"key":      key,
		"path":     absPath,
		"size":     size,
		"checksum": checksum,
	})
}

// SyncImagesMissing pulls missing images from S3 to local cache
func (h *AdminHandler) SyncImagesMissing(c *gin.Context) {
	prefix := "images/"
	keys, err := h.s3Service.ListMediaKeys(c, h.adminService.GetConfig().MediaImagesBucket, prefix, 1000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list images"})
		return
	}
	fetched := 0
	for _, k := range keys {
		abs := filepath.Join(h.adminService.GetConfig().LocalAssetsPath, filepath.FromSlash(k))
		if _, err := os.Stat(abs); err == nil {
			continue
		}
		buf, err := h.s3Service.DownloadMedia(c, h.adminService.GetConfig().MediaImagesBucket, k)
		if err != nil {
			continue
		}
		if _, _, _, err := h.storageService.SaveStream(c, k, bytes.NewReader(buf.Bytes())); err == nil {
			fetched++
		}
	}
	c.JSON(http.StatusOK, gin.H{"synced": fetched})
}

// GetInviteQR generates (if not yet) and returns a PDF with the invite QR code
func (h *AdminHandler) GetInviteQR(c *gin.Context) {
	idStr := c.Param("id")
	inviteID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invite id"})
		return
	}
	invite, err := h.inviteService.GetInviteByID(inviteID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
		return
	}
	pdfBytes, err := h.qrService.GenerateInviteQRPDF(invite)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate QR"})
		return
	}
	if !invite.QRGenerated {
		_ = h.inviteService.SetInviteQRGenerated(inviteID)
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename=invite_"+inviteID.String()+".pdf")
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// ExportInvitesCSV exports not-yet-exported invites as CSV, with group-specific structure
func (h *AdminHandler) ExportInvitesCSV(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	groupFilter := strings.TrimSpace(c.Query("group")) // optional: "bubble"|"guests"|"plus"
	if groupFilter != "" && groupFilter != "bubble" && groupFilter != "guests" && groupFilter != "plus" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group; must be 'bubble', 'guests' or 'plus'"})
		return
	}

	invites, err := h.inviteService.ListUnexportedInvites(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query invites"})
		return
	}
	if len(invites) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no_invites_to_export"})
		return
	}

	// Filter by group if requested
	filtered := invites
	if groupFilter != "" {
		tmp := make([]*models.InviteCode, 0, len(invites))
		for _, inv := range invites {
			if inv.Group == groupFilter {
				tmp = append(tmp, inv)
			}
		}
		filtered = tmp
		if len(filtered) == 0 {
			c.JSON(http.StatusOK, gin.H{"status": "no_invites_to_export"})
			return
		}
	}

	// Build CSV
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)

	base := h.adminService.GetConfig().FrontendURL
	if !strings.HasSuffix(base, "/") {
		base = base + "/"
	}
	base = base + "register?invite="

	// Partition by group
	bubble := make([]*models.InviteCode, 0)
	guests := make([]*models.InviteCode, 0)
	for _, inv := range filtered {
		if inv.Group == "bubble" {
			bubble = append(bubble, inv)
		} else {
			guests = append(guests, inv)
		}
	}

	ids := make([]uuid.UUID, 0, len(filtered))

	// Write bubble section
	if len(bubble) > 0 {
		_ = writer.Write([]string{"Public-ID", "QR-Link"})
		for _, inv := range bubble {
			qr := base + inv.Code
			pub := ""
			if inv.PublicID != nil {
				pub = *inv.PublicID
			}
			_ = writer.Write([]string{pub, qr})
			ids = append(ids, inv.ID)
		}
	}

	// If both present, add empty line separator
	if len(bubble) > 0 && len(guests) > 0 {
		_ = writer.Write([]string{})
	}

	// Write guests section
	if len(guests) > 0 {
		_ = writer.Write([]string{"QR-Link"})
		for _, inv := range guests {
			qr := base + inv.Code
			_ = writer.Write([]string{qr})
			ids = append(ids, inv.ID)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate csv"})
		return
	}

	// Mark exported
	if err := h.inviteService.MarkInvitesExported(ids); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark invites exported"})
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=invites_export.csv")
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

// ExportInvitesBubbleCSV exports bubble invites as CSV with Public-ID and full register link
func (h *AdminHandler) ExportInvitesBubbleCSV(c *gin.Context) {
	// Fetch unexported (or all) invites for bubble group
	invites, err := h.inviteService.CreateBulkInviteCodesWithGroup(0, "bubble")
	_ = invites
	// Reuse existing listing to avoid creating; list all unexported and filter
	list, err := h.inviteService.ListUnexportedInvites(0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query invites"})
		return
	}
	bubble := make([]*models.InviteCode, 0)
	for _, inv := range list {
		if inv.Group == "bubble" {
			bubble = append(bubble, inv)
		}
	}
	if len(bubble) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no_invites_to_export"})
		return
	}
	// Build CSV
	base := strings.TrimRight(h.adminService.GetConfig().FrontendURL, "/") + "/register?invite="
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"Public-ID", "QR-Link"})
	ids := make([]uuid.UUID, 0, len(bubble))
	for _, inv := range bubble {
		pub := ""
		if inv.PublicID != nil {
			pub = *inv.PublicID
		}
		_ = w.Write([]string{pub, base + inv.Code})
		ids = append(ids, inv.ID)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate csv"})
		return
	}
	// Mark exported
	_ = h.inviteService.MarkInvitesExported(ids)
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=invites_bubble.csv")
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

// ExportInvitesGuestsCSV exports guests invites as CSV with full register link only
func (h *AdminHandler) ExportInvitesGuestsCSV(c *gin.Context) {
	list, err := h.inviteService.ListUnexportedInvites(0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query invites"})
		return
	}
	guests := make([]*models.InviteCode, 0)
	for _, inv := range list {
		if inv.Group == "guests" {
			guests = append(guests, inv)
		}
	}
	if len(guests) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no_invites_to_export"})
		return
	}
	base := strings.TrimRight(h.adminService.GetConfig().FrontendURL, "/") + "/register?invite="
	buf := &bytes.Buffer{}
	// BOM + sep=, sorgt dafür, dass Excel/LibreOffice einheitlich Komma als Trenner nutzt
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = buf.WriteString("sep=,\n")
	w := csv.NewWriter(buf)
	// EXAKT wie bubble: zwei Spalten, Public-ID + QR-Link (Public-ID leer bei guests)
	_ = w.Write([]string{"Public-ID", "QR-Link"})
	ids := make([]uuid.UUID, 0, len(guests))
	for _, inv := range guests {
		pub := ""
		url := base + inv.Code
		_ = w.Write([]string{pub, url})
		ids = append(ids, inv.ID)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate csv"})
		return
	}
	_ = h.inviteService.MarkInvitesExported(ids)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=invites_guests.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportInvitesPlusCSV exports plus invites as CSV with Public-ID and full register link
func (h *AdminHandler) ExportInvitesPlusCSV(c *gin.Context) {
	list, err := h.inviteService.ListUnexportedInvites(0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query invites"})
		return
	}
	plus := make([]*models.InviteCode, 0)
	for _, inv := range list {
		if inv.Group == "plus" {
			plus = append(plus, inv)
		}
	}
	if len(plus) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no_invites_to_export"})
		return
	}
	base := strings.TrimRight(h.adminService.GetConfig().FrontendURL, "/") + "/register?invite="
	buf := &bytes.Buffer{}
	// BOM + sep=, for Excel compatibility
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = buf.WriteString("sep=,\n")
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"Public-ID", "QR-Link"})
	ids := make([]uuid.UUID, 0, len(plus))
	for _, inv := range plus {
		pub := ""
		if inv.PublicID != nil {
			pub = *inv.PublicID
		}
		url := base + inv.Code
		_ = w.Write([]string{pub, url})
		ids = append(ids, inv.ID)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate csv"})
		return
	}
	_ = h.inviteService.MarkInvitesExported(ids)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=invites_plus.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// UpdateUserActive allows admin to set is_active for a user
func (h *AdminHandler) UpdateUserActive(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	var req struct {
		IsActive *bool `json:"is_active"`
		Active   *bool `json:"active"` // alias for convenience
	}
	parseOK := false
	if err := c.ShouldBindJSON(&req); err == nil {
		if req.IsActive != nil {
			parseOK = true
			if err := h.userService.UpdateUserActive(userID, *req.IsActive); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else if req.Active != nil {
			parseOK = true
			if err := h.userService.UpdateUserActive(userID, *req.Active); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	}
	if !parseOK {
		// Try form or query parameter fallback
		val := c.PostForm("is_active")
		if val == "" {
			val = c.PostForm("active")
		}
		if val == "" {
			val = c.Query("is_active")
		}
		if val == "" {
			val = c.Query("active")
		}
		if val == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "is_active boolean required"})
			return
		}
		b, perr := strconv.ParseBool(strings.TrimSpace(val))
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid is_active value"})
			return
		}
		if err := h.userService.UpdateUserActive(userID, b); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "User active status updated", "is_active": true})
}

// ExportPickupCSV exports pickup bookings (paid by default) as CSV: name, mobile, pickup_address
func (h *AdminHandler) ExportPickupCSV(c *gin.Context) {
	eventIDStr := strings.TrimSpace(c.Query("event_id"))
	status := strings.TrimSpace(c.DefaultQuery("status", "paid")) // paid|all
	var eventID *uuid.UUID
	if eventIDStr != "" {
		if id, err := uuid.Parse(eventIDStr); err == nil {
			eventID = &id
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_id"})
			return
		}
	}

	// Use ticket service to list tickets with pickup
	ts := services.NewTicketService(h.eventService.GetDB(), h.adminService.GetConfig())
	tickets, err := ts.ListPickupTickets(eventID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load pickup tickets"})
		return
	}
	if len(tickets) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no_pickups"})
		return
	}

	buf := &bytes.Buffer{}
	// Prepend UTF-8 BOM for better Excel compatibility (äöüß etc.)
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"Name", "Mobile", "Pickup-Address"})
	for _, t := range tickets {
		name := t.User.Name
		mobile := t.User.Mobile
		addr := t.PickupAddress
		_ = w.Write([]string{name, mobile, addr})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate csv"})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=pickups.csv")
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

// GetAllBackups retrieves all backups with pagination
func (h *AdminHandler) GetAllBackups(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	backups, total, err := h.backupService.ListBackups(offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve backups"})
		return
	}

	backupList := make([]gin.H, len(backups))
	for i, backup := range backups {
		backupList[i] = gin.H{
			"id":            backup.ID,
			"filename":      backup.Filename,
			"s3_key":        backup.S3Key,
			"size_bytes":    backup.SizeBytes,
			"status":        backup.Status,
			"type":          backup.Type,
			"started_at":    backup.StartedAt,
			"completed_at":  backup.CompletedAt,
			"error_message": backup.ErrorMessage,
			"created_at":    backup.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"backups": backupList,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetBackupStats retrieves statistics about backups
func (h *AdminHandler) GetBackupStats(c *gin.Context) {
	stats, err := h.backupService.GetBackupStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve backup stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// SyncBackupsFromS3 synchronizes backup records from S3
func (h *AdminHandler) SyncBackupsFromS3(c *gin.Context) {
	synced, err := h.backupService.SyncBackupsFromS3()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to sync backups: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Backups synchronized successfully",
		"synced":  synced,
	})
}

// DeleteBackup deletes a backup record and optionally the S3 object
func (h *AdminHandler) DeleteBackup(c *gin.Context) {
	backupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid backup ID"})
		return
	}

	deleteFromS3 := c.Query("delete_from_s3") == "true"

	if err := h.backupService.DeleteBackup(backupID, deleteFromS3); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete backup: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Backup deleted successfully",
	})
}
