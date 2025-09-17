package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Server
	Port        string
	Env         string
	APIUrl      string
	FrontendURL string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	DBTimeZone string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	// JWT
	JWTSecret               string
	JWTAccessTokenDuration  time.Duration
	JWTRefreshTokenDuration time.Duration

	// Admin
	AdminUsername string
	AdminPassword string
	AdminEmail    string

	// Stripe
	StripeSecretKey     string
	StripeWebhookSecret string
	StripeSuccessURL    string
	StripeCancelURL     string

	// SMTP
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string

	// Cloudflare R2
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicURL       string

	// Media S3 (Strato) - separate account for media
	MediaS3Endpoint        string
	MediaS3Region          string
	MediaS3AccessKeyID     string
	MediaS3SecretAccessKey string
	MediaS3UsePathStyle    bool
	MediaImagesBucket      string
	MediaAudioBucket       string

	// Backup S3 - separate account for daily DB dumps
	BackupS3Endpoint        string
	BackupS3Region          string
	BackupS3AccessKeyID     string
	BackupS3SecretAccessKey string
	BackupS3UsePathStyle    bool
	BackupBucket            string

	// Local storage
	LocalAssetsPath  string
	MediaSyncOnStart bool

	// Media caching
	MediaCacheAudio bool
	AudioCachePath  string

	// Security
	BcryptCost        int
	RateLimitRequests int
	RateLimitDuration time.Duration

	// CORS
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string

	// SMS Verification
	SMSVerificationEnabled bool
	SMSProvider            string // "seven" | "clicksend"
	SMSFrom                string
	SevenAPIKey            string

	// Legacy ClickSend (optional for backwards compat)
	ClickSendUsername string
	ClickSendAPIKey   string
	ClickSendFrom     string
}

func New() *Config {
	return &Config{
		// Server
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		APIUrl:      getEnv("API_URL", "http://localhost:8080"),
		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),

		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5433"),
		DBUser:     getEnv("DB_USER", "synesthesie"),
		DBPassword: getEnv("DB_PASSWORD", "password"),
		DBName:     getEnv("DB_NAME", "synesthesie_db"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		DBTimeZone: getEnv("DB_TIMEZONE", "Europe/Berlin"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		// JWT
		JWTSecret:               getEnv("JWT_SECRET", "your-secret-key"),
		JWTAccessTokenDuration:  getEnvAsDuration("JWT_ACCESS_TOKEN_DURATION", "1h"),
		JWTRefreshTokenDuration: getEnvAsDuration("JWT_REFRESH_TOKEN_DURATION", "168h"),

		// Admin
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),
		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@synesthesie.de"),

		// Stripe
		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripeSuccessURL:    getEnv("STRIPE_SUCCESS_URL", "https://synesthesie.de/payment/success"),
		StripeCancelURL:     getEnv("STRIPE_CANCEL_URL", "https://synesthesie.de/payment/cancel"),

		// SMTP
		SMTPHost:     getEnv("SMTP_HOST", "smtp.strato.de"),
		SMTPPort:     getEnvAsInt("SMTP_PORT", 465),
		SMTPUsername: getEnv("SMTP_USERNAME", "info@synesthesie.de"),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", "info@synesthesie.de"),
		SMTPFromName: getEnv("SMTP_FROM_NAME", "Synesthesie"),

		// Cloudflare R2
		R2AccountID:       getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:     getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey: getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:      getEnv("R2_BUCKET_NAME", "synesthesie-storage"),
		R2PublicURL:       getEnv("R2_PUBLIC_URL", ""),

		// Media S3 (Strato)
		MediaS3Endpoint:        getEnv("MEDIA_S3_ENDPOINT", ""),
		MediaS3Region:          getEnv("MEDIA_S3_REGION", "us-east-1"),
		MediaS3AccessKeyID:     getEnv("MEDIA_S3_ACCESS_KEY_ID", ""),
		MediaS3SecretAccessKey: getEnv("MEDIA_S3_SECRET_ACCESS_KEY", ""),
		MediaS3UsePathStyle:    getEnv("MEDIA_S3_USE_PATH_STYLE", "true") == "true",
		MediaImagesBucket:      getEnv("MEDIA_IMAGES_BUCKET", "synesthesie-images"),
		MediaAudioBucket:       getEnv("MEDIA_AUDIO_BUCKET", "synesthesie-audio"),

		// Backup S3
		BackupS3Endpoint:        getEnv("BACKUP_S3_ENDPOINT", ""),
		BackupS3Region:          getEnv("BACKUP_S3_REGION", "us-east-1"),
		BackupS3AccessKeyID:     getEnv("BACKUP_S3_ACCESS_KEY_ID", ""),
		BackupS3SecretAccessKey: getEnv("BACKUP_S3_SECRET_ACCESS_KEY", ""),
		BackupS3UsePathStyle:    getEnv("BACKUP_S3_USE_PATH_STYLE", "true") == "true",
		BackupBucket:            getEnv("BACKUP_BUCKET", "synesthesie-backups"),

		// Local storage
		LocalAssetsPath:  getEnv("LOCAL_ASSETS_PATH", "/data/assets"),
		MediaSyncOnStart: getEnv("MEDIA_SYNC_ON_START", "false") == "true",

		// Media caching
		MediaCacheAudio: getEnv("MEDIA_CACHE_AUDIO", "false") == "true",
		AudioCachePath:  getEnv("AUDIO_CACHE_PATH", "/data/assets_cache/audio"),

		// Security
		BcryptCost:        getEnvAsInt("BCRYPT_COST", 12),
		RateLimitRequests: getEnvAsInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitDuration: getEnvAsDuration("RATE_LIMIT_DURATION", "1m"),

		// CORS
		AllowedOrigins: getEnvAsSlice("ALLOWED_ORIGINS", []string{"http://localhost:3000", "https://synesthesie.de"}),
		AllowedMethods: getEnvAsSlice("ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		AllowedHeaders: getEnvAsSlice("ALLOWED_HEADERS", []string{"Content-Type", "Authorization"}),

		// SMS Verification
		SMSVerificationEnabled: getEnv("SMS_VERIFICATION_ENABLED", "true") == "true",
		SMSProvider:            getEnv("SMS_PROVIDER", "seven"),
		SMSFrom:                getEnv("SMS_FROM", "Synesthesie"),
		SevenAPIKey:            getEnv("SEVEN_API_KEY", ""),

		// Legacy ClickSend
		ClickSendUsername: getEnv("CLICKSEND_USERNAME", ""),
		ClickSendAPIKey:   getEnv("CLICKSEND_API_KEY", ""),
		ClickSendFrom:     getEnv("CLICKSEND_FROM", "Synesthesie"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue string) time.Duration {
	valueStr := getEnv(key, defaultValue)
	if duration, err := time.ParseDuration(valueStr); err == nil {
		return duration
	}
	if duration, err := time.ParseDuration(defaultValue); err == nil {
		return duration
	}
	return time.Hour
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	return strings.Split(valueStr, ",")
}
