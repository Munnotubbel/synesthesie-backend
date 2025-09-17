package handlers

import (
	"bytes"
	"encoding/csv"
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
	storageService *services.StorageService
	s3Service      *services.S3Service
	qrService      *services.QRService
}

func NewAdminHandler(adminService *services.AdminService, eventService *services.EventService, inviteService *services.InviteService, userService *services.UserService, storageService *services.StorageService, s3Service *services.S3Service, qrService *services.QRService) *AdminHandler {
	return &AdminHandler{
		adminService:   adminService,
		eventService:   eventService,
		inviteService:  inviteService,
		userService:    userService,
		storageService: storageService,
		s3Service:      s3Service,
		qrService:      qrService,
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
		AllowedGroup    string    `json:"allowed_group"` // all|guests|bubble, default all
		GuestsPrice     float64   `json:"guests_price"`  // default 200
		BubblePrice     float64   `json:"bubble_price"`  // default 35
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
		AllowedGroup    string    `json:"allowed_group"`
		GuestsPrice     *float64  `json:"guests_price"`
		BubblePrice     *float64  `json:"bubble_price"`
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

	c.JSON(http.StatusOK, gin.H{"message": "Tickets refunded successfully"})
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

// CreateInvite creates a new invite code
func (h *AdminHandler) CreateInvite(c *gin.Context) {
	var req struct {
		Count int    `json:"count"`
		Group string `json:"group"` // optional: "bubble"|"guests"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		req.Count = 1
	}
	if req.Count <= 0 {
		req.Count = 1
	}

	if req.Group != "" && req.Group != "bubble" && req.Group != "guests" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group; must be 'bubble' or 'guests'"})
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
	if req.Group != "bubble" && req.Group != "guests" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group; must be 'bubble' or 'guests'"})
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
	groupFilter := strings.TrimSpace(c.Query("group")) // optional: "bubble"|"guests"
	if groupFilter != "" && groupFilter != "bubble" && groupFilter != "guests" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group; must be 'bubble' or 'guests'"})
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

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=pickups.csv")
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}
