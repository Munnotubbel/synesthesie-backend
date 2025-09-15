package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/synesthesie/backend/internal/config"
)

// CORS creates a CORS middleware
func CORS(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		normalizedOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")

		// Debug logging
		log.Printf("CORS: Request origin: %s", origin)
		log.Printf("CORS: Allowed origins: %v", cfg.AllowedOrigins)

		// Check if origin is allowed (normalized comparison)
		allowed := false
		for _, allowedOrigin := range cfg.AllowedOrigins {
			normalizedAllowed := strings.TrimRight(strings.TrimSpace(allowedOrigin), "/")
			if normalizedOrigin == normalizedAllowed {
				allowed = true
				break
			}
		}

		// For development, if no origin matches, allow localhost origins
		if !allowed && origin != "" && cfg.Env == "development" {
			log.Printf("CORS: Development mode - allowing origin: %s", origin)
			allowed = true
		}

		if allowed && normalizedOrigin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", normalizedOrigin)
		}

		c.Writer.Header().Add("Vary", "Origin")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
