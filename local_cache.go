package cache

import (
	"context"
	"time"

	"github.com/chestnutheng/go-onion-cache/middleware/lfu"
)

type LocalCache struct {
	cache                  *lfu.Cache
	DefaultExpiredDuration time.Duration
}

func NewLocalCache(cap int64, defaultExpiredDuration time.Duration) *LocalCache {
	return &LocalCache{
		cache:                  lfu.NewLFUCache(cap, time.Minute),
		DefaultExpiredDuration: time.Minute,
	}
}

func (l *LocalCache) Get(ctx context.Context, k Key) (Value, bool) {
	v := l.cache.Get(k)
	if v == nil {
		return nil, false
	}
	return v, true
}

func (l *LocalCache) Set(ctx context.Context, k Key, v Value) {
	expiration := time.Now().Add(l.DefaultExpiredDuration)
	l.cache.Set(k, v, &expiration)
}

func (l *LocalCache) MultiGet(ctx context.Context, keys []Key) map[Key]Value {
	values := make(map[Key]Value, len(keys))
	if len(keys) == 0 {
		return values
	}
	keyList := make([]interface{}, len(keys))
	for i, key := range keys {
		keyList[i] = key
	}
	valueMap := l.cache.MGet(keyList)
	for _, k := range keyList {
		key := k.(Key)
		if v := valueMap[k]; v != nil {
			if value, ok := v.(Value); ok {
				values[key] = value
			}
		}
	}
	return values
}

func (l *LocalCache) MultiSet(ctx context.Context, kvs map[Key]Value) {
	if len(kvs) == 0 {
		return
	}
	kvMap := make(map[interface{}]interface{}, len(kvs))
	for k, v := range kvs {
		if k != nil && v != nil {
			kvMap[k] = v
		}
	}
	if l.DefaultExpiredDuration > 0 {
		expiration := time.Now().Add(l.DefaultExpiredDuration)
		l.cache.MSet(kvMap, &expiration)
	} else {
		l.cache.MSet(kvMap, nil)
	}
}
