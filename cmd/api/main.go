package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	authService := services.NewAuthService(db, redisClient, cfg)
	userService := services.NewUserService(db)
	eventService := services.NewEventService(db)
	inviteService := services.NewInviteService(db)
	ticketService := services.NewTicketService(db, cfg)
	emailService := services.NewEmailService(cfg)
	adminService := services.NewAdminService(db, cfg)

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
	userHandler := handlers.NewUserHandler(userService, eventService, ticketService)
	adminHandler := handlers.NewAdminHandler(adminService, eventService, inviteService, userService)
	publicHandler := handlers.NewPublicHandler(eventService, inviteService)
	stripeHandler := handlers.NewStripeHandler(ticketService, cfg)

	// Setup routes
	api := router.Group("/api/v1")
	{
		// Health check
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "healthy"})
		})

		// Public routes
		public := api.Group("/public")
		{
			public.GET("/events", publicHandler.GetUpcomingEvents)
			public.GET("/invite/:code", publicHandler.CheckInviteCode)
			public.POST("/invite/:code/view", publicHandler.ViewInviteCode)
		}

		// Auth routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", middleware.Auth(authService), authHandler.Logout)
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
			user.DELETE("/tickets/:id", userHandler.CancelTicket)
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
			admin.POST("/events/:id/deactivate", adminHandler.DeactivateEvent)
			admin.POST("/events/:id/refund", adminHandler.RefundEventTickets)

			// Invite management
			admin.GET("/invites", adminHandler.GetAllInvites)
			admin.POST("/invites", adminHandler.CreateInvite)
			admin.DELETE("/invites/:id", adminHandler.DeactivateInvite)

			// User management
			admin.GET("/users", adminHandler.GetAllUsers)
			admin.GET("/users/:id", adminHandler.GetUserDetails)
			admin.PUT("/users/:id/password", adminHandler.ResetUserPassword)

			// Service price management
			admin.GET("/settings/pickup-price", adminHandler.GetPickupServicePrice)
			admin.PUT("/settings/pickup-price", adminHandler.UpdatePickupServicePrice)
		}

		// Stripe webhook
		api.POST("/stripe/webhook", stripeHandler.HandleWebhook)
	}

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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
