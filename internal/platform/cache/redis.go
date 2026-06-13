package cache

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultLockPrefix = "lock:"

// RedisConfig holds configuration for connecting to a Redis server.
type RedisConfig struct {
	// Addr is the host:port of the Redis server.
	Addr string
	// Password for the Redis server. Leave empty for no authentication.
	Password string
	// DB is the Redis database number. Defaults to 0.
	DB int
	// LockPrefix is the prefix used for distributed lock keys.
	LockPrefix string
	// TLSEnabled toggles TLS connectivity to Redis.
	TLSEnabled bool
	// TLSCACert points to a PEM CA bundle file when TLS is enabled.
	TLSCACert string
	// TLSServerName overrides the TLS server name for certificate validation.
	TLSServerName string
}

// RedisCache implements the Cache interface using the go-redis client.
type RedisCache struct {
	client     *redis.Client
	lockPrefix string
}

// NewRedis creates a RedisCache from configuration. It dials the Redis server
// and returns an error if the connection cannot be established.
func NewRedis(cfg RedisConfig) (*RedisCache, error) {
	lockPrefix := cfg.LockPrefix
	if lockPrefix == "" {
		lockPrefix = defaultLockPrefix
	}
	options := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLSEnabled {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		serverName := cfg.TLSServerName
		if serverName == "" {
			host, _, err := net.SplitHostPort(cfg.Addr)
			if err == nil {
				serverName = host
			}
		}
		tlsConfig.ServerName = serverName
		if cfg.TLSCACert != "" {
			caCert, err := os.ReadFile(cfg.TLSCACert)
			if err != nil {
				return nil, fmt.Errorf("read redis tls ca cert: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("parse redis tls ca cert: invalid pem")
			}
			tlsConfig.RootCAs = pool
		}
		options.TLSConfig = tlsConfig
	}
	client := redis.NewClient(options)
	return NewRedisWithClient(client, lockPrefix), nil
}

// NewRedisWithClient creates a RedisCache from an existing *redis.Client.
func NewRedisWithClient(client *redis.Client, lockPrefix string) *RedisCache {
	if lockPrefix == "" {
		lockPrefix = defaultLockPrefix
	}
	return &RedisCache{
		client:     client,
		lockPrefix: lockPrefix,
	}
}

// Get retrieves the value stored at key and decodes it into dest.
func (c *RedisCache) Get(ctx context.Context, key string, dest any) error {
	value, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrCacheMiss
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(value, dest); err != nil {
		return fmt.Errorf("decode cache value: %w", err)
	}
	return nil
}

// Set stores value at key with the given TTL. Value is marshalled as JSON.
func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cache value: %w", err)
	}
	return c.client.Set(ctx, key, payload, ttl).Err()
}

// Delete removes one or more keys from the cache.
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Exists reports whether key exists in the cache.
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Increment atomically increments the integer value at key, setting TTL
// on first creation. It returns the new counter value.
func (c *RedisCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		return 0, fmt.Errorf("increment ttl must be positive")
	}
	const script = `
local count = redis.call("INCR", KEYS[1])
if count == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return count
`
	return c.client.Eval(ctx, script, []string{key}, ttl.Milliseconds()).Int64()
}

// WithLock acquires a distributed lock for key, runs fn, then releases the
// lock. Returns ErrLockNotAcquired when the lock is held by another owner.
func (c *RedisCache) WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	if ttl <= 0 {
		return fmt.Errorf("lock ttl must be positive")
	}
	owner, err := randomOwner()
	if err != nil {
		return err
	}
	lockKey := c.lockPrefix + key
	acquired, err := c.client.SetNX(ctx, lockKey, owner, ttl).Result()
	if err != nil {
		return err
	}
	if !acquired {
		return ErrLockNotAcquired
	}
	defer func() {
		ignoreError(c.releaseLock(context.Background(), lockKey, owner))
	}()

	return fn(ctx)
}

// Ping checks connectivity to the cache backend.
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close releases the underlying Redis connection pool.
func (c *RedisCache) Close() error {
	return c.client.Close()
}

func (c *RedisCache) releaseLock(ctx context.Context, key, owner string) error {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	return c.client.Eval(ctx, script, []string{key}, owner).Err()
}

func randomOwner() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
