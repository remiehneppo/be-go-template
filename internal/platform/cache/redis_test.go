package cache

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type cachedUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func TestRedisCacheSetGetJSON(t *testing.T) {
	cache, cleanup := newTestCache(t)
	defer cleanup()

	ctx := context.Background()
	want := cachedUser{ID: "u1", Email: "user@example.com"}

	if err := cache.Set(ctx, "user:u1", want, time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	var got cachedUser
	if err := cache.Get(ctx, "user:u1", &got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Fatalf("Get() = %+v, want %+v", got, want)
	}
}

func TestRedisCacheMiss(t *testing.T) {
	cache, cleanup := newTestCache(t)
	defer cleanup()

	var got cachedUser
	err := cache.Get(context.Background(), "missing", &got)
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("Get() error = %v, want ErrCacheMiss", err)
	}
}

func TestRedisCacheDeleteAndExists(t *testing.T) {
	cache, cleanup := newTestCache(t)
	defer cleanup()

	ctx := context.Background()
	if err := cache.Set(ctx, "k", map[string]string{"v": "1"}, time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	exists, err := cache.Exists(ctx, "k")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false")
	}
	if err := cache.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	exists, err = cache.Exists(ctx, "k")
	if err != nil {
		t.Fatalf("Exists() after delete error = %v", err)
	}
	if exists {
		t.Fatal("Exists() after delete = true")
	}
}

func TestRedisCacheIncrement(t *testing.T) {
	cache, cleanup := newTestCache(t)
	defer cleanup()

	ctx := context.Background()
	first, err := cache.Increment(ctx, "rate:k", time.Minute)
	if err != nil {
		t.Fatalf("Increment() first error = %v", err)
	}
	second, err := cache.Increment(ctx, "rate:k", time.Minute)
	if err != nil {
		t.Fatalf("Increment() second error = %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("increments = %d, %d", first, second)
	}
	ttl, err := cache.client.TTL(ctx, "rate:k").Result()
	if err != nil {
		t.Fatalf("TTL() error = %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("TTL() = %s", ttl)
	}
}

func TestRedisCacheTLSConfigUsesServerName(t *testing.T) {
	cache, err := NewRedis(RedisConfig{Addr: "redis.example.com:6380", TLSEnabled: true})
	if err != nil {
		t.Fatalf("NewRedis() error = %v", err)
	}
	defer cache.Close()

	if cache.client.Options().TLSConfig == nil {
		t.Fatal("TLSConfig = nil")
	}
	if got := cache.client.Options().TLSConfig.ServerName; got != "redis.example.com" {
		t.Fatalf("ServerName = %q", got)
	}
}

func TestRedisCacheTLSConfigLoadsCACert(t *testing.T) {
	dir := t.TempDir()
	caPath := dir + "/redis-ca.pem"
	if err := os.WriteFile(caPath, mustGenerateCACertPEM(t), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cache, err := NewRedis(RedisConfig{Addr: "redis.example.com:6380", TLSEnabled: true, TLSCACert: caPath})
	if err != nil {
		t.Fatalf("NewRedis() error = %v", err)
	}
	defer cache.Close()

	cfg := cache.client.Options().TLSConfig
	if cfg == nil {
		t.Fatal("TLSConfig = nil")
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs = nil")
	}
	if got := cfg.ServerName; got != "redis.example.com" {
		t.Fatalf("ServerName = %q", got)
	}
}

func TestRedisCacheTLSConfigRejectsInvalidCACert(t *testing.T) {
	dir := t.TempDir()
	caPath := dir + "/redis-ca.pem"
	if err := os.WriteFile(caPath, []byte("not pem"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := NewRedis(RedisConfig{Addr: "redis.example.com:6380", TLSEnabled: true, TLSCACert: caPath})
	if err == nil {
		t.Fatal("NewRedis() error = nil")
	}
}

func mustGenerateCACertPEM(t *testing.T) []byte {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "redis-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestRedisCacheWithLockExcludesConcurrentOwner(t *testing.T) {
	cache, cleanup := newTestCache(t)
	defer cleanup()

	ctx := context.Background()
	err := cache.WithLock(ctx, "critical", time.Minute, func(ctx context.Context) error {
		err := cache.WithLock(ctx, "critical", time.Minute, func(ctx context.Context) error {
			return nil
		})
		if !errors.Is(err, ErrLockNotAcquired) {
			t.Fatalf("nested WithLock() error = %v, want ErrLockNotAcquired", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithLock() error = %v", err)
	}
}

func newTestCache(t *testing.T) (*RedisCache, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	cache := NewRedisWithClient(client, "test-lock:")
	return cache, func() {
		_ = cache.Close()
		server.Close()
	}
}
