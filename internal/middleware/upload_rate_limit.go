package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/synesthesie/backend/internal/config"
)

// UploadRateLimit creates a rate limiting middleware specifically for admin uploads
// Prevents abuse by limiting the number of uploads per admin within a time window
func UploadRateLimit(redisClient *redis.Client, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()

		// Only apply to upload endpoints
		if c.Request.Method != "POST" {
			c.Next()
			return
		}

		// Check if this is an upload endpoint
		path := c.Request.URL.Path
		if !isUploadEndpoint(path) {
			c.Next()
			return
		}

		// Get admin ID from context (set by Auth middleware)
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

		// Rate limit key: upload_limit:{admin_id}:{date}
		// Resets daily at midnight for predictable behavior
		today := time.Now().Format("2006-01-02")
		key := fmt.Sprintf("upload_limit:%s:%s", adminID.String(), today)

		// Check current count
		count, err := redisClient.Get(ctx, key).Int()
		if err == redis.Nil {
			// First upload today
			// Set with expiration until midnight
			now := time.Now()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			ttl := midnight.Sub(now)
			err = redisClient.Set(ctx, key, 1, ttl).Err()
			if err != nil {
				// Log error but don't block upload
				c.Next()
				return
			}
		} else if err != nil {
			// Redis error - don't block upload
			c.Next()
			return
		} else if count >= cfg.UploadMaxConcurrent*10 {
			// Rate limit: max 10x the concurrent limit per day
			// For default config (3 concurrent), this = 30 uploads/day
			ttl, _ := redisClient.TTL(ctx, key).Result()
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":               "upload_rate_limit_exceeded",
				"message":             "Too many uploads today. Please try again tomorrow.",
				"retry_after_hours":   int(ttl.Hours()),
				"uploads_today":       count,
				"max_uploads_per_day": cfg.UploadMaxConcurrent * 10,
			})
			c.Abort()
			return
		} else {
			// Increment counter
			redisClient.Incr(ctx, key)
		}

		c.Next()
	}
}

// isUploadEndpoint checks if the path is an upload endpoint
func isUploadEndpoint(path string) bool {
	// Check for image upload endpoints
	if path == "/api/v1/admin/images" || path == "/api/v1/admin/images/batch" {
		return true
	}
	// Check for asset upload endpoint (existing)
	if path == "/api/v1/admin/assets/upload" {
		return true
	}
	return false
}
