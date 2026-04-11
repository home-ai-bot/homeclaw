// Package data provides data access layer for HomeClaw.
// All JSON files use backup mechanism to prevent data loss on power failure.
package data

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileCache provides a simple file-based cache with TTL support.
// Files are stored with configurable naming (plain or base64-encoded).
// TTL of -1 means no expiration (cache forever).
type FileCache struct {
	cacheDir string
	ttl      time.Duration
	encode   bool // Whether to base64-encode keys as filenames
	mu       sync.RWMutex
}

// FileCacheConfig holds configuration for FileCache
type FileCacheConfig struct {
	// CacheDir is the directory to store cache files
	CacheDir string
	// TTL is the time-to-live for cache entries
	// Use -1 for no expiration
	TTL time.Duration
	// EncodeKey determines whether to base64-encode keys as filenames
	// Set to true if keys contain special characters (e.g., ":", "/")
	EncodeKey bool
}

// NewFileCache creates a new FileCache instance.
// The cache directory will be created if it doesn't exist.
func NewFileCache(config FileCacheConfig) (*FileCache, error) {
	if config.CacheDir == "" {
		return nil, fmt.Errorf("cache directory is required")
	}

	if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &FileCache{
		cacheDir: config.CacheDir,
		ttl:      config.TTL,
		encode:   config.EncodeKey,
	}, nil
}

// Get retrieves a cached value by key.
// Returns ErrCacheMiss if the key doesn't exist or has expired.
func (c *FileCache) Get(key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	path := c.filePath(key)

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to stat cache file: %w", err)
	}

	// Check TTL (skip if TTL is -1)
	if c.ttl >= 0 && time.Since(info.ModTime()) > c.ttl {
		return nil, ErrCacheMiss
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	return data, nil
}

// GetAsString retrieves a cached value as string.
// Returns ErrCacheMiss if the key doesn't exist or has expired.
func (c *FileCache) GetAsString(key string) (string, error) {
	data, err := c.Get(key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Set stores a value in the cache.
func (c *FileCache) Set(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := c.filePath(key)

	// Write file atomically (write to temp, then rename)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// SetString stores a string value in the cache.
func (c *FileCache) SetString(key string, data string) error {
	return c.Set(key, []byte(data))
}

// Delete removes a cached value by key.
func (c *FileCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := c.filePath(key)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete cache file: %w", err)
	}
	return nil
}

// Exists checks if a key exists in the cache and is not expired.
func (c *FileCache) Exists(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	path := c.filePath(key)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check TTL (skip if TTL is -1)
	if c.ttl >= 0 && time.Since(info.ModTime()) > c.ttl {
		return false
	}

	return true
}

// Cleanup removes all expired cache entries.
// If TTL is -1, this is a no-op.
func (c *FileCache) Cleanup() error {
	if c.ttl < 0 {
		return nil // TTL is -1, never expire
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	now := time.Now()
	var removed int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip temp files
		if filepath.Ext(entry.Name()) == ".tmp" {
			continue
		}

		path := filepath.Join(c.cacheDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Remove if expired
		if now.Sub(info.ModTime()) > c.ttl {
			if err := os.Remove(path); err != nil {
				// Log but continue with other files
				continue
			}
			removed++
		}
	}

	return nil
}

// Clear removes all cache entries (including non-expired).
func (c *FileCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(c.cacheDir, entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			// Log but continue
			continue
		}
	}

	return nil
}

// Count returns the number of cache entries (excluding expired if TTL >= 0).
func (c *FileCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return 0
	}

	count := 0
	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip temp files
		if filepath.Ext(entry.Name()) == ".tmp" {
			continue
		}

		// Count non-expired entries (or all if TTL is -1)
		if c.ttl < 0 {
			count++
		} else {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) <= c.ttl {
				count++
			}
		}
	}

	return count
}

// GetCacheDir returns the cache directory path.
func (c *FileCache) GetCacheDir() string {
	return c.cacheDir
}

// GetTTL returns the TTL duration.
func (c *FileCache) GetTTL() time.Duration {
	return c.ttl
}

// filePath returns the file path for a cache key.
func (c *FileCache) filePath(key string) string {
	filename := key
	if c.encode {
		filename = base64.URLEncoding.EncodeToString([]byte(key))
	}
	return filepath.Join(c.cacheDir, filename)
}

// ErrCacheMiss is returned when a cache key doesn't exist or has expired.
var ErrCacheMiss = fmt.Errorf("cache miss")
