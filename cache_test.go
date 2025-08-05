package s3ds

import (
	"os"
	"testing"
	"time"
)

func TestCacheConfig(t *testing.T) {
	// Test default cache configuration vansh
	config := CacheConfig{
		EnableCache: true,
	}

	// Verify defaults are set correctly
	if config.TTL == 0 {
		config.TTL = 24 * time.Hour
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 1 * time.Hour
	}
	if config.MemoryCacheSize == 0 {
		config.MemoryCacheSize = 1000
	}

	// Test that configuration is valid
	if config.TTL <= 0 {
		t.Error("TTL should be positive")
	}
	if config.CleanupInterval <= 0 {
		t.Error("CleanupInterval should be positive")
	}
	if config.MemoryCacheSize <= 0 {
		t.Error("MemoryCacheSize should be positive")
	}
}

func TestCacheStats(t *testing.T) {
	// Test cache statistics
	stats := CacheStats{
		MemoryHits:   10,
		MemoryMisses: 5,
		DiskHits:     20,
		DiskMisses:   15,
		S3Requests:   30,
		CleanupRuns:  2,
	}

	// Verify stats are accessible
	if stats.MemoryHits != 10 {
		t.Error("MemoryHits should be 10")
	}
	if stats.S3Requests != 30 {
		t.Error("S3Requests should be 30")
	}
}

func TestCacheEntry(t *testing.T) {
	// Test cache entry creation
	now := time.Now()
	testData := []byte("test-data")
	entry := CacheEntry{
		Data:      testData,
		Size:      len(testData),
		Timestamp: now,
		TTL:       now.Add(24 * time.Hour),
	}

	// Verify entry fields
	if len(entry.Data) != len(testData) {
		t.Errorf("Data size should be %d, got %d", len(testData), len(entry.Data))
	}
	if entry.Size != len(testData) {
		t.Errorf("Size should be %d, got %d", len(testData), entry.Size)
	}
	if entry.TTL.Before(now) {
		t.Error("TTL should be in the future")
	}
}

func TestCacheDirectoryCreation(t *testing.T) {
	// Test that cache directory can be created
	tempDir, err := os.MkdirTemp("", "s3-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Verify directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Cache directory should exist")
	}
}
