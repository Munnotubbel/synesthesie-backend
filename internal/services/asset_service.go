package services

import (
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
	"github.com/synesthesie/backend/internal/models"
	"gorm.io/gorm"
)

type AssetService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAssetService(db *gorm.DB, cfg *config.Config) *AssetService {
	return &AssetService{db: db, cfg: cfg}
}

func (s *AssetService) GetByID(id uuid.UUID) (*models.Asset, error) {
	var asset models.Asset
	if err := s.db.First(&asset, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (s *AssetService) GetAbsolutePath(asset *models.Asset) string {
	return filepath.Join(s.cfg.LocalAssetsPath, filepath.FromSlash(asset.Key))
}

func (s *AssetService) String() string {
	return fmt.Sprintf("AssetService(local=%s)", s.cfg.LocalAssetsPath)
}
