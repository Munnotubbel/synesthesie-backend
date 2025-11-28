package models

import (
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/synesthesie/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB initializes the database connection
func InitDB(cfg *config.Config) (*gorm.DB, error) {
	// Include timezone in DSN
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode, cfg.DBTimeZone)

	// Load location for NowFunc
	loc, err := time.LoadLocation(cfg.DBTimeZone)
	if err != nil {
		loc = time.FixedZone("Europe/Berlin", 1*60*60) // Fallback CET
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().In(loc)
		},
		PrepareStmt: true,
	}

	if cfg.Env == "production" {
		gormConfig.Logger = logger.Default.LogMode(logger.Error)
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL database
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Database connection established")
	return db, nil
}

// InitRedis initializes Redis connection
func InitRedis(cfg *config.Config) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	log.Println("Redis connection established")
	return client
}

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	// Run manual migrations first (for existing tables)
	if err := runManualMigrations(db); err != nil {
		log.Printf("Warning: Manual migrations failed: %v", err)
		// Don't fail completely, continue with AutoMigrate
	}

	// Run AutoMigrate for all models
	return db.AutoMigrate(
		&User{},
		&Event{},
		&Ticket{},
		&InviteCode{},
		&RefreshToken{},
		&SystemSetting{},
		&Asset{},
		&PhoneVerification{},
		&PasswordReset{},
		&Backup{},
	)
}

// runManualMigrations runs manual SQL migrations for existing tables
func runManualMigrations(db *gorm.DB) error {
	log.Println("Running manual migrations...")

	// Migration: Add PayPal support columns to tickets table
	if err := addPayPalSupportToTickets(db); err != nil {
		return fmt.Errorf("failed to add PayPal support: %w", err)
	}

	log.Println("Manual migrations completed successfully")
	return nil
}

// addPayPalSupportToTickets adds PayPal-related columns to tickets table
func addPayPalSupportToTickets(db *gorm.DB) error {
	// Check if columns already exist
	var count int64
	err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_name = 'tickets'
		AND column_name = 'paypal_capture_id'
	`).Scan(&count).Error

	if err != nil {
		return err
	}

	// If column already exists, skip
	if count > 0 {
		log.Println("PayPal columns already exist in tickets table, skipping migration")
		return nil
	}

	log.Println("Adding PayPal support columns to tickets table...")

	// Add columns
	sqls := []string{
		`ALTER TABLE tickets ADD COLUMN IF NOT EXISTS payment_provider VARCHAR(20) NOT NULL DEFAULT 'stripe'`,
		`ALTER TABLE tickets ADD COLUMN IF NOT EXISTS paypal_order_id VARCHAR(255)`,
		`ALTER TABLE tickets ADD COLUMN IF NOT EXISTS paypal_capture_id VARCHAR(255)`,
	}

	for _, sql := range sqls {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("failed to execute: %s - error: %w", sql, err)
		}
	}

	log.Println("✅ PayPal support columns added successfully")
	return nil
}
