# External Integrations

**Analysis Date:** 2026-02-18

## APIs & External Services

**Payments:**
- Stripe - Payment processing
  - SDK/Client: github.com/stripe/stripe-go/v82
  - Auth: STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET
  - URLs: STRIPE_SUCCESS_URL, STRIPE_CANCEL_URL
- PayPal - Alternative payment provider
  - SDK/Client: github.com/logpacker/PayPal-Go-SDK
  - Integration: Via internal payment provider abstraction

**Databases:**
- PostgreSQL - Primary database
  - Connection: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
  - Client: GORM PostgreSQL driver
- Redis - Caching and session storage
  - Connection: REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_DB
  - Client: Redis Go client

**File Storage:**
- Cloudflare R2 - Primary file storage
  - Connection: R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY
  - Client: AWS SDK v2 for R2
- Strato S3 - Media storage (images/audio)
  - Connection: MEDIA_S3_ENDPOINT, MEDIA_S3_ACCESS_KEY_ID, MEDIA_S3_SECRET_ACCESS_KEY
  - Client: AWS SDK v2
- Backup S3 - Database backups
  - Connection: BACKUP_S3_ENDPOINT, BACKUP_S3_ACCESS_KEY_ID, BACKUP_S3_SECRET_ACCESS_KEY
  - Client: AWS SDK v2

**Email:**
- Strato SMTP - Email sending
  - Connection: SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD
  - Implementation: Internal email service

## Authentication & Identity

**Auth Provider:**
- Custom JWT authentication
  - Implementation: JWT tokens with refresh tokens
  - Keys: JWT_SECRET, JWT_ACCESS_TOKEN_DURATION, JWT_REFRESH_TOKEN_DURATION
- Admin credentials
  - Implementation: Basic auth for admin endpoints
  - Credentials: ADMIN_USERNAME, ADMIN_PASSWORD

## Monitoring & Observability

**Error Tracking:**
- Not detected (no Sentry, etc.)

**Logs:**
- Standard Go logging
- GORM logging for database operations
- Health check endpoints

## CI/CD & Deployment

**Hosting:**
- Docker containers
- Docker Compose for development
- Alpine Linux for production

**CI Pipeline:**
- Not detected (no .github workflows)

## Environment Configuration

**Required env vars:**
- Database: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE
- Redis: REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_DB
- JWT: JWT_SECRET, JWT_ACCESS_TOKEN_DURATION, JWT_REFRESH_TOKEN_DURATION
- Stripe: STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET
- PayPal: (via payment provider, keys not visible)
- Email: SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD
- S3 Services: Multiple sets of credentials for different S3 endpoints
- General: PORT, ENV, CORS settings

**Secrets location:**
- .env file (development)
- Environment variables (production)

## Webhooks & Callbacks

**Incoming:**
- Stripe webhook endpoint for payment events
- Local asset synchronization on startup

**Outgoing:**
- Payment success/cancel URLs for redirects
- Email notifications

---

*Integration audit: 2026-02-18*