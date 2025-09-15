package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synesthesie/backend/internal/config"
)

// StorageService stores assets locally and can provide signed download URLs via the API
type StorageService struct {
	cfg *config.Config
}

func NewStorageService(cfg *config.Config) *StorageService {
	// ensure local path exists
	_ = os.MkdirAll(cfg.LocalAssetsPath, 0o755)
	return &StorageService{cfg: cfg}
}

// BuildObjectKey creates a namespaced storage key
func (s *StorageService) BuildObjectKey(kind string, originalName string) string {
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext == "" {
		// try to guess from mime
		if mt := mime.TypeByExtension(ext); mt != "" {
			_ = mt
		}
	}
	return fmt.Sprintf("%s/%s%s", kind, uuid.New().String(), ext)
}

// SaveStream saves an incoming stream to local storage and returns absolute path, size and checksum
func (s *StorageService) SaveStream(ctx context.Context, key string, r io.Reader) (string, int64, string, error) {
	absPath := filepath.Join(s.cfg.LocalAssetsPath, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", 0, "", err
	}

	tmp := absPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", 0, "", err
	}
	defer f.Close()

	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hasher), r)
	if err != nil {
		_ = os.Remove(tmp)
		return "", 0, "", err
	}

	if err := f.Sync(); err != nil {
		_ = os.Remove(tmp)
		return "", 0, "", err
	}

	if err := os.Rename(tmp, absPath); err != nil {
		_ = os.Remove(tmp)
		return "", 0, "", err
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	return absPath, n, checksum, nil
}

// ServeFileWithRange serves a local file with HTTP range support
func (s *StorageService) ServeFileWithRange(w http.ResponseWriter, req *http.Request, absPath, downloadName string) error {
	// set headers
	if downloadName != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
	}
	http.ServeFile(w, req, absPath)
	return nil
}

// BuildExpiringToken is a simple HMAC-less token (for now): we use UUID v4 and short TTL stored in memory in future iteration
// For current approach, downloads are proxied via API and require auth, so no token is needed.

// PresignGetURL: Not used for local FS; downloads go through API which checks auth and streams file.
func (s *StorageService) PresignGetURL(key string, ttl time.Duration) (string, time.Time, error) {
	return "", time.Now().Add(ttl), nil
}
