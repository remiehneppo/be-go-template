package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultLockPrefix = "lock:"

type RedisConfig struct {
	Addr       string
	Password   string
	DB         int
	LockPrefix string
}

type RedisCache struct {
	client     *redis.Client
	lockPrefix string
}

func NewRedis(cfg RedisConfig) *RedisCache {
	lockPrefix := cfg.LockPrefix
	if lockPrefix == "" {
		lockPrefix = defaultLockPrefix
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return NewRedisWithClient(client, lockPrefix)
}

func NewRedisWithClient(client *redis.Client, lockPrefix string) *RedisCache {
	if lockPrefix == "" {
		lockPrefix = defaultLockPrefix
	}
	return &RedisCache{
		client:     client,
		lockPrefix: lockPrefix,
	}
}

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

func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cache value: %w", err)
	}
	return c.client.Set(ctx, key, payload, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

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
		_ = c.releaseLock(context.Background(), lockKey, owner)
	}()

	return fn(ctx)
}

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
