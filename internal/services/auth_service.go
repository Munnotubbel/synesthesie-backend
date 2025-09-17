package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/pkg/crypto"
	jwtpkg "github.com/synesthesie/backend/pkg/jwt"
	"gorm.io/gorm"
)

type AuthService struct {
	db    *gorm.DB
	redis *redis.Client
	cfg   *config.Config
}

func NewAuthService(db *gorm.DB, redis *redis.Client, cfg *config.Config) *AuthService {
	return &AuthService{
		db:    db,
		redis: redis,
		cfg:   cfg,
	}
}

// Login authenticates a user and returns tokens
func (s *AuthService) Login(username, password string) (string, string, *models.User, error) {
	var user models.User

	// Find user by username
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil, errors.New("invalid credentials")
		}
		return "", "", nil, err
	}

	// Check if user is active
	if !user.IsActive {
		return "", "", nil, errors.New("account is deactivated")
	}

	// Verify password
	log.Printf("DEBUG: Checking password for user %s", username)
	log.Printf("DEBUG: Password from request: %s", password)
	log.Printf("DEBUG: Password hash from DB: %s", user.Password)

	if !crypto.CheckPassword(password, user.Password) {
		log.Printf("DEBUG: Password check failed!")
		return "", "", nil, errors.New("invalid credentials")
	}
	log.Printf("DEBUG: Password check passed!")

	// Generate tokens
	accessToken, err := jwtpkg.GenerateToken(user.ID.String(), jwtpkg.AccessToken, s.cfg.JWTSecret, s.cfg.JWTAccessTokenDuration)
	if err != nil {
		return "", "", nil, err
	}

	refreshToken, err := jwtpkg.GenerateToken(user.ID.String(), jwtpkg.RefreshToken, s.cfg.JWTSecret, s.cfg.JWTRefreshTokenDuration)
	if err != nil {
		return "", "", nil, err
	}

	// Store refresh token in database
	refreshTokenModel := &models.RefreshToken{
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTokenDuration),
	}

	if err := s.db.Create(refreshTokenModel).Error; err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, &user, nil
}

// Register creates a new user account
func (s *AuthService) Register(username, email, password, name string, drink1, drink2, drink3 string, inviteCode string) (*models.User, error) {
	// Check if username already exists
	var existingUser models.User
	if err := s.db.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		if existingUser.Username == username {
			return nil, errors.New("username already taken")
		}
		return nil, errors.New("email already registered")
	}

	// Verify invite code
	var invite models.InviteCode
	if err := s.db.Where("code = ?", inviteCode).First(&invite).Error; err != nil {
		return nil, errors.New("invalid invite code")
	}

	if !invite.CanBeUsedForRegistration() {
		return nil, errors.New("invite code must be viewed first before registration")
	}

	// Hash password
	hashedPassword, err := crypto.HashPassword(password, s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}

	// Create user
	user := &models.User{
		Username:           username,
		Email:              email,
		Password:           hashedPassword,
		Name:               name,
		Drink1:             drink1,
		Drink2:             drink2,
		Drink3:             drink3,
		RegisteredWithCode: inviteCode,
		Group:              invite.Group,
	}

	// Start transaction
	tx := s.db.Begin()

	// Create user
	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Mark invite code as registered
	invite.MarkAsRegistered(user.ID)
	if err := tx.Save(&invite).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()
	return user, nil
}

// RefreshToken generates new access token from refresh token
func (s *AuthService) RefreshToken(refreshToken string) (string, error) {
	// Validate refresh token
	claims, err := jwtpkg.ValidateToken(refreshToken, s.cfg.JWTSecret)
	if err != nil {
		return "", errors.New("invalid refresh token")
	}

	if claims.TokenType != jwtpkg.RefreshToken {
		return "", errors.New("invalid token type")
	}

	// Check if refresh token exists in database
	var tokenModel models.RefreshToken
	if err := s.db.Where("token = ?", refreshToken).First(&tokenModel).Error; err != nil {
		return "", errors.New("refresh token not found")
	}

	// Check if token is expired
	if time.Now().After(tokenModel.ExpiresAt) {
		return "", errors.New("refresh token expired")
	}

	// Generate new access token
	accessToken, err := jwtpkg.GenerateToken(claims.UserID, jwtpkg.AccessToken, s.cfg.JWTSecret, s.cfg.JWTAccessTokenDuration)
	if err != nil {
		return "", err
	}

	return accessToken, nil
}

// Logout invalidates the refresh token
func (s *AuthService) Logout(userID uuid.UUID) error {
	// Delete all refresh tokens for the user
	return s.db.Where("user_id = ?", userID).Delete(&models.RefreshToken{}).Error
}

// ValidateAccessToken validates an access token and returns claims
func (s *AuthService) ValidateAccessToken(token string) (*jwtpkg.Claims, error) {
	claims, err := jwtpkg.ValidateToken(token, s.cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != jwtpkg.AccessToken {
		return nil, errors.New("invalid token type")
	}

	// Optional: Check if user is blacklisted in Redis
	// If redis is down, we allow the request to proceed
	ctx := context.Background()
	blacklistKey := fmt.Sprintf("blacklist:token:%s", token)
	exists, err := s.redis.Exists(ctx, blacklistKey).Result()
	if err != nil {
		log.Printf("WARN: Could not connect to Redis to check token blacklist: %v", err)
	} else if exists > 0 {
		return nil, errors.New("token is blacklisted")
	}

	return claims, nil
}

// GetUserByID retrieves a user by ID
func (s *AuthService) GetUserByID(userID uuid.UUID) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// CleanupExpiredTokens removes expired refresh tokens
func (s *AuthService) CleanupExpiredTokens() error {
	return s.db.Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{}).Error
}
