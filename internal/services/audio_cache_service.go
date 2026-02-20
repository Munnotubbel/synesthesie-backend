package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/synesthesie/backend/internal/config"
)

// AudioCacheService handles local caching of audio files
// with streaming support for large files
type AudioCacheService struct {
	cfg        *config.Config
	s3Service  *S3Service
	downloads  map[string]*sync.Mutex // Prevent duplicate downloads
	downloadsM sync.Mutex
}

func NewAudioCacheService(cfg *config.Config, s3Service *S3Service) *AudioCacheService {
	return &AudioCacheService{
		cfg:       cfg,
		s3Service: s3Service,
		downloads: make(map[string]*sync.Mutex),
	}
}

// GetLocalPath returns the local file path for an audio key.
// The cache base is AudioCachePath (e.g. /data/assets_cache/audio).
// The key format is "music/<filename>"; we store only the filename inside the cache dir.
func (s *AudioCacheService) GetLocalPath(key string) string {
	// Strip leading directory component ("music/") so that
	// key "music/uuid.flac" → cached at "<AudioCachePath>/uuid.flac"
	filename := key
	if idx := len("music/"); len(key) > idx && key[:idx] == "music/" {
		filename = key[idx:]
	}
	return filepath.Join(s.cfg.AudioCachePath, filename)
}

// IsCached returns true if the file exists locally
func (s *AudioCacheService) IsCached(key string) bool {
	localPath := s.GetLocalPath(key)
	_, err := os.Stat(localPath)
	return err == nil
}

// GetDownloadLock returns a mutex for a specific key to prevent duplicate downloads
func (s *AudioCacheService) GetDownloadLock(key string) *sync.Mutex {
	s.downloadsM.Lock()
	defer s.downloadsM.Unlock()

	if _, exists := s.downloads[key]; !exists {
		s.downloads[key] = &sync.Mutex{}
	}
	return s.downloads[key]
}

// StreamAudio streams audio from local cache or S3
// Returns an io.ReadCloser that the caller must close
func (s *AudioCacheService) StreamAudio(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	localPath := s.GetLocalPath(key)

	// Fast path: serve from local cache
	if info, err := os.Stat(localPath); err == nil {
		file, err := os.Open(localPath)
		if err == nil {
			return file, info.Size(), nil
		}
	}

	// Slow path: download from S3 while streaming to client
	// Use lock to prevent duplicate downloads of the same file
	lock := s.GetDownloadLock(key)
	lock.Lock()
	defer lock.Unlock()

	// Double-check after acquiring lock (another goroutine might have downloaded)
	if info, err := os.Stat(localPath); err == nil {
		file, err := os.Open(localPath)
		if err == nil {
			return file, info.Size(), nil
		}
	}

	// Download from S3
	return s.downloadAndCache(ctx, key, localPath)
}

// downloadAndCache downloads from S3 and saves to local cache
// Returns a reader for the downloaded content
func (s *AudioCacheService) downloadAndCache(ctx context.Context, key, localPath string) (io.ReadCloser, int64, error) {
	// Ensure directory exists
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, 0, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download from S3 to local file
	err := s.s3Service.DownloadMediaToFile(ctx, s.cfg.MediaAudioBucket, key, localPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to download from S3: %w", err)
	}

	// Now serve from local file
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat cached file: %w", err)
	}

	file, err := os.Open(localPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open cached file: %w", err)
	}

	return file, info.Size(), nil
}

// DownloadToCache downloads a file from S3 to local cache
// This is blocking - use for first-time caching before serving
func (s *AudioCacheService) DownloadToCache(ctx context.Context, key string) error {
	localPath := s.GetLocalPath(key)

	// Double-check if already cached
	if s.IsCached(key) {
		return nil
	}

	// Use lock to prevent duplicate downloads
	lock := s.GetDownloadLock(key)
	lock.Lock()
	defer lock.Unlock()

	// Double-check after acquiring lock
	if s.IsCached(key) {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download from S3 to local file
	return s.s3Service.DownloadMediaToFile(ctx, s.cfg.MediaAudioBucket, key, localPath)
}

// StreamFromS3 streams the audio directly from S3 without caching.
// Returns an io.ReadCloser — caller MUST close it.
// Use as fallback when local cache is unavailable.
func (s *AudioCacheService) StreamFromS3(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.s3Service.StreamMedia(ctx, s.cfg.MediaAudioBucket, key)
}

// StartBackgroundDownload downloads a file in the background for caching
func (s *AudioCacheService) StartBackgroundDownload(ctx context.Context, key string) {
	go func() {
		localPath := s.GetLocalPath(key)

		// Skip if already cached
		if _, err := os.Stat(localPath); err == nil {
			return
		}

		lock := s.GetDownloadLock(key)
		lock.Lock()
		defer lock.Unlock()

		// Double-check
		if _, err := os.Stat(localPath); err == nil {
			return
		}

		// Download to cache (discard the reader, we just want to cache)
		reader, _, err := s.StreamAudio(ctx, key)
		if err != nil {
			return
		}
		defer reader.Close()

		// Drain the reader to complete the download
		io.Copy(io.Discard, reader)
	}()
}
