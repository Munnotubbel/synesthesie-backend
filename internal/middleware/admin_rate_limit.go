package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/synesthesie/backend/internal/services"
)

// AdminActionRateLimit middleware to prevent mass admin actions with escalating blocks
func AdminActionRateLimit(auditService *services.AuditService, redisClient *redis.Client, maxActions, windowMinutes int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only apply to specific sensitive actions
		action := c.GetString("audit_action")
		if action == "" {
			c.Next()
			return
		}

		// Get admin ID from context
		userIDInterface, exists := c.Get("userID")
		if !exists {
			c.Next()
			return
		}

		adminID, ok := userIDInterface.(uuid.UUID)
		if !ok {
			c.Next()
			return
		}

		ctx := context.Background()
		blockKey := fmt.Sprintf("admin_blocked:%s:%s", adminID.String(), action)

		// Check if admin is currently blocked (1 hour block)
		if redisClient != nil {
			blocked, err := redisClient.Get(ctx, blockKey).Result()
			if err == nil && blocked == "1" {
				ttl, _ := redisClient.TTL(ctx, blockKey).Result()
				c.JSON(http.StatusForbidden, gin.H{
					"error":              "admin_temporarily_blocked",
					"message":            "Your account has been temporarily blocked due to suspicious activity. Please contact the system administrator.",
					"blocked_until_minutes": int(ttl.Minutes()),
				})
				c.Abort()
				return
			}
		}

		// Check rate limit
		since := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

		count, err := auditService.GetActionCount(adminID, action, since)
		if err != nil {
			// Log error but don't block the request
			c.Next()
			return
		}

		// If more than 5 actions in window, block for 1 hour
		if count >= 5 && redisClient != nil {
			// Set 1-hour block
			_ = redisClient.Set(ctx, blockKey, "1", 1*time.Hour).Err()

			c.JSON(http.StatusForbidden, gin.H{
				"error":              "admin_temporarily_blocked",
				"message":            "Too many actions detected. Your account has been temporarily blocked for 1 hour. If this was not you, please contact the system administrator immediately.",
				"blocked_for_minutes": 60,
			})
			c.Abort()
			return
		}

		// Standard rate limit check (max actions in window)
		if count >= int64(maxActions) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":               "rate_limit_exceeded",
				"message":             "Too many actions in a short time. Please wait a few minutes.",
				"retry_after_minutes": windowMinutes,
				"warning":             "Further attempts will result in a 1-hour block.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

