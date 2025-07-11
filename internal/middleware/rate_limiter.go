package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/synesthesie/backend/internal/config"
)

// RateLimiter creates a rate limiting middleware
func RateLimiter(redisClient *redis.Client, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()

		// If Redis is not available, bypass the rate limiter
		if err := redisClient.Ping(ctx).Err(); err != nil {
			log.Printf("WARN: Redis not available for rate limiting: %v", err)
			c.Next()
			return
		}

		// Get client IP
		clientIP := c.ClientIP()
		key := fmt.Sprintf("rate_limit:%s", clientIP)

		// Check current count
		count, err := redisClient.Get(ctx, key).Int()
		if err == redis.Nil {
			// First request
			err = redisClient.Set(ctx, key, 1, cfg.RateLimitDuration).Err()
			if err != nil {
				// Log error and bypass if Redis fails
				log.Printf("WARN: Rate limiter failed to set key: %v", err)
				c.Next()
				return
			}
		} else if err != nil {
			// Log error and bypass if Redis fails
			log.Printf("WARN: Rate limiter failed to get key: %v", err)
			c.Next()
			return
		} else if count >= cfg.RateLimitRequests {
			// Rate limit exceeded
			ttl, _ := redisClient.TTL(ctx, key).Result()
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RateLimitRequests))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Too many requests",
				"retry_after": ttl.Seconds(),
			})
			c.Abort()
			return
		} else {
			// Increment counter
			newCount, _ := redisClient.Incr(ctx, key).Result()
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RateLimitRequests))
			c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", cfg.RateLimitRequests-int(newCount)))
		}

		c.Next()
	}
}
