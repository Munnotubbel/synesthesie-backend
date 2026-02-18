# Coding Conventions

**Analysis Date:** 2026-02-18

## Code Style

### Go Formatting
- Standard `gofmt` formatting
- Tabs for indentation
- Import grouping: standard library, external packages, internal packages

### Naming

**Packages:**
- Lowercase, single word: `handlers`, `services`, `models`, `config`

**Types:**
- PascalCase for exported: `UserService`, `AuthHandler`
- camelCase for unexported: `db`, `cfg`

**Functions/Methods:**
- PascalCase for exported: `NewUserService()`, `GetProfile()`
- camelCase for unexported: `getEnv()`, `validateEmail()`

**Constants:**
- PascalCase: `StatusPending`, `StatusConfirmed`

## Patterns

### Dependency Injection
Services are initialized with explicit dependencies:
```go
authService := services.NewAuthService(db, redisClient, cfg, smsService)
```

### Constructor Pattern
```go
func NewUserService(db *gorm.DB) *UserService {
    return &UserService{db: db}
}
```

### Interface Abstraction
Payment providers use an interface:
```go
type PaymentProvider interface {
    CreateCheckoutSession(...) (*PaymentSession, error)
}
```

### Handler Struct Pattern
```go
type UserHandler struct {
    userService   *services.UserService
    eventService  *services.EventService
    // ...
}
```

### Configuration
Environment variables via helper functions:
```go
func getEnv(key, defaultValue string) string
func getEnvAsInt(key string, defaultValue int) int
func getEnvAsDuration(key, defaultValue string) time.Duration
```

## Error Handling

### Service Layer
```go
if err != nil {
    return nil, fmt.Errorf("failed to ...: %w", err)
}
```

### Handler Layer
```go
if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
}
```

### Database Errors
GORM errors checked explicitly:
```go
if errors.Is(err, gorm.ErrRecordNotFound) {
    // handle not found
}
```

## HTTP Response Patterns

### Success Response
```go
c.JSON(http.StatusOK, gin.H{
    "data": result,
})
```

### Error Response
```go
c.JSON(http.StatusBadRequest, gin.H{
    "error": "error message",
})
```

### Validation
Early return pattern:
```go
if req.Field == "" {
    c.JSON(http.StatusBadRequest, gin.H{"error": "field required"})
    return
}
```

## Middleware Patterns

### Authentication
```go
func Auth(authService *services.AuthService) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Extract and validate token
        c.Next()
    }
}
```

### Admin-Only
```go
func AdminOnly() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Check admin status
        c.Next()
    }
}
```

## Logging

### Standard Logging
```go
log.Printf("Message: %v", value)
log.Fatalf("Critical error: %v", err)
log.Println("Status message")
```

### Debug Logging
```go
log.Printf("DEBUG: ...")
```

## Context Usage

### HTTP Handlers
```go
c *gin.Context  // Gin context
```

### Background Operations
```go
ctx := context.Background()
```

### External Service Calls
```go
result, err := s3Service.DownloadMedia(ctx, bucket, key)
```

## Database Patterns

### GORM Queries
```go
db.Where("field = ?", value).First(&model)
db.Preload("Relation").Find(&models)
```

### Migrations
```go
db.AutoMigrate(&Model{})
```

## JSON Tags

Consistent snake_case for JSON:
```go
type User struct {
    ID        string `json:"id"`
    Email     string `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}
```

---

*Conventions analysis: 2026-02-18*
