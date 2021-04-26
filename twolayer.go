package cache

import (
	"context"
	"errors"
	"time"

	"github.com/golang/groupcache/singleflight"

	"github.com/chestnutheng/go-onion-cache/util"
)

type Key interface {
	String() string
}

type Value interface {
}

// Cache 缓存
type Cache interface {
	MultiGet(context.Context, []Key) map[Key]Value
	MultiSet(context.Context, map[Key]Value)
}

type (
	LoadFunc func(context.Context, []Key) map[Key]Value
)

// LayeredCache 双层缓存
type LayeredCache struct {
	localCache *LocalCache
	redisCache *RedisCache
	dbLoader   LoadFunc
	sg         singleflight.Group
}

/*NewLayeredCache 双层缓存
  loader: 击穿db的读方法
  memCap: lfu的大小
  memThreshold: lfu热点百分比（小于百分比的key不会缓存） 全量缓存填1
  memExpire: lfu过期时间 0永不过期
  redisExpire: redis过期时间
*/
func NewLayeredCache(loader LoadFunc, memCap int64,
	memExpire time.Duration, redisExpire time.Duration) *LayeredCache {
	return &LayeredCache{
		redisCache: NewRedisCache(redisExpire),
		localCache: NewLocalCache(memCap, memExpire),
		dbLoader:   loader,
	}
}

/*MultiGetValuesAndMisses 读数据，记下miss的值
  cache: 需要读的缓存
  key : 所有的key
  values : 存放value的map
*/
func MultiGetValuesAndMisses(ctx context.Context, cache Cache, keys []Key, values map[Key]Value) ([]Key, map[Key]Value) {
	vMap := cache.MultiGet(ctx, keys)
	misses := make([]Key, 0)
	for _, k := range keys {
		if v, ok := vMap[k]; ok {
			values[k] = util.MarshalToValue(v)
		} else {
			misses = append(misses, k)
		}
	}
	return misses, vMap
}

// MultiGet 依次读缓存
func (l *LayeredCache) MultiGet(ctx context.Context, keys []Key) map[Key]Value {
	values := make(map[Key]Value, len(keys))
	if len(keys) == 0 {
		return values
	}
	// Local Cache
	misses, missMap := MultiGetValuesAndMisses(ctx, l.localCache, keys, values)
	if len(misses) == 0 {
		return values
	}

	// 击穿到redis
	misses, missMap = MultiGetValuesAndMisses(ctx, l.redisCache, misses, values)
	if len(missMap) > 0 {
		l.localCache.MultiSet(ctx, missMap)
	}
	if len(misses) == 0 {
		return values
	}

	// 击穿到db
	missMap = l.dbLoader(ctx, misses)
	for _, k := range misses {
		if v, ok := missMap[k]; ok {
			values[k] = v
		}
	}
	if len(missMap) > 0 {
		l.localCache.MultiSet(ctx, missMap)
		l.redisCache.MultiSet(ctx, missMap)
	}
	return values
}

// MultiSet 依次写缓存
func (l *LayeredCache) MultiSet(ctx context.Context, kvs map[Key]Value) {
	if len(kvs) == 0 {
		return
	}
	l.localCache.MultiSet(ctx, kvs)
	l.redisCache.MultiSet(ctx, kvs)
}

func (l *LayeredCache) Get(ctx context.Context, key Key) Value {
	// Local Cache
	res, ok := l.localCache.Get(ctx, key)
	if ok {
		return res
	}

	// 击穿到redis
	res, ok = l.redisCache.Get(ctx, key)
	if ok {
		l.localCache.Set(ctx, key, res)
		return res
	}

	// 击穿到db
	keys := []Key{key}
	resMap := l.dbLoader(ctx, keys)
	if res, ok = resMap[key]; ok {
		l.localCache.Set(ctx, key, res)
		l.redisCache.Set(ctx, key, res)
		return res
	}
	return nil
}

func (l *LayeredCache) Set(ctx context.Context, key Key, value Value) {
	l.localCache.Set(ctx, key, value)
	l.redisCache.Set(ctx, key, value)
}

// GetWithSF get with singleflight
func (l *LayeredCache) GetWithSF(ctx context.Context, key Key) Value {
	// Local Cache
	res, ok := l.localCache.Get(ctx, key)
	if ok {
		return res
	}

	// 击穿到redis
	resIft, err := l.sg.Do(key.String(), func() (ret interface{}, err error) {
		res, ok := l.redisCache.Get(ctx, key)
		if ok {
			l.localCache.Set(ctx, key, res)
			return res, nil
		}

		// 击穿到db
		keys := []Key{key}
		resMap := l.dbLoader(ctx, keys)
		if res, ok = resMap[key]; ok {
			l.localCache.Set(ctx, key, res)
			l.redisCache.Set(ctx, key, res)
			return res, nil
		}
		return res, errors.New("cache get missed")
	})
	if err != nil {
		return nil
	}
	return resIft.(Value)
}
