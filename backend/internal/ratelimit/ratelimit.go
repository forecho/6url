package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
}

type Redis struct {
	client *redis.Client
	prefix string
}

type Memory struct {
	mu      sync.Mutex
	buckets map[string]memoryBucket
	now     func() time.Time
}

type memoryBucket struct {
	count   int
	expires time.Time
}

func NewMemory() *Memory {
	return &Memory{buckets: make(map[string]memoryBucket), now: time.Now}
}

func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 || window <= 0 {
		return false, errors.New("invalid rate limit")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	bucket := m.buckets[key]
	if !bucket.expires.After(now) {
		bucket = memoryBucket{expires: now.Add(window)}
	}
	bucket.count++
	m.buckets[key] = bucket
	return bucket.count <= limit, nil
}

func NewRedis(addr, password string, db int, prefix string) (*Redis, error) {
	if addr == "" || prefix == "" {
		return nil, errors.New("redis rate limiter configuration is incomplete")
	}
	return &Redis{client: redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db}), prefix: prefix}, nil
}

func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }
func (r *Redis) Close() error                   { return r.client.Close() }

func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 || window <= 0 {
		return false, errors.New("invalid rate limit")
	}
	bucket := time.Now().UTC().UnixNano() / window.Nanoseconds()
	redisKey := r.prefix + "rate:" + key + ":" + time.Unix(0, bucket*window.Nanoseconds()).UTC().Format(time.RFC3339Nano)
	pipe := r.client.TxPipeline()
	count := pipe.Incr(ctx, redisKey)
	pipe.Expire(ctx, redisKey, window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return count.Val() <= int64(limit), nil
}
