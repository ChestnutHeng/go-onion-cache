package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis"
)

type RedisCache struct {
	redis                  *redis.Client
	DefaultExpiredDuration time.Duration
}

func NewRedisCache(expiredTime time.Duration) *RedisCache {
	return &RedisCache{
		redis:                  initRedisClient(),
		DefaultExpiredDuration: expiredTime,
	}
}

func initRedisClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	return client
}

func (r *RedisCache) Get(ctx context.Context, key Key) (Value, bool) {
	result := r.redis.Get(key.String())
	if err := result.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			fmt.Printf("redis cache key=%v pipeline.get error: redis Nil", key)
		} else {
			fmt.Printf("redis cache key=%v single.get error: %v", key, err)
		}
		return nil, false
	}
	return result.Val(), true
}

func (r *RedisCache) Set(ctx context.Context, key Key, v Value) {
	result := r.redis.Set(key.String(), v, r.DefaultExpiredDuration)
	if err := result.Err(); err != nil {
		fmt.Printf("redis cache key=%+v, value=%+v single.set error: %v", key, v, err)
	}
}

func (r *RedisCache) MultiGet(ctx context.Context, keys []Key) map[Key]Value {
	values := make(map[Key]Value, len(keys))
	if len(keys) == 0 {
		return values
	}

	pipeline := r.redis.Pipeline()
	defer func() {
		if err := pipeline.Close(); err != nil {
			fmt.Printf("redis cache pipeline close error: %v", err)
		}
	}()

	results := make(map[Key]*redis.StringCmd, len(keys))
	for _, key := range keys {
		results[key] = pipeline.Get(key.String())
	}

	if _, err := pipeline.Exec(); err != nil {
		fmt.Printf("redis cache keys=%+v, MultiGet error: %v", keys, err)
		return values
	}

	for key, result := range results {
		if err := result.Err(); err != nil {
			if errors.Is(err, redis.Nil) {
				fmt.Printf("redis cache key=%v pipeline.get error: redis Nil", key)
			} else {
				fmt.Printf("redis cache key=%v pipeline.get error: %v", key, err)
			}
		} else {
			values[key] = result.Val()
		}
	}
	return values
}

func (r *RedisCache) MultiSet(ctx context.Context, kvs map[Key]Value) {
	if len(kvs) == 0 {
		return
	}
	pipeline := r.redis.Pipeline()
	defer func() {
		if err := pipeline.Close(); err != nil {
			fmt.Printf("redis cache pipeline close error: %v", err)
		}
	}()

	results := make(map[Key]*redis.StatusCmd, len(kvs))
	for k, v := range kvs {
		results[k] = pipeline.Set(k.String(), v, r.DefaultExpiredDuration)
	}

	if _, err := pipeline.Exec(); err != nil {
		fmt.Printf("redis cache kvs=%v, MultiSet error: %v", kvs, err)
	}

	for k, result := range results {
		if err := result.Err(); err != nil {
			fmt.Printf("redis cache key=%+v, value=%+v pipeline.set error: %v", k, kvs[k], err)
		}
	}
}
