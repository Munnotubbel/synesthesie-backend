# Codebase Concerns

**Analysis Date:** 2026-02-18

## Tech Debt

**Multiple Payment Providers TODOs:**
- Issue: Missing admin alert emails for critical payment failures
- Files: `internal/services/paypal_provider.go:218`, `internal/services/paypal_provider.go:225`, `internal/handlers/paypal_handler.go:113`
- Impact: Critical payment issues won't be alerted, requiring manual monitoring
- Fix approach: Implement email notification service with proper error handling

**Payment Confirmation Email Missing:**
- Issue: PayPal webhooks don't send confirmation emails to users
- Files: `internal/handlers/paypal_handler.go:185-186`
- Impact: Users won't receive payment confirmation emails after PayPal payments
- Fix approach: Implement email confirmation logic in PayPal webhook handler

**Database Migration Grace Period:**
- Issue: Manual migrations continue despite warnings, potential data inconsistency
- Files: `internal/models/db.go:75`
- Impact: Some migration steps might fail but app continues with partial schema
- Fix approach: Implement transaction-based migrations with rollback on failure

**File Size Concerns:**
- Issue: Admin handler is very large (1991 lines), violates single responsibility principle
- Files: `internal/handlers/admin_handler.go`
- Impact: Difficult to maintain, test, and understand; high coupling
- Fix approach: Split into smaller handlers (EventHandler, InviteHandler, etc.) or use command pattern

## Known Bugs

**Grace Period Logic Race Condition:**
- Symptoms: Payments might be processed multiple times during the 5-minute grace period
- Files: `internal/services/paypal_provider.go:200-230`
- Trigger: When payment completes during ticket grace period
- Workaround: Current polling handles it but could lead to duplicate processing

**Debug Logging in Production:**
- Symptoms: Excessive debug logging output in production
- Files: `internal/services/paypal_provider.go:89`, `internal/services/paypal_provider.go:105`
- Trigger: Every PayPal order creation
- Workaround: None - logs are being printed

## Security Considerations

**Missing Webhook Verification:**
- Risk: PayPal webhooks not properly verified, potential fraud attacks
- Files: `internal/handlers/paypal_handler.go`
- Current mitigation: Basic webhook handling without signature verification
- Recommendations: Implement PayPal webhook signature verification

**Environment Variables Exposed:**
- Risk: Config uses os.Getenv directly without validation
- Files: `internal/config/config.go:269`
- Current mitigation: Defaults provided for most critical values
- Recommendations: Add environment variable validation and secure secrets management

**Hardcoded Admin Email:**
- Risk: Default admin email could be exploited
- Files: `internal/config/config.go:262`
- Current mitigation: Environment variable override possible
- Recommendations: Require admin email setup during initial configuration

## Performance Bottlenecks

**Multiple Background Polling:**
- Problem: 5 separate goroutines running continuously
- Files: `cmd/api/main.go:96-165`
- Cause: Fallback polling for webhook failures
- Improvement path: Implement event-driven architecture with retry queues

**CSV Export Memory Usage:**
- Problem: Large event exports load all data into memory
- Files: `internal/handlers/admin_handler.go:526-570`
- Cause: Full ticket data loaded before CSV generation
- Improvement path: Stream CSV generation with pagination

## Fragile Areas

**PayPal Integration Complex Logic:**
- Files: `internal/services/paypal_provider.go`, `internal/handlers/paypal_handler.go`
- Why fragile: Multiple edge cases (grace period, cancelled tickets, missing capture IDs)
- Safe modification: Comprehensive unit tests needed for all payment scenarios
- Test coverage: Partial, missing integration tests for webhook handling

**Large Admin Handler:**
- Files: `internal/handlers/admin_handler.go`
- Why fragile: Multiple responsibilities, high coupling between methods
- Safe modification: Extract smaller handlers before adding new features
- Test coverage: Likely insufficient due to complexity

## Scaling Limits

**Database Connection Pooling:**
- Current capacity: Max 100 open connections, 10 idle
- Limit: Could exhaust connections under high load
- Scaling path: Implement connection pool monitoring and auto-scaling

**Memory Usage for Events:**
- Current capacity: Limited by in-memory processing
- Limit: Large events with thousands of participants could exhaust memory
- Scaling path: Implement streaming processing for large exports

## Dependencies at Risk

**GORM ORM:**
- Risk: Active development but potential breaking changes
- Impact: All database operations would need updates
- Migration plan: Keep current version, monitor for Go 1.21+ compatibility

## Missing Critical Features

**Payment Retry Mechanism:**
- Problem: No automatic retry for failed webhook processing
- Blocks: Reliable payment confirmation in case of temporary failures

**Webhook Monitoring Dashboard:**
- Problem: No monitoring of webhook delivery status
- Blocks: Proactive detection of payment processing issues

## Test Coverage Gaps

**Webhook Handler Tests:**
- What's not tested: PayPal webhook processing logic
- Files: `internal/handlers/paypal_handler.go`
- Risk: Webhook changes could break payment processing unnoticed
- Priority: High - payment processing is critical

**Service Layer Integration Tests:**
- What's not tested: Integration between payment services and database
- Files: `internal/services/paypal_provider.go`, `internal/services/ticket_service.go`
- Risk: Database schema changes could break payment flows
- Priority: Medium

---

*Concerns audit: 2026-02-18*