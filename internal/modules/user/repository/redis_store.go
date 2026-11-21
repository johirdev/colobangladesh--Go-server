package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"

	"github.com/redis/go-redis/v9"
)

const prefix = "cb:user:"

type redisStore struct{ rdb redis.UniversalClient }

func NewRedisStore(rdb redis.UniversalClient) domain.Store { return &redisStore{rdb: rdb} }

func k(key string) string { return prefix + key }

func (s *redisStore) SaveOTP(ctx context.Context, key, hash string, ttl time.Duration) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, k(key))
	pipe.HSet(ctx, k(key), "h", hash, "a", 0)
	pipe.PExpire(ctx, k(key), ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) GetOTP(ctx context.Context, key string) (*domain.OTPRecord, error) {
	m, err := s.rdb.HGetAll(ctx, k(key)).Result()
	if err != nil {
		return nil, err
	}
	if len(m) == 0 || m["h"] == "" {
		return nil, domain.ErrNotFound
	}
	a, _ := strconv.Atoi(m["a"])
	return &domain.OTPRecord{Hash: m["h"], Attempts: a}, nil
}

func (s *redisStore) IncrOTPAttempts(ctx context.Context, key string) (int, error) {
	n, err := s.rdb.HIncrBy(ctx, k(key), "a", 1).Result()
	return int(n), err
}

var incrScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 or redis.call('PTTL', KEYS[1]) < 0 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return c
`)

func (s *redisStore) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	return incrScript.Run(ctx, s.rdb, []string{k(key)}, window.Milliseconds()).Int64()
}

func (s *redisStore) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	err := s.rdb.SetArgs(ctx, k(key), "1", redis.SetArgs{Mode: "NX", TTL: ttl}).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	return err == nil, err
}

func (s *redisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.rdb.Set(ctx, k(key), value, ttl).Err()
}

func (s *redisStore) Get(ctx context.Context, key string) (string, error) {
	v, err := s.rdb.Get(ctx, k(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", domain.ErrNotFound
	}
	return v, err
}

func (s *redisStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := s.rdb.PTTL(ctx, k(key)).Result()
	if err != nil || d < 0 {
		return 0, err
	}
	return d, nil
}

func (s *redisStore) Del(ctx context.Context, keys ...string) (int64, error) {
	full := make([]string, len(keys))
	for i, key := range keys {
		full[i] = k(key)
	}
	return s.rdb.Del(ctx, full...).Result()
}
