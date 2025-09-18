package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType represents the type of JWT token
type TokenType string

const (
	AccessToken   TokenType = "access"
	RefreshToken  TokenType = "refresh"
	CalendarToken TokenType = "calendar"
)

// Claims represents the JWT claims
type Claims struct {
	UserID    string    `json:"user_id"`
	EventID   string    `json:"event_id,omitempty"`
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

// GenerateToken generates a JWT token
func GenerateToken(userID string, tokenType TokenType, secret string, duration time.Duration) (string, error) {
	claims := Claims{
		UserID:    userID,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateCalendarToken generates a short-lived token encoding the event ID
func GenerateCalendarToken(eventID string, secret string, duration time.Duration) (string, error) {
	claims := Claims{
		EventID:   eventID,
		TokenType: CalendarToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken validates a JWT token and returns the claims
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// IsTokenValid checks if a token is valid
func IsTokenValid(tokenString string, secret string, expectedType TokenType) bool {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return false
	}

	return claims.TokenType == expectedType
}
