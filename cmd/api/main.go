package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/handlers"
	"github.com/synesthesie/backend/internal/middleware"
	"github.com/synesthesie/backend/internal/models"
	"github.com/synesthesie/backend/internal/services"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize configuration
	cfg := config.New()

	// Initialize database
	db, err := models.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Run migrations
	if err := models.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize Redis
	redisClient := models.InitRedis(cfg)
	defer redisClient.Close()

	// Initialize services
	smsService := services.NewSMSService(cfg)
	authService := services.NewAuthService(db, redisClient, cfg, smsService)
	userService := services.NewUserService(db)
	eventService := services.NewEventService(db)
	inviteService := services.NewInviteService(db)
	ticketService := services.NewTicketService(db, cfg)
	emailService := services.NewEmailService(cfg)
	adminService := services.NewAdminService(db, cfg)
	// Attach email service so AuthService and AdminService can send emails
	authService.AttachEmailService(emailService)
	adminService.AttachEmailService(emailService)
	storageService := services.NewStorageService(cfg)
	assetService := services.NewAssetService(db, cfg)
	s3Service, err := services.NewS3Service(cfg)
	if err != nil {
		log.Fatalf("Failed to init S3 service: %v", err)
	}
	mediaService := services.NewMediaService(db, cfg, s3Service, storageService)
	musicService := services.NewMusicService(db, cfg, s3Service, storageService)
	audioCacheService := services.NewAudioCacheService(cfg, s3Service)
	qrService := services.NewQRService(cfg)
	backupService := services.NewBackupService(db, cfg, s3Service)
	auditService := services.NewAuditService(db, emailService, cfg)

	// Optional: sync missing images on start
	if cfg.MediaSyncOnStart {
		go func() {
			log.Println("MediaSyncOnStart enabled: syncing missing images...")
			prefix := "images/"
			keys, err := s3Service.ListMediaKeys(context.Background(), cfg.MediaImagesBucket, prefix, 1000)
			if err != nil {
				log.Printf("Image sync list error: %v", err)
				return
			}
			for _, k := range keys {
				abs := filepath.Join(cfg.LocalAssetsPath, filepath.FromSlash(k))
				if _, err := os.Stat(abs); err == nil {
					continue
				}
				buf, derr := s3Service.DownloadMedia(context.Background(), cfg.MediaImagesBucket, k)
				if derr != nil {
					continue
				}
				if _, _, _, err := storageService.SaveStream(context.Background(), k, bytes.NewReader(buf.Bytes())); err != nil {
					continue
				}
			}
			log.Println("MediaSyncOnStart: image sync complete")
		}()
	}

	// Start background WebP conversion worker
	if cfg.WebPConversionEnabled {
		go func() {
			// Initial delay to let the server start first
			time.Sleep(30 * time.Second)
			for {
				pending, err := mediaService.GetPendingWebPConversions()
				if err != nil {
					log.Printf("WebP conversion scan error: %v", err)
				} else if len(pending) > 0 {
					log.Printf("WebP conversion: found %d images to convert", len(pending))
					converted := 0
					for _, originalPath := range pending {
						webpPath, err := mediaService.ConvertToWebP(originalPath)
						if err != nil {
							log.Printf("WebP conversion error for %s: %v", originalPath, err)
						} else {
							converted++
							log.Printf("WebP converted: %s -> %s", filepath.Base(originalPath), filepath.Base(webpPath))
						}
						// Small delay between conversions to not overload CPU
						time.Sleep(100 * time.Millisecond)
					}
					log.Printf("WebP conversion batch complete: %d/%d converted", converted, len(pending))
				}
				// Check every 5 minutes for new images to convert
				time.Sleep(5 * time.Minute)
			}
		}()
	}

	// Start periodic cleanup for stale pending tickets
	if cfg.PendingTicketCleanupEnabled {
		go func() {
			for {
				deleted, err := ticketService.CleanupStalePending()
				if err != nil {
					log.Printf("Pending ticket cleanup error: %v", err)
				} else if deleted > 0 {
					log.Printf("Pending ticket cleanup: cancelled %d stale tickets", deleted)
				}
				time.Sleep(5 * time.Minute)
			}
		}()
	}

	// Start periodic cleanup for pending_cancellation tickets (Grace Period)
	// Finalizes cancellations after 5 minutes grace period
	go func() {
		for {
			finalized, err := ticketService.CleanupPendingCancellations()
			if err != nil {
				log.Printf("Pending cancellation cleanup error: %v", err)
			} else if finalized > 0 {
				log.Printf("Grace period cleanup: finalized %d cancelled tickets after 5 min grace period", finalized)
			}
			time.Sleep(1 * time.Minute) // Check every minute
		}
	}()

	// Fast-poll for very recent pending tickets (0-30 seconds old)
	// Checks every 5 seconds for quick user feedback (like Shopify, Airbnb)
	go func() {
		for {
			confirmed, err := ticketService.FastCheckRecentPending()
			if err != nil {
				log.Printf("Fast-poll error: %v", err)
			} else if confirmed > 0 {
				log.Printf("✅ Fast-poll: Confirmed %d payments", confirmed)
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Regular poll for older pending tickets (30 sec - 30 min old)
	// Checks every 30 seconds as fallback if webhooks fail
	go func() {
		for {
			confirmed, err := ticketService.CheckPendingPayments()
			if err != nil {
				log.Printf("Pending payment check error: %v", err)
			} else if confirmed > 0 {
				log.Printf("✅ Pending payment check: Confirmed %d payments", confirmed)
			}
			time.Sleep(30 * time.Second)
		}
	}()

	// Active check for pending_cancellation tickets (Grace Period)
	// Checks every 10 seconds during 5-minute grace period
	go func() {
		for {
			reactivated, err := ticketService.CheckPendingCancellations()
			if err != nil {
				log.Printf("Pending cancellation check error: %v", err)
			} else if reactivated > 0 {
				log.Printf("✅ Grace period: Reactivated %d tickets after finding completed payments", reactivated)
			}
			time.Sleep(10 * time.Second)
		}
	}()

	// Create admin user if not exists
	if err := adminService.CreateDefaultAdmin(); err != nil {
		log.Printf("Failed to create default admin: %v", err)
	}

	// Setup Gin router
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(cfg))
	router.Use(middleware.RateLimiter(redisClient, cfg))

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, userService, inviteService, emailService)
	userHandler := handlers.NewUserHandler(userService, eventService, ticketService, authService, emailService)
	// wire asset/storage into userHandler (exported fields)
	userHandler.AssetService = assetService
	userHandler.StorageService = storageService
	adminHandler := handlers.NewAdminHandler(adminService, eventService, inviteService, userService, ticketService, storageService, s3Service, qrService, backupService, emailService, auditService)
	publicHandler := handlers.NewPublicHandler(eventService, inviteService, cfg)
	stripeHandler := handlers.NewStripeHandler(ticketService, cfg, emailService)
	paypalHandler := handlers.NewPayPalHandler(ticketService, emailService, cfg)
	mediaHandler := handlers.NewMediaHandler(mediaService, storageService)
	musicHandler := handlers.NewMusicHandler(musicService, storageService, audioCacheService)

	// Health check outside API group (no /api/v1 prefix)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Setup routes
	api := router.Group("/api/v1")
	{
		// Health check also available under /api/v1/health for compatibility
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "healthy"})
		})

		// Catch-all OPTIONS handler for CORS preflight requests
		// This ensures all OPTIONS requests get a proper CORS response
		api.OPTIONS("/*path", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		// Public routes
		public := api.Group("/public")
		{
			public.GET("/events", publicHandler.GetUpcomingEvents)
			public.GET("/invite/:code", publicHandler.CheckInviteCode)
			public.POST("/invite/:code/view", publicHandler.ViewInviteCode)
			public.GET("/events/ics", publicHandler.GetEventICS)
		}

		// Auth routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", middleware.Auth(authService), authHandler.Logout)
			// Mobile verification (requires auth)
			auth.POST("/verify-mobile", middleware.Auth(authService), authHandler.VerifyMobile)
			auth.POST("/verify-mobile/resend", middleware.Auth(authService), authHandler.ResendMobileVerification)
			// Password reset
			auth.POST("/password/forgot", authHandler.ForgotPassword)
			auth.POST("/password/reset", authHandler.ResetPassword)
		}

		// User routes
		user := api.Group("/user")
		user.Use(middleware.Auth(authService))
		{
			user.GET("/profile", userHandler.GetProfile)
			user.PUT("/profile", userHandler.UpdateProfile)
			user.GET("/events", userHandler.GetUserEvents)
			user.GET("/tickets", userHandler.GetUserTickets)
			user.POST("/tickets", userHandler.BookTicket)
			user.POST("/tickets/:id/retry-checkout", userHandler.RetryPendingCheckout)
			user.POST("/tickets/:id/confirm-payment", userHandler.ConfirmPayment)
			user.DELETE("/tickets/:id", userHandler.CancelTicket)
			user.POST("/tickets/:id/cancel-refund", userHandler.CancelTicketRefund)
			user.POST("/tickets/:id/cancel", userHandler.CancelTicketNoRefund)
			user.GET("/assets/:id/download", userHandler.DownloadAsset)
			user.GET("/settings/pickup-price", userHandler.GetPickupServicePrice)
			// Image gallery
			user.GET("/images", mediaHandler.GetPublicImages)
			user.GET("/images/:id", mediaHandler.GetPublicImage)
			user.GET("/images/:id/file", mediaHandler.ServeImageFile) // Fast local cache serving

			// Music sets
			user.GET("/music-sets", musicHandler.GetPublicMusicSets)
			user.GET("/music-sets/:id", musicHandler.GetPublicMusicSet)
		}

		// Audio stream endpoints with token query param support (outside user group for <audio> compatibility)
		// These accept both Authorization header AND ?token=xxx query parameter
		userStream := api.Group("/user/music-sets")
		userStream.Use(handlers.TokenFromQueryMiddleware())
		userStream.Use(middleware.Auth(authService))
		{
			userStream.GET("/:id/stream", musicHandler.StreamMusicSetUser)
			userStream.GET("/:id/stream/*filepath", musicHandler.StreamMusicSetUser)
		}

		// Admin routes
		admin := api.Group("/admin")
		admin.Use(middleware.Auth(authService))
		admin.Use(middleware.AdminOnly())
		{
			// Event management
			admin.GET("/events", adminHandler.GetAllEvents)
			admin.POST("/events", adminHandler.CreateEvent)
			admin.PUT("/events/:id", adminHandler.UpdateEvent)
			admin.DELETE("/events/:id", adminHandler.DeleteEvent)
			// Specific routes BEFORE generic :id route to avoid conflicts
			admin.GET("/events/:id/drinks.xlsx", adminHandler.ExportEventDrinksXLSX)
			admin.GET("/events/:id/participants.csv", func(c *gin.Context) {
				log.Printf("DEBUG: Route /events/:id/participants.csv matched for event ID: %s", c.Param("id"))
				adminHandler.ExportEventParticipantsCSV(c)
			})
			admin.POST("/events/:id/deactivate", adminHandler.DeactivateEvent)
			admin.POST("/events/:id/refund", adminHandler.RefundEventTickets)
			admin.POST("/events/:id/announce", adminHandler.SendEventAnnouncement)
			// Generic announcement to all users
			admin.POST("/users/announce", adminHandler.SendAnnouncementToAllUsers)

			// Generic :id route last
			admin.GET("/events/:id", adminHandler.GetEventDetails)

			// Invite management
			admin.GET("/invites", adminHandler.GetAllInvites)
			admin.GET("/invites/stats", adminHandler.GetInviteStats)
			admin.POST("/invites", adminHandler.CreateInvite)
			admin.DELETE("/invites/:id", adminHandler.DeactivateInvite)
			admin.POST("/invites/:id/assign", adminHandler.AssignInvite)
			admin.GET("/invites/:id/qr.pdf", adminHandler.GetInviteQR)
			admin.GET("/invites/export.csv", adminHandler.ExportInvitesCSV)
			admin.GET("/invites/export_bubble.csv", adminHandler.ExportInvitesBubbleCSV)
			admin.GET("/invites/export_guests.csv", adminHandler.ExportInvitesGuestsCSV)
			admin.GET("/invites/export_plus.csv", adminHandler.ExportInvitesPlusCSV)
			admin.PUT("/users/:id/group", adminHandler.ReassignUserGroup)

			// User management
			admin.GET("/users", adminHandler.GetAllUsers)
			admin.GET("/users/:id", adminHandler.GetUserDetails)
			if cfg.AdminPasswordResetEnabled {
				admin.PUT("/users/:id/password", adminHandler.ResetUserPassword)
			}
			admin.PUT("/users/:id/active", adminHandler.UpdateUserActive)

			// Ticket management (with rate limiting and 1-hour block after 5 attempts)
			ticketCancelGroup := admin.Group("/tickets")
			ticketCancelGroup.Use(middleware.AdminActionRateLimit(auditService, redisClient, cfg.AdminRateLimitActions, cfg.AdminRateLimitWindowMinutes))
			{
				ticketCancelGroup.POST("/:id/cancel", adminHandler.CancelTicket)
			}

			// Audit log management
			admin.GET("/audit/logs", adminHandler.GetAuditLogs)
			admin.GET("/audit/stats", adminHandler.GetAuditStats)

			// Service price management
			admin.GET("/settings/pickup-price", adminHandler.GetPickupServicePrice)
			admin.PUT("/settings/pickup-price", adminHandler.UpdatePickupServicePrice)

			// Pickup export
			admin.GET("/pickups/export.csv", adminHandler.ExportPickupCSV)

			// Backup management (read-only for monitoring)
			admin.GET("/backups", adminHandler.GetAllBackups)
			admin.GET("/backups/stats", adminHandler.GetBackupStats)
			admin.POST("/backups/sync", adminHandler.SyncBackupsFromS3)
			// DELETE disabled for security - backups are disaster recovery!

			// Image gallery management (read operations)
			admin.GET("/images", mediaHandler.GetAllImages)
			admin.GET("/images/:id", mediaHandler.GetImageDetails)
			admin.GET("/images/:id/file", mediaHandler.ServeImageFileAdmin) // Fast local cache serving
			admin.DELETE("/images/:id", mediaHandler.DeleteImage)
			admin.PUT("/images/:id/visibility", mediaHandler.UpdateImageVisibility)
			admin.PUT("/images/:id/metadata", mediaHandler.UpdateImageMetadata)

			// Asset sync
			admin.POST("/assets/images/sync-missing", adminHandler.SyncImagesMissing)

			// Admin upload routes with rate limiting (SEC-02)
			uploadGroup := admin.Group("")
			uploadGroup.Use(middleware.UploadRateLimit(redisClient, cfg))
			{
				uploadGroup.POST("/images", mediaHandler.UploadImage)
				uploadGroup.POST("/images/batch", mediaHandler.UploadImages)
				uploadGroup.POST("/assets/upload", adminHandler.UploadAsset)
			}

			// Music set management
			admin.GET("/music-sets", musicHandler.GetAllMusicSets)
			admin.GET("/music-sets/:id", musicHandler.GetMusicSetDetails)
			admin.POST("/music-sets", musicHandler.CreateMusicSet)
			admin.PUT("/music-sets/:id", musicHandler.UpdateMusicSetMetadata)
			admin.DELETE("/music-sets/:id", musicHandler.DeleteMusicSet)
			admin.PUT("/music-sets/:id/visibility", musicHandler.UpdateMusicSetVisibility)

			// Music upload (with rate limiting)
			uploadGroup.POST("/music-sets/:id/upload", musicHandler.UploadMusicSetFile)
		}

		// Audio stream endpoints with token query param support (outside admin group for <audio> compatibility)
		// These accept both Authorization header AND ?token=xxx query parameter
		adminStream := api.Group("/admin/music-sets")
		adminStream.Use(handlers.TokenFromQueryMiddleware())
		adminStream.Use(middleware.Auth(authService))
		adminStream.Use(middleware.AdminOnly())
		{
			adminStream.GET("/:id/stream", musicHandler.StreamMusicSetAdmin)
			adminStream.GET("/:id/stream/*filepath", musicHandler.StreamMusicSetAdmin)
		}

		// Payment webhooks
		api.POST("/stripe/webhook", stripeHandler.HandleWebhook)
		api.POST("/paypal/webhook", paypalHandler.HandleWebhook)
	}

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  120 * time.Second, // 2 min for large audio uploads
		WriteTimeout: 120 * time.Second, // 2 min for large responses
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting server on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
