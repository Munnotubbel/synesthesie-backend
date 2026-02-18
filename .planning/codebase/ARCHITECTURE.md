# Architecture

**Analysis Date:** 2026-02-18

## Pattern

**Architecture Style:** Layered Architecture with Service-Oriented Design

The codebase follows a classic 3-tier architecture:
1. **Handlers Layer** - HTTP request handling, validation, response formatting
2. **Services Layer** - Business logic, orchestration, external integrations
3. **Models Layer** - Data models, database operations, migrations

## Layers

### Handlers (`internal/handlers/`)
- **auth_handler.go** - Registration, login, token refresh, password reset
- **user_handler.go** - Profile management, ticket booking, asset downloads
- **admin_handler.go** - Admin operations for events, invites, users, backups
- **public_handler.go** - Public endpoints for event info, invite codes
- **stripe_handler.go** - Stripe webhook handling
- **paypal_handler.go** - PayPal webhook handling

### Services (`internal/services/`)
- **auth_service.go** - Authentication, JWT tokens, verification
- **user_service.go** - User CRUD operations
- **event_service.go** - Event management
- **ticket_service.go** - Ticket booking, payment confirmation
- **invite_service.go** - Invite code management
- **email_service.go** - Email sending via SMTP
- **sms_service.go** - SMS verification
- **payment_provider.go** - Payment abstraction interface
- **stripe_provider.go** - Stripe implementation
- **paypal_provider.go** - PayPal implementation
- **storage_service.go** - Local file storage
- **s3_service.go** - S3-compatible cloud storage
- **backup_service.go** - Database backups
- **audit_service.go** - Audit logging
- **admin_service.go** - Admin operations
- **asset_service.go** - Asset management
- **qr_service.go** - QR code generation

### Models (`internal/models/`)
- **db.go** - Database initialization, migrations
- **redis.go** - Redis client setup
- Domain models: User, Event, Ticket, Invite, etc.

### Middleware (`internal/middleware/`)
- **auth.go** - JWT authentication
- **cors.go** - CORS handling
- **rate_limiter.go** - Request rate limiting
- **admin_rate_limit.go** - Admin action rate limiting

## Data Flow

```
HTTP Request
    ↓
Router (Gin)
    ↓
Middleware (Auth, CORS, Rate Limit)
    ↓
Handler (Validation, Response)
    ↓
Service (Business Logic)
    ↓
Model/Repository (Database)
    ↓
External Services (Stripe, PayPal, S3, SMTP)
```

## Entry Points

**Main Entry:** `cmd/api/main.go`
- Loads configuration
- Initializes database (PostgreSQL + Redis)
- Sets up all services with dependency injection
- Configures Gin router with routes
- Starts HTTP server with graceful shutdown

**Background Processes:**
- Pending ticket cleanup (5 min interval)
- Grace period cleanup (1 min interval)
- Fast-poll for recent payments (5 sec interval)
- Regular payment check (30 sec interval)
- Media sync on start (optional)

## Abstractions

**Payment Provider Interface:**
```go
type PaymentProvider interface {
    CreateCheckoutSession(...) (*PaymentSession, error)
    GetCheckoutSession(...) (*PaymentSession, error)
    // ...
}
```
Allows swapping between Stripe and PayPal implementations.

**Storage Abstraction:**
- Local filesystem via `storage_service.go`
- S3-compatible via `s3_service.go`
- Multiple S3 endpoints for different purposes

## Key Dependencies Between Components

- Handlers depend on Services (not Models directly)
- Services depend on Models and external clients
- Models handle all database operations via GORM
- Middleware depends on AuthService and Redis

---

*Architecture analysis: 2026-02-18*
