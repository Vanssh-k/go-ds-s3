# S3 Datastore with Hybrid Caching

This plugin now supports hybrid caching to significantly improve performance by reducing S3 API calls.

## How It Works

The hybrid cache implements a two-level caching system:

1. **Memory Cache (Level 1)**: Fast in-memory LRU cache for frequently accessed data
2. **Disk Cache (Level 2)**: Persistent LevelDB cache for larger datasets
3. **S3 Backend (Level 3)**: Original S3 storage

## Cache Flow

```
Request → Memory Cache → Disk Cache → S3
   ↑           ↓              ↓
Response ← Cache Hit ← Cache Hit
```

## Configuration

### Basic Configuration (No Cache)

```json
{
  "Datastore": {
    "Spec": {
      "mounts": [
        {
          "child": {
            "type": "s3ds",
            "region": "us-east-1",
            "bucket": "my-ipfs-bucket",
            "rootDirectory": "blocks",
            "accessKey": "your-access-key",
            "secretKey": "your-secret-key"
          },
          "mountpoint": "/blocks",
          "prefix": "s3.datastore",
          "type": "measure"
        }
      ]
    }
  }
}
```

### With Hybrid Caching Enabled

```json
{
  "Datastore": {
    "Spec": {
      "mounts": [
        {
          "child": {
            "type": "s3ds",
            "region": "us-east-1",
            "bucket": "my-ipfs-bucket",
            "rootDirectory": "blocks",
            "accessKey": "your-access-key",
            "secretKey": "your-secret-key",
            "enableCache": true,
            "memoryCacheSize": 2000,
            "diskCachePath": "/var/lib/ipfs/s3-cache",
            "cacheTTL": 24,
            "cleanupInterval": 1
          },
          "mountpoint": "/blocks",
          "prefix": "s3.datastore",
          "type": "measure"
        }
      ]
    }
  }
}
```

## Cache Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enableCache` | bool | false | Enable hybrid caching |
| `memoryCacheSize` | int | 1000 | Number of items in memory cache |
| `diskCachePath` | string | "./s3-cache" | Path for LevelDB disk cache |
| `cacheTTL` | float64 | 24 | Time to live in hours (24 = 24 hours) |
| `cleanupInterval` | float64 | 1 | Cleanup interval in hours |

## Performance Benefits

### Before Caching
- Every `Get` operation = 1 S3 API call
- Every `Has` operation = 1 S3 HeadObject API call
- High latency for repeated requests
- High S3 costs

### After Caching
- First request = 1 S3 API call + cache storage
- Subsequent requests = 0 S3 API calls (cache hit)
- 10-100x faster for cached data
- Significant cost reduction

## Cache Statistics

You can monitor cache performance through the plugin:

```go
// Get cache statistics
stats := cachedS3.GetCacheStats()
fmt.Printf("Memory hits: %d\n", stats.MemoryHits)
fmt.Printf("Memory misses: %d\n", stats.MemoryMisses)
fmt.Printf("Disk hits: %d\n", stats.DiskHits)
fmt.Printf("Disk misses: %d\n", stats.DiskMisses)
fmt.Printf("S3 requests: %d\n", stats.S3Requests)
fmt.Printf("Cleanup runs: %d\n", stats.CleanupRuns)
```

## Automatic Cleanup

The cache automatically cleans up expired entries:

- **TTL**: 24 hours by default
- **Cleanup Interval**: Every hour by default
- **Memory Cache**: LRU automatically manages size
- **Disk Cache**: Expired entries removed during cleanup

## Manual Cache Management

```go
// Force immediate cleanup
cachedS3.ForceCacheCleanup()

// Clear all cached data
cachedS3.ClearCache()

// Get cache statistics
stats := cachedS3.GetCacheStats()
```

## Best Practices

### 1. Memory Cache Size
- **Small workloads**: 500-1000 items
- **Medium workloads**: 1000-5000 items
- **Large workloads**: 5000-10000 items

### 2. Disk Cache Path
- Use dedicated directory: `/var/lib/ipfs/s3-cache`
- Ensure sufficient disk space
- Consider SSD for better performance

### 3. TTL Configuration
- **Frequently changing data**: 1-6 hours
- **Stable data**: 24-48 hours
- **Static data**: 72+ hours

### 4. Cleanup Interval
- **High write frequency**: 30 minutes
- **Normal usage**: 1 hour
- **Low write frequency**: 2-4 hours

## Example Use Cases

### 1. Development Environment
```json
{
  "enableCache": true,
  "memoryCacheSize": 500,
  "diskCachePath": "./dev-cache",
  "cacheTTL": 6,
  "cleanupInterval": 0.5
}
```

### 2. Production Environment
```json
{
  "enableCache": true,
  "memoryCacheSize": 5000,
  "diskCachePath": "/var/lib/ipfs/s3-cache",
  "cacheTTL": 24,
  "cleanupInterval": 1
}
```

### 3. High-Performance Environment
```json
{
  "enableCache": true,
  "memoryCacheSize": 10000,
  "diskCachePath": "/mnt/ssd/ipfs-cache",
  "cacheTTL": 48,
  "cleanupInterval": 2
}
```

## Monitoring

Monitor cache effectiveness:

```bash
# Check cache directory size
du -sh /var/lib/ipfs/s3-cache

# Monitor cache statistics
ipfs stats repo

# Check S3 API usage (via AWS CloudWatch)
aws cloudwatch get-metric-statistics \
  --namespace AWS/S3 \
  --metric-name NumberOfRequests \
  --dimensions Name=BucketName,Value=my-ipfs-bucket
```

## Troubleshooting

### Cache Not Working
1. Check if `enableCache` is set to `true`
2. Verify disk cache path is writable
3. Check available disk space
4. Review cache statistics

### High Memory Usage
1. Reduce `memoryCacheSize`
2. Increase `cleanupInterval`
3. Monitor cache hit rates

### Slow Performance
1. Increase `memoryCacheSize`
2. Use SSD for disk cache
3. Check cache hit rates
4. Verify TTL settings

## Migration

To enable caching on existing installations:

1. Stop IPFS daemon
2. Update configuration with cache settings
3. Start IPFS daemon
4. Monitor cache performance

The cache will populate automatically as data is accessed. 