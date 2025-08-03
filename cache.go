package s3ds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	levelds "github.com/ipfs/go-ds-leveldb"
)

// CacheConfig holds configuration for the hybrid cache
type CacheConfig struct {
	MemoryCacheSize int           // Number of items in memory cache
	DiskCachePath   string        // Path for disk cache database
	TTL             time.Duration // Time to live for cached items (default 24h)
	MaxDiskSize     int64         // Maximum disk cache size in bytes
	EnableCache     bool          // Whether to enable caching
	CleanupInterval time.Duration // How often to run cleanup (default 1h)
}

// CacheEntry represents a cached item with metadata
type CacheEntry struct {
	Data      []byte    `json:"data"`
	Size      int       `json:"size"`
	Timestamp time.Time `json:"timestamp"`
	TTL       time.Time `json:"ttl"`
}

// HybridCache implements a two-level cache system with LevelDB
type HybridCache struct {
	mu          sync.RWMutex
	memoryCache *lru.Cache
	diskCache   ds.Datastore
	s3Backend   *S3Bucket
	config      CacheConfig
	stats       CacheStats
	stopCleanup chan struct{}
}

// CacheStats holds cache performance statistics
type CacheStats struct {
	MemoryHits   int64
	MemoryMisses int64
	DiskHits     int64
	DiskMisses   int64
	S3Requests   int64
	CleanupRuns  int64
}

// NewHybridCache creates a new hybrid cache instance
func NewHybridCache(s3Backend *S3Bucket, config CacheConfig) (*HybridCache, error) {
	if !config.EnableCache {
		return &HybridCache{
			s3Backend: s3Backend,
			config:    config,
		}, nil
	}

	// Set default TTL to 24 hours if not specified
	if config.TTL == 0 {
		config.TTL = 24 * time.Hour
	}

	// Set default cleanup interval to 1 hour if not specified
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 1 * time.Hour
	}

	// Initialize memory cache
	memoryCache, err := lru.New(config.MemoryCacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory cache: %w", err)
	}

	// Initialize disk cache with LevelDB
	diskCache, err := levelds.NewDatastore(config.DiskCachePath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk cache: %w", err)
	}

	hc := &HybridCache{
		memoryCache: memoryCache,
		diskCache:   diskCache,
		s3Backend:   s3Backend,
		config:      config,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go hc.startCleanupRoutine()

	return hc, nil
}

// Get retrieves data using the hybrid cache hierarchy
func (hc *HybridCache) Get(ctx context.Context, key ds.Key) ([]byte, error) {
	if !hc.config.EnableCache {
		return hc.s3Backend.Get(ctx, key)
	}

	keyStr := key.String()

	// Level 1: Check memory cache
	hc.mu.RLock()
	if data, found := hc.memoryCache.Get(keyStr); found {
		hc.mu.RUnlock()
		hc.stats.MemoryHits++
		return data.([]byte), nil
	}
	hc.mu.RUnlock()
	hc.stats.MemoryMisses++

	// Level 2: Check disk cache
	if data, err := hc.getFromDisk(keyStr); err == nil {
		// Store in memory cache for next time
		hc.mu.Lock()
		hc.memoryCache.Add(keyStr, data)
		hc.mu.Unlock()
		hc.stats.DiskHits++
		return data, nil
	}
	hc.stats.DiskMisses++

	// Level 3: Fetch from S3
	data, err := hc.s3Backend.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	hc.stats.S3Requests++

	// Store in both caches
	hc.storeInCaches(keyStr, data)

	return data, nil
}

// getFromDisk retrieves data from the disk cache
func (hc *HybridCache) getFromDisk(key string) ([]byte, error) {
	entryBytes, err := hc.diskCache.Get(context.Background(), ds.NewKey(key))
	if err != nil {
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(entryBytes, &entry); err != nil {
		return nil, err
	}

	// Check if entry has expired
	if !entry.TTL.IsZero() && time.Now().After(entry.TTL) {
		// Remove expired entry
		hc.diskCache.Delete(context.Background(), ds.NewKey(key))
		return nil, ds.ErrNotFound
	}

	return entry.Data, nil
}

// storeInCaches stores data in both memory and disk caches
func (hc *HybridCache) storeInCaches(key string, data []byte) {
	entry := CacheEntry{
		Data:      data,
		Size:      len(data),
		Timestamp: time.Now(),
		TTL:       time.Now().Add(hc.config.TTL),
	}

	// Store in memory cache
	hc.mu.Lock()
	hc.memoryCache.Add(key, data)
	hc.mu.Unlock()

	// Store in disk cache
	hc.storeToDisk(key, entry)
}

// storeToDisk stores data in the disk cache
func (hc *HybridCache) storeToDisk(key string, entry CacheEntry) {
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return // Log error but don't fail the operation
	}

	hc.diskCache.Put(context.Background(), ds.NewKey(key), entryBytes)
}

// startCleanupRoutine starts the periodic cleanup goroutine
func (hc *HybridCache) startCleanupRoutine() {
	ticker := time.NewTicker(hc.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.cleanupExpiredEntries()
		case <-hc.stopCleanup:
			return
		}
	}
}

// cleanupExpiredEntries removes expired entries from both caches
func (hc *HybridCache) cleanupExpiredEntries() {
	if !hc.config.EnableCache {
		return
	}

	hc.stats.CleanupRuns++
	now := time.Now()

	// Clean up disk cache
	query, err := hc.diskCache.Query(context.Background(), dsq.Query{})
	if err != nil {
		return
	}
	defer query.Close()

	for result := range query.Next() {
		if result.Error != nil {
			continue
		}

		var entry CacheEntry
		if err := json.Unmarshal(result.Value, &entry); err != nil {
			continue
		}

		// Remove expired entries
		if !entry.TTL.IsZero() && now.After(entry.TTL) {
			hc.diskCache.Delete(context.Background(), ds.NewKey(result.Key))
		}
	}

	// Memory cache cleanup is handled by LRU automatically
}

// InvalidateCache removes an item from both caches
func (hc *HybridCache) InvalidateCache(key ds.Key) {
	if !hc.config.EnableCache {
		return
	}

	keyStr := key.String()

	// Remove from memory cache
	hc.mu.Lock()
	hc.memoryCache.Remove(keyStr)
	hc.mu.Unlock()

	// Remove from disk cache
	hc.diskCache.Delete(context.Background(), ds.NewKey(keyStr))
}

// GetStats returns cache statistics
func (hc *HybridCache) GetStats() CacheStats {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.stats
}

// Close closes the cache and releases resources
func (hc *HybridCache) Close() error {
	if !hc.config.EnableCache {
		return nil
	}

	// Stop cleanup routine
	close(hc.stopCleanup)

	// Close disk cache
	if hc.diskCache != nil {
		return hc.diskCache.Close()
	}
	return nil
}

// ClearCache clears both memory and disk caches
func (hc *HybridCache) ClearCache() error {
	if !hc.config.EnableCache {
		return nil
	}

	// Clear memory cache
	hc.mu.Lock()
	hc.memoryCache.Purge()
	hc.mu.Unlock()

	// Clear disk cache by deleting the directory and recreating
	if hc.diskCache != nil {
		hc.diskCache.Close()

		// Remove the LevelDB directory
		if err := os.RemoveAll(hc.config.DiskCachePath); err != nil {
			return fmt.Errorf("failed to remove disk cache directory: %w", err)
		}

		// Recreate the disk cache
		diskCache, err := levelds.NewDatastore(hc.config.DiskCachePath, nil)
		if err != nil {
			return fmt.Errorf("failed to recreate disk cache: %w", err)
		}
		hc.diskCache = diskCache
	}

	return nil
}

// ForceCleanup forces an immediate cleanup of expired entries
func (hc *HybridCache) ForceCleanup() {
	hc.cleanupExpiredEntries()
}
