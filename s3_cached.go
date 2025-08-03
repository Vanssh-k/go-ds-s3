package s3ds

import (
	"context"

	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
)

// CachedS3Bucket wraps S3Bucket with hybrid caching
type CachedS3Bucket struct {
	*S3Bucket
	cache *HybridCache
}

// NewCachedS3Datastore creates a new S3 datastore with hybrid caching
func NewCachedS3Datastore(conf Config, cacheConfig CacheConfig) (*CachedS3Bucket, error) {
	// Create the base S3 datastore
	s3Bucket, err := NewS3Datastore(conf)
	if err != nil {
		return nil, err
	}

	// Create the hybrid cache
	cache, err := NewHybridCache(s3Bucket, cacheConfig)
	if err != nil {
		return nil, err
	}

	return &CachedS3Bucket{
		S3Bucket: s3Bucket,
		cache:    cache,
	}, nil
}

// Get retrieves data with caching
func (c *CachedS3Bucket) Get(ctx context.Context, k ds.Key) ([]byte, error) {
	return c.cache.Get(ctx, k)
}

// Has checks if key exists with caching
func (c *CachedS3Bucket) Has(ctx context.Context, k ds.Key) (exists bool, err error) {
	// Try to get the data from cache first
	if _, err := c.cache.Get(ctx, k); err == nil {
		return true, nil
	} else if err == ds.ErrNotFound {
		return false, nil
	} else {
		return false, err
	}
}

// GetSize gets the size of data with caching
func (c *CachedS3Bucket) GetSize(ctx context.Context, k ds.Key) (size int, err error) {
	// Try to get the data from cache first
	if data, err := c.cache.Get(ctx, k); err == nil {
		return len(data), nil
	} else if err == ds.ErrNotFound {
		return -1, ds.ErrNotFound
	} else {
		return -1, err
	}
}

// Put stores data and invalidates cache
func (c *CachedS3Bucket) Put(ctx context.Context, k ds.Key, value []byte) error {
	// Store in S3
	err := c.S3Bucket.Put(ctx, k, value)
	if err != nil {
		return err
	}

	// Invalidate cache for this key
	c.cache.InvalidateCache(k)
	return nil
}

// Delete removes data and invalidates cache
func (c *CachedS3Bucket) Delete(ctx context.Context, k ds.Key) error {
	// Delete from S3
	err := c.S3Bucket.Delete(ctx, k)
	if err != nil {
		return err
	}

	// Invalidate cache for this key
	c.cache.InvalidateCache(k)
	return nil
}

// Sync syncs data and invalidates cache
func (c *CachedS3Bucket) Sync(ctx context.Context, prefix ds.Key) error {
	// Sync to S3
	err := c.S3Bucket.Sync(ctx, prefix)
	if err != nil {
		return err
	}

	// Clear cache for all keys with this prefix
	c.clearCacheForPrefix(prefix)
	return nil
}

// Query queries the datastore with caching considerations
func (c *CachedS3Bucket) Query(ctx context.Context, q dsq.Query) (dsq.Results, error) {
	// For queries, we need to go directly to S3 since we can't cache all possible query results
	// But we can still use cache for individual gets within the query
	return c.S3Bucket.Query(ctx, q)
}

// Batch creates a batch with cache invalidation
func (c *CachedS3Bucket) Batch(ctx context.Context) (ds.Batch, error) {
	batch, err := c.S3Bucket.Batch(ctx)
	if err != nil {
		return nil, err
	}

	return &cachedBatch{
		Batch: batch,
		cache: c.cache,
	}, nil
}

// Close closes the datastore and cache
func (c *CachedS3Bucket) Close() error {
	// Close cache first
	if c.cache != nil {
		c.cache.Close()
	}

	// Then close S3 bucket
	return c.S3Bucket.Close()
}

// GetCacheStats returns cache performance statistics
func (c *CachedS3Bucket) GetCacheStats() CacheStats {
	if c.cache != nil {
		return c.cache.GetStats()
	}
	return CacheStats{}
}

// ForceCacheCleanup forces an immediate cache cleanup
func (c *CachedS3Bucket) ForceCacheCleanup() {
	if c.cache != nil {
		c.cache.ForceCleanup()
	}
}

// ClearCache clears all cached data
func (c *CachedS3Bucket) ClearCache() error {
	if c.cache != nil {
		return c.cache.ClearCache()
	}
	return nil
}

// InvalidateCache removes an item from both caches
func (c *CachedS3Bucket) InvalidateCache(key ds.Key) {
	if c.cache != nil {
		c.cache.InvalidateCache(key)
	}
}

// clearCacheForPrefix removes all cached items with the given prefix
func (c *CachedS3Bucket) clearCacheForPrefix(prefix ds.Key) {
	if c.cache == nil {
		return
	}

	// This is a simplified implementation
	// In a real implementation, you might want to iterate through the cache
	// and remove all keys that match the prefix
	// For now, we'll just clear the entire cache when syncing
	c.cache.ClearCache()
}

// cachedBatch wraps the original batch with cache invalidation
type cachedBatch struct {
	ds.Batch
	cache *HybridCache
}

func (cb *cachedBatch) Put(ctx context.Context, k ds.Key, val []byte) error {
	err := cb.Batch.Put(ctx, k, val)
	if err != nil {
		return err
	}

	// Invalidate cache for this key
	cb.cache.InvalidateCache(k)
	return nil
}

func (cb *cachedBatch) Delete(ctx context.Context, k ds.Key) error {
	err := cb.Batch.Delete(ctx, k)
	if err != nil {
		return err
	}

	// Invalidate cache for this key
	cb.cache.InvalidateCache(k)
	return nil
}

func (cb *cachedBatch) Commit(ctx context.Context) error {
	return cb.Batch.Commit(ctx)
}
