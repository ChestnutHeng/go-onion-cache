package lfu

import (
	"container/list"
	"runtime"
	"sync"
	"time"
)

type Eviction struct {
	Key   interface{}
	Value interface{}
}

type Cache struct {
	// If len > maxLen*2, cache will automatically evict
	// down to maxLen.  If either value is 0, this behavior
	// is disabled.
	maxLen int64
	values map[interface{}]*cacheEntry
	freqs  *list.List
	len    int64
	lock   *sync.Mutex
	// 如果这个chan被初始化，逐出的缓存会推入这里
	EvictionChannel chan<- Eviction
	// 清扫器
	cleaner    *cacheCleaner
	cleanCycle time.Duration
}

type cacheEntry struct {
	key      interface{}
	value    interface{}
	freqNode *list.Element
	expireAt *time.Time
	visitAt  time.Time
}

type listEntry struct {
	entries map[*cacheEntry]byte
	freq    int
}

func newCache(maxLen int64, cleanCycle time.Duration) *Cache {
	c := new(Cache)
	c.values = make(map[interface{}]*cacheEntry)
	c.freqs = list.New()
	c.lock = new(sync.Mutex)
	c.cleanCycle = cleanCycle
	c.maxLen = maxLen
	return c
}

func (c *Cache) delNoLock(entry *cacheEntry) {
	// No lock here so it can be called from within the lock (during Get)
	delete(c.values, entry.key)
	c.remEntry(entry.freqNode, entry)
	c.len--
}

func (c *Cache) Get(key interface{}) interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		if e.expireAt != nil && e.expireAt.Before(time.Now()) {
			c.delNoLock(e)
			return nil

		}
		c.increment(e)
		return e.value
	}
	return nil
}

func (c *Cache) Set(key interface{}, value interface{}, expireAt *time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		// value already exists for key.  overwrite
		e.value = value
		e.expireAt = expireAt
		c.increment(e)
	} else {
		// value doesn't exist.  insert
		e := new(cacheEntry)
		e.key = key
		e.value = value
		e.expireAt = expireAt
		c.values[key] = e
		c.increment(e)
		c.len++
		// bounds mgmt
		if c.maxLen > 0 {
			if c.len > c.maxLen*2 {
				c.evictNoLock(c.len - c.maxLen)
			}
		}
	}
}

func (c *Cache) Del(key interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if entry, ok := c.values[key]; ok {
		c.delNoLock(entry)
	}
}

func (c *Cache) Len() int64 {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.len
}

// 逐出count数量的元素
func (c *Cache) Evict(count int64) int64 {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.evictNoLock(count)
}

func (c *Cache) evictNoLock(count int64) int64 {
	// No lock here so it can be called from within the lock (during Set)
	var evicted int64
	for i := int64(0); i < count; {
		if place := c.freqs.Front(); place != nil {
			for entry, _ := range place.Value.(*listEntry).entries {
				if i < count {
					if c.EvictionChannel != nil {
						c.EvictionChannel <- Eviction{
							Key:   entry.key,
							Value: entry.value,
						}
					}
					c.delNoLock(entry)
					evicted++
					i++
				}
			}
		}
	}
	return evicted
}

func (c *Cache) increment(e *cacheEntry) {
	currentPlace := e.freqNode
	var nextFreq int
	var nextPlace *list.Element
	if currentPlace == nil {
		// new entry
		nextFreq = 1
		nextPlace = c.freqs.Front()
	} else {
		// move up
		nextFreq = currentPlace.Value.(*listEntry).freq + 1
		nextPlace = currentPlace.Next()
	}

	if nextPlace == nil || nextPlace.Value.(*listEntry).freq != nextFreq {
		// create a new list entry
		li := new(listEntry)
		li.freq = nextFreq
		li.entries = make(map[*cacheEntry]byte)
		if currentPlace != nil {
			nextPlace = c.freqs.InsertAfter(li, currentPlace)
		} else {
			nextPlace = c.freqs.PushFront(li)
		}
	}
	e.freqNode = nextPlace
	e.visitAt = time.Now()
	nextPlace.Value.(*listEntry).entries[e] = 1
	if currentPlace != nil {
		// remove from current position
		c.remEntry(currentPlace, e)
	}
}

func (c *Cache) remEntry(place *list.Element, entry *cacheEntry) {
	entries := place.Value.(*listEntry).entries
	delete(entries, entry)
	if len(entries) == 0 {
		c.freqs.Remove(place)
	}
}

func (c *Cache) ScanAndClean() {
	c.lock.Lock()
	defer c.lock.Unlock()
	lookBackTime := time.Now().Add(-c.cleanCycle)
	for _, e := range c.values {
		if e.expireAt != nil && e.expireAt.Before(time.Now()) ||
			e.visitAt.Before(lookBackTime) {
			c.delNoLock(e)
		}
	}
}

func (c *Cache) MGet(keys []interface{}) map[interface{}]interface{} {
	res := make(map[interface{}]interface{})
	for _, k := range keys {
		res[k] = c.Get(k)
	}
	return res
}

func (c *Cache) MSet(kvs map[interface{}]interface{}, expiresAt *time.Time) {
	for k, v := range kvs {
		c.Set(k, v, expiresAt)
	}
}

type cacheCleaner struct {
	cleanCycle time.Duration
	stop       chan bool
}

func (c *cacheCleaner) Run(cache *Cache) {
	ticker := time.NewTicker(cache.cleanCycle)
	for {
		select {
		case <-ticker.C:
			cache.ScanAndClean()
		case <-c.stop:
			ticker.Stop()
			return
		}
	}
}

func NewLFUCache(maxLen int64, cleanCycle time.Duration) *Cache {
	cache := newCache(maxLen, cleanCycle)
	if cleanCycle > 0 {
		cache.cleaner = &cacheCleaner{
			cleanCycle: cleanCycle,
			stop:       make(chan bool),
		}
		go cache.cleaner.Run(cache)
		runtime.SetFinalizer(cache, func(c *Cache) {
			c.cleaner.stop <- true
		})
	}
	return cache
}
