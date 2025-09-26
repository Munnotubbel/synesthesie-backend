package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/services"
	"github.com/synesthesie/backend/pkg/validation"
)

type AuthHandler struct {
	authService   *services.AuthService
	userService   *services.UserService
	inviteService *services.InviteService
	emailService  *services.EmailService
}

func NewAuthHandler(authService *services.AuthService, userService *services.UserService, inviteService *services.InviteService, emailService *services.EmailService) *AuthHandler {
	return &AuthHandler{
		authService:   authService,
		userService:   userService,
		inviteService: inviteService,
		emailService:  emailService,
	}
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		InviteCode string `json:"invite_code" binding:"required"`
		Username   string `json:"username" binding:"required,min=2,max=30"`
		Email      string `json:"email" binding:"required,email"`
		Password   string `json:"password" binding:"required,min=8"`
		Name       string `json:"name" binding:"required"`
		Mobile     string `json:"mobile"`
		Drink1     string `json:"drink1"`
		Drink2     string `json:"drink2"`
		Drink3     string `json:"drink3"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate username
	if !validation.ValidateUsername(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid username format"})
		return
	}

	// Validate email
	if !validation.ValidateEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email format"})
		return
	}

	// Validate password
	if !validation.ValidatePassword(req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must contain at least one uppercase letter, one lowercase letter, one number, and one special character"})
		return
	}

	// Validate mobile only if SMS verification is enabled and mobile provided
	if h.authService != nil && h.authService.GetConfig() != nil && h.authService.GetConfig().SMSVerificationEnabled {
		if req.Mobile == "" || !validation.ValidateE164Mobile(req.Mobile) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Mobile number required and must be E.164 (+491234567890) when SMS verification is enabled"})
			return
		}
	} else {
		// If SMS verification is disabled, ignore any provided mobile
		req.Mobile = ""
	}

	// Validate invite code for registration
	_, err := h.inviteService.ValidateInviteCodeForRegistration(req.InviteCode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Register user
	user, err := h.authService.Register(
		req.Username,
		req.Email,
		req.Password,
		req.Name,
		req.Mobile,
		req.Drink1,
		req.Drink2,
		req.Drink3,
		req.InviteCode,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send registration email (optional)
	go h.emailService.SendRegistrationConfirmation(user.Email, user.Name, user.Username, user.Email)

	message := "Registration successful"
	if h.authService != nil && h.authService.GetConfig() != nil && h.authService.GetConfig().SMSVerificationEnabled {
		message = "Registration successful. Please verify your mobile number."
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": message,
		"user": gin.H{
			"id":              user.ID,
			"username":        user.Username,
			"email":           user.Email,
			"name":            user.Name,
			"group":           user.Group,
			"mobile_verified": user.MobileVerified,
		},
	})
}

// VerifyMobile handles mobile verification by code
func (h *AuthHandler) VerifyMobile(c *gin.Context) {
	if h.authService == nil || h.authService.GetConfig() == nil || !h.authService.GetConfig().SMSVerificationEnabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint disabled"})
		return
	}
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.VerifyMobile(userID.(uuid.UUID), req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Mobile verified"})
}

// ResendMobileVerification issues a new verification code
func (h *AuthHandler) ResendMobileVerification(c *gin.Context) {
	if h.authService == nil || h.authService.GetConfig() == nil || !h.authService.GetConfig().SMSVerificationEnabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint disabled"})
		return
	}
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if err := h.authService.ResendMobileVerification(userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Verification code sent"})
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessToken, refreshToken, user, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":              user.ID,
			"username":        user.Username,
			"email":           user.Email,
			"name":            user.Name,
			"is_admin":        user.IsAdmin,
			"group":           user.Group,
			"mobile_verified": user.MobileVerified,
		},
	})
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := h.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
	})
}

// Logout handles user logout
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := h.authService.Logout(userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to logout"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logout successful"})
}

// ForgotPassword requests a password reset link via email
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = h.authService.RequestPasswordReset(req.Email)
	c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link has been sent."})
}

// ResetPassword sets a new password using a valid token
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validation.ValidatePassword(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be strong (min 12, upper/lower/number/special)"})
		return
	}
	if err := h.authService.PerformPasswordReset(req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password reset successful"})
}
