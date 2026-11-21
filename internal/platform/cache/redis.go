// Package cache connects to Redis.
package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cholobangladesh-server/internal/config"

	"github.com/redis/go-redis/v9"
)

func Connect(cfg config.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     50,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	slog.Info("redis connected")
	return rdb, nil
}
