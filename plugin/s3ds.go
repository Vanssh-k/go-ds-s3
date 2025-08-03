package plugin

import (
	"fmt"
	"time"

	s3ds "github.com/Vanssh-k/go-ds-s3"
	"github.com/ipfs/kubo/plugin"
	"github.com/ipfs/kubo/repo"
	"github.com/ipfs/kubo/repo/fsrepo"
)

var Plugins = []plugin.Plugin{
	&S3Plugin{},
}

type S3Plugin struct{}

func (s3p S3Plugin) Name() string {
	return "s3-datastore-plugin"
}

func (s3p S3Plugin) Version() string {
	return "0.0.1"
}

func (s3p S3Plugin) Init(env *plugin.Environment) error {
	return nil
}

func (s3p S3Plugin) DatastoreTypeName() string {
	return "s3ds"
}

func (s3p S3Plugin) DatastoreConfigParser() fsrepo.ConfigFromMap {
	return func(m map[string]interface{}) (fsrepo.DatastoreConfig, error) {
		region, ok := m["region"].(string)
		if !ok {
			return nil, fmt.Errorf("s3ds: no region specified")
		}

		bucket, ok := m["bucket"].(string)
		if !ok {
			return nil, fmt.Errorf("s3ds: no bucket specified")
		}

		accessKey, ok := m["accessKey"].(string)
		if !ok {
			return nil, fmt.Errorf("s3ds: no accessKey specified")
		}

		secretKey, ok := m["secretKey"].(string)
		if !ok {
			return nil, fmt.Errorf("s3ds: no secretKey specified")
		}

		// Optional.

		var sessionToken string
		if v, ok := m["sessionToken"]; ok {
			sessionToken, ok = v.(string)
			if !ok {
				return nil, fmt.Errorf("s3ds: sessionToken not a string")
			}
		}

		var endpoint string
		if v, ok := m["regionEndpoint"]; ok {
			endpoint, ok = v.(string)
			if !ok {
				return nil, fmt.Errorf("s3ds: regionEndpoint not a string")
			}
		}
		var rootDirectory string
		if v, ok := m["rootDirectory"]; ok {
			rootDirectory, ok = v.(string)
			if !ok {
				return nil, fmt.Errorf("s3ds: rootDirectory not a string")
			}
		}
		var workers int
		if v, ok := m["workers"]; ok {
			workersf, ok := v.(float64)
			workers = int(workersf)
			switch {
			case !ok:
				return nil, fmt.Errorf("s3ds: workers not a number")
			case workers <= 0:
				return nil, fmt.Errorf("s3ds: workers <= 0: %f", workersf)
			case float64(workers) != workersf:
				return nil, fmt.Errorf("s3ds: workers is not an integer: %f", workersf)
			}
		}
		var credentialsEndpoint string
		if v, ok := m["credentialsEndpoint"]; ok {
			credentialsEndpoint, ok = v.(string)
			if !ok {
				return nil, fmt.Errorf("s3ds: credentialsEndpoint not a string")
			}
		}

		// Cache configuration
		var cacheConfig s3ds.CacheConfig
		if v, ok := m["enableCache"]; ok {
			if enableCache, ok := v.(bool); ok {
				cacheConfig.EnableCache = enableCache
			}
		}

		if cacheConfig.EnableCache {
			// Memory cache size (default 1000 items)
			if v, ok := m["memoryCacheSize"]; ok {
				if size, ok := v.(float64); ok {
					cacheConfig.MemoryCacheSize = int(size)
				}
			}
			if cacheConfig.MemoryCacheSize == 0 {
				cacheConfig.MemoryCacheSize = 1000
			}

			// Disk cache path (default: "./s3-cache")
			if v, ok := m["diskCachePath"]; ok {
				if path, ok := v.(string); ok {
					cacheConfig.DiskCachePath = path
				}
			}
			if cacheConfig.DiskCachePath == "" {
				cacheConfig.DiskCachePath = "./s3-cache"
			}

			// TTL in hours (default 24 hours)
			if v, ok := m["cacheTTL"]; ok {
				if ttlHours, ok := v.(float64); ok {
					cacheConfig.TTL = time.Duration(ttlHours) * time.Hour
				}
			}
			if cacheConfig.TTL == 0 {
				cacheConfig.TTL = 24 * time.Hour
			}

			// Cleanup interval in hours (default 1 hour)
			if v, ok := m["cleanupInterval"]; ok {
				if intervalHours, ok := v.(float64); ok {
					cacheConfig.CleanupInterval = time.Duration(intervalHours) * time.Hour
				}
			}
			if cacheConfig.CleanupInterval == 0 {
				cacheConfig.CleanupInterval = 1 * time.Hour
			}
		}

		return &S3Config{
			cfg: s3ds.Config{
				Region:              region,
				Bucket:              bucket,
				AccessKey:           accessKey,
				SecretKey:           secretKey,
				SessionToken:        sessionToken,
				RootDirectory:       rootDirectory,
				Workers:             workers,
				RegionEndpoint:      endpoint,
				CredentialsEndpoint: credentialsEndpoint,
			},
			cacheConfig: cacheConfig,
		}, nil
	}
}

type S3Config struct {
	cfg         s3ds.Config
	cacheConfig s3ds.CacheConfig
}

func (s3c *S3Config) DiskSpec() fsrepo.DiskSpec {
	return fsrepo.DiskSpec{
		"region":        s3c.cfg.Region,
		"bucket":        s3c.cfg.Bucket,
		"rootDirectory": s3c.cfg.RootDirectory,
		"accessKey":     s3c.cfg.AccessKey,
		"secretKey":     s3c.cfg.SecretKey,
	}
}

func (s3c *S3Config) Create(path string) (repo.Datastore, error) {
	if s3c.cacheConfig.EnableCache {
		// Use cached version
		return s3ds.NewCachedS3Datastore(s3c.cfg, s3c.cacheConfig)
	}
	// Use original version without cache
	return s3ds.NewS3Datastore(s3c.cfg)
}
