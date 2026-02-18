# Architecture Patterns

**Domain:** Media management for event platform
**Researched:** 2025-02-18
**Overall confidence:** HIGH

## Existing Architecture Analysis

The codebase follows a **clean layered Go architecture** with clear separation:

```
cmd/api/main.go              # Entry point, dependency injection
internal/
├── config/                  # Configuration management
├── handlers/                # HTTP handlers (controllers)
│   ├── admin_handler.go     # Admin endpoints
│   ├── user_handler.go      # User endpoints
│   ├── auth_handler.go      # Auth endpoints
│   └── ...
├── services/                # Business logic layer
│   ├── s3_service.go        # S3 operations (media)
│   ├── storage_service.go   # Local file operations
│   ├── asset_service.go     # Asset database operations
│   └── ...
├── models/                  # GORM models
│   ├── asset.go             # Asset model with Visibility
│   ├── user.go
│   ├── event.go
│   └── ...
└── middleware/              # HTTP middleware
    ├── auth.go              # JWT auth + AdminOnly
    └── ...
```

**Key architectural patterns observed:**

1. **Dependency Injection via Constructor Functions**
   - Each service has a `New*Service(db, cfg, ...)` constructor
   - Handlers receive services as constructor dependencies
   - All wiring happens in `main.go`

2. **Middleware Pattern for Cross-Cutting Concerns**
   - `middleware.Auth(authService)` - validates JWT, sets user context
   - `middleware.AdminOnly()` - checks admin flag
   - Chained as `user.Use(middleware.Auth(authService))`

3. **Service Layer Abstraction**
   - Services contain business logic
   - Handlers are thin - they parse requests, call services, format responses
   - No GORM calls in handlers

4. **Model-First Database Design**
   - GORM `AutoMigrate` runs on startup in `models.Migrate()`
   - All models registered in `internal/models/db.go`

## Recommended Architecture for Media Management

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `MediaService` (new) | Media business logic: upload, visibility, queries | `S3Service`, `AssetService`, `ImageRepository`, `MusicSetRepository` |
| `ImageRepository` (new) | Image model CRUD operations | GORM `db` |
| `MusicSetRepository` (new) | MusicSet model CRUD operations | GORM `db` |
| `MediaHandler` (new) | HTTP endpoints for media | `MediaService` |
| `AdminHandler` (extend) | Admin-only media operations | `MediaService` |
| `UserHandler` (extend) | User-facing media operations | `MediaService` |
| `S3Service` (existing) | S3 upload/download/presign | Called by `MediaService` |
| `AssetService` (existing) | Generic Asset operations | Called by `MediaService` |

### Data Flow

**Upload Flow (Admin):**
```
AdminHandler.UploadImage() / AdminHandler.UploadMusicSet()
  -> MediaService.UploadImage() / MediaService.UploadMusicSet()
    -> S3Service.UploadMedia() [upload to S3]
    -> ImageRepository.Create() / MusicSetRepository.Create() [persist metadata]
    -> AssetService.CreateAssetRecord() [create Asset record for tracking]
```

**View Flow (User):**
```
UserHandler.GetImages() / UserHandler.GetMusicSets()
  -> MediaService.GetVisibleImages() / MediaService.GetVisibleMusicSets()
    -> ImageRepository.FindByVisibility() / MusicSetRepository.FindByVisibility()
    -> S3Service.PresignMediaGet() [generate signed URLs]
```

**Download Flow (User):**
```
UserHandler.DownloadImage() / UserHandler.DownloadMusicSet()
  -> MediaService.GetSignedURL()
    -> S3Service.PresignMediaGet()
```

## New Models Needed

### 1. Image Model

**Location:** `internal/models/image.go`

**Purpose:** Represent images with visibility controls (gallery photos, event photos, etc.)

```go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type ImageVisibility string

const (
    ImageVisibilityPrivate ImageVisibility = "private" // Admin only
    ImageVisibilityPublic  ImageVisibility = "public"  // All users
)

type Image struct {
    ID          uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
    AssetID     uuid.UUID       `gorm:"type:uuid;not null" json:"asset_id"` // References Asset
    Title       string          `gorm:"size:255" json:"title"`
    Description string          `gorm:"type:text" json:"description"`
    Visibility  ImageVisibility `gorm:"size:16;default:private" json:"visibility"`
    EventID     *uuid.UUID      `gorm:"type:uuid" json:"event_id,omitempty"` // Optional event association
    Position    int             `gorm:"default:0" json:"position"` // For ordering
    CreatedAt   time.Time       `json:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at"`

    // Relations
    Asset Asset `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
    Event *Event `gorm:"foreignKey:EventID" json:"event,omitempty"`
}

func (i *Image) BeforeCreate(tx *gorm.DB) error {
    if i.ID == uuid.Nil {
        i.ID = uuid.New()
    }
    return nil
}
```

### 2. MusicSet Model

**Location:** `internal/models/music_set.go`

**Purpose:** Represent music sets/playlists with visibility controls

```go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type MusicSetVisibility string

const (
    MusicSetVisibilityPrivate MusicSetVisibility = "private" // Admin only
    MusicSetVisibilityPublic  MusicSetVisibility = "public"  // All users
)

type MusicSet struct {
    ID          uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
    Title       string             `gorm:"size:255;not null" json:"title"`
    Description string             `gorm:"type:text" json:"description"`
    Visibility  MusicSetVisibility `gorm:"size:16;default:private" json:"visibility"`
    EventID     *uuid.UUID         `gorm:"type:uuid" json:"event_id,omitempty"` // Optional event association
    Position    int                `gorm:"default:0" json:"position"` // For ordering
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`

    // Relations - one-to-many with tracks
    Tracks []MusicTrack `gorm:"foreignKey:MusicSetID" json:"tracks,omitempty"`
    Event  *Event       `gorm:"foreignKey:EventID" json:"event,omitempty"`
}

func (m *MusicSet) BeforeCreate(tx *gorm.DB) error {
    if m.ID == uuid.Nil {
        m.ID = uuid.New()
    }
    return nil
}

// MusicTrack represents individual tracks in a music set
type MusicTrack struct {
    ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
    MusicSetID  uuid.UUID `gorm:"type:uuid;not null" json:"music_set_id"`
    AssetID     uuid.UUID `gorm:"type:uuid;not null" json:"asset_id"` // References Asset (audio file)
    Title       string    `gorm:"size:255" json:"title"`
    Artist      string    `gorm:"size:255" json:"artist"`
    Duration    int       `json:"duration"` // Seconds
    Position    int       `gorm:"default:0" json:"position"` // For ordering within set
    CreatedAt   time.Time `json:"created_at"`

    // Relations
    MusicSet MusicSet `gorm:"foreignKey:MusicSetID" json:"-"`
    Asset    Asset    `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
}

func (t *MusicTrack) BeforeCreate(tx *gorm.DB) error {
    if t.ID == uuid.Nil {
        t.ID = uuid.New()
    }
    return nil
}
```

### 3. Update `db.go` for Migrations

**Location:** `internal/models/db.go`

**Change:** Add new models to `AutoMigrate` list

```go
func Migrate(db *gorm.DB) error {
    // ... existing manual migrations ...

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
        &Image{},           // NEW
        &MusicSet{},        // NEW
        &MusicTrack{},      // NEW
    )
}
```

## New Services Needed

### 1. ImageRepository

**Location:** `internal/repositories/image_repository.go` (new directory)

**Purpose:** CRUD operations for Image model

**Why a new `repositories` package?**
- Separates data access from business logic
- Follows the repository pattern already implicit in the codebase
- Makes testing easier (can mock repositories)

```go
package repositories

import (
    "github.com/google/uuid"
    "gorm.io/gorm"
    "github.com/synesthesie/backend/internal/models"
)

type ImageRepository interface {
    Create(image *models.Image) error
    GetByID(id uuid.UUID) (*models.Image, error)
    GetAll(visibility models.ImageVisibility) ([]models.Image, error)
    Update(image *models.Image) error
    Delete(id uuid.UUID) error
    GetByEventID(eventID uuid.UUID) ([]models.Image, error)
}

type imageRepository struct {
    db *gorm.DB
}

func NewImageRepository(db *gorm.DB) ImageRepository {
    return &imageRepository{db: db}
}

// Implementation...
```

### 2. MusicSetRepository

**Location:** `internal/repositories/music_set_repository.go`

**Purpose:** CRUD operations for MusicSet and MusicTrack models

```go
package repositories

type MusicSetRepository interface {
    Create(musicSet *models.MusicSet) error
    GetByID(id uuid.UUID) (*models.MusicSet, error)
    GetAll(visibility models.MusicSetVisibility) ([]models.MusicSet, error)
    Update(musicSet *models.MusicSet) error
    Delete(id uuid.UUID) error
    AddTrack(musicSetID uuid.UUID, track *models.MusicTrack) error
    RemoveTrack(trackID uuid.UUID) error
}

type musicSetRepository struct {
    db *gorm.DB
}

func NewMusicSetRepository(db *gorm.DB) MusicSetRepository {
    return &musicSetRepository{db: db}
}
```

### 3. MediaService (Business Logic)

**Location:** `internal/services/media_service.go`

**Purpose:** Orchestrates media operations (S3 + database)

```go
package services

import (
    "context"
    "github.com/google/uuid"
    "github.com/synesthesie/backend/internal/config"
    "github.com/synesthesie/backend/internal/models"
    "github.com/synesthesie/backend/internal/repositories"
)

type MediaService struct {
    db                *gorm.DB
    cfg               *config.Config
    s3Service         *S3Service
    assetService      *AssetService
    imageRepo         repositories.ImageRepository
    musicSetRepo      repositories.MusicSetRepository
}

func NewMediaService(
    db *gorm.DB,
    cfg *config.Config,
    s3Service *S3Service,
    assetService *AssetService,
    imageRepo repositories.ImageRepository,
    musicSetRepo repositories.MusicSetRepository,
) *MediaService {
    return &MediaService{
        db:           db,
        cfg:          cfg,
        s3Service:    s3Service,
        assetService: assetService,
        imageRepo:    imageRepo,
        musicSetRepo: musicSetRepo,
    }
}

// Image operations
func (s *MediaService) UploadImage(ctx context.Context, req ImageUploadRequest) (*models.Image, error) {
    // 1. Upload to S3
    // 2. Create Asset record
    // 3. Create Image record
    // 4. Return Image with signed URL
}

func (s *MediaService) GetVisibleImages(userIsAdmin bool) ([]ImageDTO, error) {
    visibility := models.ImageVisibilityPublic
    if userIsAdmin {
        visibility = "" // Get all
    }
    images, err := s.imageRepo.GetAll(visibility)
    // ... add signed URLs ...
}

func (s *MediaService) GetImageSignedURL(imageID uuid.UUID) (string, error) {
    // 1. Get image
    // 2. Get asset
    // 3. Generate presigned URL
}

// MusicSet operations
func (s *MediaService) CreateMusicSet(ctx context.Context, req CreateMusicSetRequest) (*models.MusicSet, error) {
    // 1. Validate tracks
    // 2. Upload tracks to S3
    // 3. Create Asset records
    // 4. Create MusicSet with tracks
}

func (s *MediaService) GetVisibleMusicSets(userIsAdmin bool) ([]MusicSetDTO, error) {
    // Similar to GetVisibleImages
}
```

## New Handlers Needed

### 1. MediaHandler (Dedicated media endpoints)

**Location:** `internal/handlers/media_handler.go`

**Purpose:** Dedicated endpoints for media operations

```go
package handlers

import (
    "github.com/gin-gonic/gin"
    "github.com/synesthesie/backend/internal/services"
)

type MediaHandler struct {
    mediaService *services.MediaService
    authService  *services.AuthService
}

func NewMediaHandler(mediaService *services.MediaService, authService *services.AuthService) *MediaHandler {
    return &MediaHandler{
        mediaService: mediaService,
        authService:  authService,
    }
}

// Public endpoints (no auth required for public content)
func (h *MediaHandler) GetPublicImages(c *gin.Context) { ... }
func (h *MediaHandler) GetPublicMusicSets(c *gin.Context) { ... }

// Authenticated user endpoints
func (h *MediaHandler) GetImages(c *gin.Context) { ... }      // Returns user-visible images
func (h *MediaHandler) GetMusicSets(c *gin.Context) { ... }   // Returns user-visible music sets
func (h *MediaHandler) GetImageDownloadURL(c *gin.Context) { ... }

// Admin endpoints
func (h *MediaHandler) UploadImage(c *gin.Context) { ... }
func (h *MediaHandler) UpdateImageVisibility(c *gin.Context) { ... }
func (h *MediaHandler) DeleteImage(c *gin.Context) { ... }
func (h *MediaHandler) CreateMusicSet(c *gin.Context) { ... }
func (h *MediaHandler) UpdateMusicSetVisibility(c *gin.Context) { ... }
func (h *MediaHandler) DeleteMusicSet(c *gin.Context) { ... }
```

### 2. Extend AdminHandler

**Option A:** Add methods directly to `AdminHandler`
- Simpler for now
- Keeps admin functionality centralized

**Option B:** Delegate to `MediaHandler` from `AdminHandler`
- Better separation
- Allows reuse of media logic

**Recommendation:** Option B for cleaner architecture. AdminHandler can wrap MediaHandler methods with admin middleware.

## Route Registration

**Location:** `cmd/api/main.go`

**Add after existing route groups:**

```go
// Initialize handlers
mediaHandler := handlers.NewMediaHandler(mediaService, authService)

// Public routes (no auth)
public := api.Group("/public")
{
    // ... existing public routes ...
    public.GET("/images", mediaHandler.GetPublicImages)
    public.GET("/images/:id", mediaHandler.GetPublicImage)
    public.GET("/music-sets", mediaHandler.GetPublicMusicSets)
    public.GET("/music-sets/:id", mediaHandler.GetPublicMusicSet)
}

// User routes (authenticated)
user := api.Group("/user")
user.Use(middleware.Auth(authService))
{
    // ... existing user routes ...
    user.GET("/images", mediaHandler.GetImages)
    user.GET("/music-sets", mediaHandler.GetMusicSets)
    user.GET("/images/:id/download", mediaHandler.GetImageDownloadURL)
    user.GET("/music-sets/:id/download", mediaHandler.GetMusicSetDownloadURL)
}

// Admin routes (admin only)
admin := api.Group("/admin")
admin.Use(middleware.Auth(authService))
admin.Use(middleware.AdminOnly())
{
    // ... existing admin routes ...
    admin.POST("/images/upload", mediaHandler.UploadImage)
    admin.PUT("/images/:id", mediaHandler.UpdateImage)
    admin.DELETE("/images/:id", mediaHandler.DeleteImage)
    admin.POST("/music-sets", mediaHandler.CreateMusicSet)
    admin.PUT("/music-sets/:id", mediaHandler.UpdateMusicSet)
    admin.DELETE("/music-sets/:id", mediaHandler.DeleteMusicSet)
}
```

## Migration Strategy

### Safe Deployment (Production Cannot Break)

**Phase 1: Database Schema (Zero Downtime)**
```bash
# 1. Create migration SQL file (run outside AutoMigrate for control)
ALTER TABLE images ADD COLUMN IF NOT EXISTS ...;
ALTER TABLE music_sets ADD COLUMN IF NOT EXISTS ...;
ALTER TABLE music_tracks ADD COLUMN IF NOT EXISTS ...;

# 2. Run migration manually in production
# 3. Verify schema changes
# 4. Deploy code with new models
```

**Phase 2: Feature Flag Control**
```go
// In config
MediaFeaturesEnabled bool `env:"MEDIA_FEATURES_ENABLED" default:"false"`

// In main.go - only register routes if enabled
if cfg.MediaFeaturesEnabled {
    public.GET("/images", mediaHandler.GetPublicImages)
    // ...
}
```

**Phase 3: Gradual Rollout**
1. Deploy with feature flag disabled
2. Enable in staging first
3. Enable in production with monitoring
4. Roll back by disabling flag if issues arise

### Build Order Implications

**Dependencies (must build in this order):**

1. **Models** (`internal/models/image.go`, `music_set.go`)
   - No dependencies, pure Go structs

2. **Repositories** (`internal/repositories/`)
   - Depend on: Models, GORM

3. **Services** (`internal/services/media_service.go`)
   - Depend on: Repositories, S3Service, AssetService

4. **Handlers** (`internal/handlers/media_handler.go`)
   - Depend on: Services, Middleware

5. **Main** (`cmd/api/main.go`)
   - Wire everything together

**Build verification:**
```bash
# Verify compilation after each layer
go build ./internal/models/...
go build ./internal/repositories/...
go build ./internal/services/...
go build ./internal/handlers/...
go build ./cmd/api
```

## Patterns to Follow

### Pattern 1: Service Composition

**What:** Services receive other services as dependencies, not raw GORM db

**When:** Your service needs functionality from another service

**Example:**
```go
// GOOD - Service composition
mediaService := services.NewMediaService(
    db,
    cfg,
    s3Service,      // Existing service
    assetService,   // Existing service
    imageRepo,
    musicSetRepo,
)

// AVOID - Direct db access bypassing services
type MediaService struct {
    db *gorm.DB  // OK for repositories
    s3Service *S3Service
    // Don't access other services' models directly
}
```

### Pattern 2: Handler Context Pattern

**What:** Extract user info from Gin context set by middleware

**When:** Handler needs authenticated user info

**Example:**
```go
func (h *MediaHandler) UploadImage(c *gin.Context) {
    userID, _ := c.Get("userID")        // Set by middleware.Auth
    isAdmin, _ := c.Get("isAdmin")      // Set by middleware.Auth
    user, _ := c.Get("user")            // Full user object

    // Use info
    if !(isAdmin.(bool)) {
        c.JSON(403, gin.H{"error": "admin only"})
        return
    }

    // Call service
    image, err := h.mediaService.UploadImage(c, userID.(uuid.UUID), req)
    // ...
}
```

### Pattern 3: Presigned URLs for Media

**What:** Generate time-limited S3 signed URLs instead of proxying

**When:** Serving private media to authenticated users

**Example:**
```go
func (s *MediaService) GetImageSignedURL(imageID uuid.UUID, userIsAdmin bool) (string, error) {
    // 1. Fetch image (check visibility)
    image, err := s.imageRepo.GetByID(imageID)
    if err != nil {
        return "", err
    }

    // 2. Check visibility
    if image.Visibility == models.ImageVisibilityPrivate && !userIsAdmin {
        return "", ErrUnauthorized
    }

    // 3. Generate presigned URL (15 min valid)
    url, err := s.s3Service.PresignMediaGet(
        context.Background(),
        s.cfg.MediaImagesBucket,
        image.Asset.Key,
        15*time.Minute,
    )
    return url, err
}
```

**Why:** Offloads bandwidth to S3, no server bottleneck, simpler code.

## Anti-Patterns to Avoid

### Anti-Pattern 1: GORM in Handlers

**What:** Direct `db.Find()` calls in handler functions

**Why bad:**
- Bypasses business logic layer
- Hard to test (need full DB)
- Duplicates logic across handlers

**Instead:** Always call service methods. Services own data access.

### Anti-Pattern 2: Tight Coupling to S3

**What:** Hardcoding S3 operations throughout handlers

**Why bad:**
- Can't swap storage backends
- Hard to test (need real S3)
- Business logic mixed with infrastructure

**Instead:** Use `S3Service` abstraction. Handlers only know about `MediaService`.

### Anti-Pattern 3: Blocking Migrations

**What:** `AutoMigrate` that adds columns to existing tables in production

**Why bad:**
- Can lock tables, cause downtime
- No rollback path
- Unpredictable order

**Instead:** Write manual SQL migrations for schema changes to existing tables. AutoMigrate only for new tables.

### Anti-Pattern 4: Admin Bypass

**What:** Checking `isAdmin` flag without using `AdminOnly()` middleware

**Why bad:**
- Easy to forget checks
- Inconsistent security
- Can't audit which endpoints are admin-only

**Instead:** Always use middleware for route-level protection.

## Scalability Considerations

| Concern | At 100 users | At 10K users | At 1M users |
|---------|--------------|--------------|-------------|
| Image uploads | Direct to S3, fine | Direct to S3, fine | Direct to S3, fine |
| Image serving | Presigned URLs | Presigned URLs + CDN | Presigned URLs + CDN + cache |
| Database queries | Simple index on visibility | Add index on event_id | Add composite index + cache |
| Concurrent uploads | No limits | Add rate limiting per user | Queue system, background processing |

**Key points:**
- S3/CloudFront scales horizontally - no bottleneck
- Database queries need proper indexing from day 1
- Presigned URLs are stateless - scales infinitely
- For 1M+ users, consider background job processing for uploads

## Integration Points with Existing Code

### 1. Auth Middleware (Reuse)

**Existing:** `middleware.Auth(authService)` and `middleware.AdminOnly()`

**Use exactly as-is:**
```go
user := api.Group("/user")
user.Use(middleware.Auth(authService))  // Reuse

admin := api.Group("/admin")
admin.Use(middleware.Auth(authService))
admin.Use(middleware.AdminOnly())       // Reuse
```

### 2. S3Service (Reuse)

**Existing methods:**
- `UploadMedia(ctx, bucket, key, body, ctype)`
- `PresignMediaGet(ctx, bucket, key, ttl)`
- `DownloadMedia(ctx, bucket, key)`
- `ListMediaKeys(ctx, bucket, prefix, max)`

**Use existing buckets:**
- `cfg.MediaImagesBucket` for images
- `cfg.MediaAudioBucket` for audio tracks

### 3. AssetService (Reuse)

**Existing:**
- `GetByID(id)` - fetch Asset by ID
- `CreateAssetRecord()` - create Asset records

**Integration:**
```go
// After S3 upload, create Asset record
asset, err := s.assetService.CreateAssetRecord(
    key, filename, size, checksum, isLocal,
)
// Then use asset.ID in Image or MusicTrack
```

### 4. Existing Asset Model (Reference)

**The new Image and MusicSet models reference existing Asset:**
```go
type Image struct {
    AssetID uuid.UUID `gorm:"type:uuid;not null"` // References Asset
    Asset   Asset     `gorm:"foreignKey:AssetID"`
    // ...
}
```

This leverages existing infrastructure while adding domain-specific metadata.

## Testing Strategy

### Unit Tests

**Level 1: Repository Tests**
```go
func TestImageRepository_Create(t *testing.T) {
    // Use SQLite in-memory
    db := setupTestDB()
    repo := repositories.NewImageRepository(db)
    // Test CRUD operations
}
```

**Level 2: Service Tests (mocked S3)**
```go
func TestMediaService_UploadImage(t *testing.T) {
    // Mock S3Service
    mockS3 := &MockS3Service{}
    service := services.NewMediaService(db, cfg, mockS3, ...)
    // Test upload logic
}
```

**Level 3: Handler Tests (mocked service)**
```go
func TestMediaHandler_UploadImage(t *testing.T) {
    // Mock MediaService
    mockService := &MockMediaService{}
    handler := NewMediaHandler(mockService, authService)
    // Test HTTP handling
}
```

### Integration Tests

**Test with real S3-compatible local stack:**
```bash
# Use MinIO for local S3 testing
docker run -p 9000:9000 minio/minio server /data
```

## Sources

- Existing codebase analysis (HIGH confidence)
  - `internal/models/asset.go` - Asset model with Visibility pattern
  - `internal/services/s3_service.go` - S3 operations
  - `internal/handlers/admin_handler.go` - Existing upload pattern
  - `cmd/api/main.go` - Dependency injection pattern
  - `internal/middleware/auth.go` - Auth patterns
