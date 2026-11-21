// Package ratelimit implements an atomic fixed-window counter on Redis.
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var script = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then redis.call('PEXPIRE', KEYS[1], ARGV[1]); ttl = tonumber(ARGV[1]) end
return {c, ttl}
`)

type Limiter struct{ rdb redis.UniversalClient }

func New(rdb redis.UniversalClient) *Limiter { return &Limiter{rdb: rdb} }

type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// Allow counts one hit for key and reports whether it is within limit for the window.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	res, err := script.Run(ctx, l.rdb, []string{"rl:" + key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return Result{Allowed: true, Limit: limit, Remaining: limit}, err
	}
	count, ttl := int(res[0]), time.Duration(res[1])*time.Millisecond
	r := Result{Allowed: count <= limit, Limit: limit, Remaining: max(0, limit-count)}
	if !r.Allowed {
		r.RetryAfter = ttl
	}
	return r, nil
}
