# Directory Structure

**Analysis Date:** 2026-02-18

## Root Layout

```
synesthesie-backend/
├── cmd/
│   └── api/
│       └── main.go           # Application entry point
├── internal/
│   ├── config/               # Configuration loading
│   ├── handlers/             # HTTP handlers (controllers)
│   ├── middleware/           # HTTP middleware
│   ├── models/               # Database models & migrations
│   └── services/             # Business logic services
├── templates/                # Email templates (HTML)
├── .planning/                # GSD planning documents
├── .env                      # Environment variables
├── docker-compose.yml        # Production Docker setup
├── docker-compose.dev.yml    # Development Docker setup
├── Dockerfile                # Multi-stage build
├── Makefile                  # Build automation
├── go.mod                    # Go module definition
├── go.sum                    # Dependency checksums
└── package.json              # Frontend dependencies
```

## Key Locations

### Configuration
- `internal/config/config.go` - All configuration loading via environment variables

### Handlers
- `internal/handlers/auth_handler.go` - Authentication endpoints
- `internal/handlers/user_handler.go` - User-facing endpoints
- `internal/handlers/admin_handler.go` - Admin-only endpoints (large file!)
- `internal/handlers/public_handler.go` - Public endpoints (no auth)
- `internal/handlers/stripe_handler.go` - Stripe webhook handling
- `internal/handlers/paypal_handler.go` - PayPal webhook handling

### Services
- `internal/services/auth_service.go` - Authentication logic
- `internal/services/user_service.go` - User management
- `internal/services/ticket_service.go` - Ticket operations
- `internal/services/event_service.go` - Event management
- `internal/services/invite_service.go` - Invite codes
- `internal/services/payment_provider.go` - Payment interface
- `internal/services/stripe_provider.go` - Stripe implementation
- `internal/services/paypal_provider.go` - PayPal implementation
- `internal/services/email_service.go` - Email sending
- `internal/services/s3_service.go` - S3 operations
- `internal/services/storage_service.go` - Local storage
- `internal/services/backup_service.go` - Database backups
- `internal/services/audit_service.go` - Audit logging

### Models
- `internal/models/db.go` - Database init & migrations
- `internal/models/redis.go` - Redis client
- Domain models in same directory

### Middleware
- `internal/middleware/auth.go` - JWT authentication
- `internal/middleware/cors.go` - CORS handling
- `internal/middleware/rate_limiter.go` - Rate limiting
- `internal/middleware/admin_rate_limit.go` - Admin rate limiting

### Templates
- `templates/*.html` - Email templates for various notifications

## Naming Conventions

### Files
- `*_handler.go` - HTTP handlers
- `*_service.go` - Business logic services
- `*_provider.go` - External service implementations
- `*_test.go` - Test files (when present)

### Go Packages
- `config` - Configuration
- `handlers` - HTTP handlers
- `middleware` - HTTP middleware
- `models` - Database models
- `services` - Business services

### Functions
- `NewXxx()` - Constructor functions
- `InitDB()`, `InitRedis()` - Initialization
- Handler methods on struct types

## Docker Setup

- `docker-compose.yml` - Production: postgres, redis, api
- `docker-compose.dev.yml` - Development: adds adminer
- `Dockerfile` - Multi-stage build for Alpine

## Make Targets

```
make dev       - Start dev environment with docker-compose
make dev-deps  - Start only postgres + redis
make run       - Run API locally
make test      - Run tests
make build     - Build binary
```

---

*Structure analysis: 2026-02-18*
