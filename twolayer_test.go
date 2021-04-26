package cache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

var myDB = map[Key]Value{
	NewCacheKey("video", 1):  `{"video_title":"me1"}`,
	NewCacheKey("video", 12): `{"video_title":"me12"}`,
	NewCacheKey("pic", 2):    `{"pic_title":"me22"}`,
	NewCacheKey("pic", 22):   `{"pic_title":"me22"}`,
}

type CacheKey struct {
	cacheType string
	id        int64
}

func (p CacheKey) String() string {
	return fmt.Sprintf("%s_%d", p.cacheType, p.id)
}

func NewCacheKey(t string, id int64) CacheKey {
	return CacheKey{t, id}
}

func GetIdsFromCacheKey(keys []Key) []int64 {
	ids := make([]int64, 0, len(keys))
	for i := range keys {
		ids = append(ids, keys[i].(CacheKey).id)
	}
	return ids
}

func GetItemFromDB(ctx context.Context, keys []Key) map[Key]Value {
	// value 必须是string
	infos := map[Key]Value{}
	for _, k := range keys {
		if v, ok := myDB[k]; ok {
			infos[k] = v
		}
	}
	return infos
}

func TestSingleCache(t *testing.T) {
	var loader LoadFunc
	loader = GetItemFromDB
	cache := NewLayeredCache(loader, 100000, time.Second*5, time.Second*5)

	key := CacheKey{"video", 1}

	fmt.Printf("1st:\n")
	mp := cache.Get(context.Background(), key)
	fmt.Printf("%+v\n", mp)

	fmt.Printf("2rd:\n")
	mp = cache.Get(context.Background(), key)
	fmt.Printf("%+v\n", mp)
}
