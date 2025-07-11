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
		InviteCode       string `json:"invite_code" binding:"required"`
		Username         string `json:"username" binding:"required,min=3,max=30"`
		Email            string `json:"email" binding:"required,email"`
		Password         string `json:"password" binding:"required,min=8"`
		Name             string `json:"name" binding:"required"`
		FavoriteDrink    string `json:"favorite_drink"`
		FavoriteCocktail string `json:"favorite_cocktail"`
		FavoriteShot     string `json:"favorite_shot"`
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
		req.FavoriteDrink,
		req.FavoriteCocktail,
		req.FavoriteShot,
		req.InviteCode,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send registration confirmation email
	go h.emailService.SendRegistrationConfirmation(user.Email, user.Name, user.Username, user.Email)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Registration successful",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"name":     user.Name,
		},
	})
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
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"name":     user.Name,
			"is_admin": user.IsAdmin,
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
