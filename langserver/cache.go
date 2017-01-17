package langserver

import (
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru"
)

var (
	// typecheckCache is a process level cache for storing typechecked
	// values. Do not directly use this, instead use newTypecheckCache()
	typecheckCache = newARC(1000)

	// symbolCache is a process level cache for storing symbols found. Do
	// not directly use this, instead use newSymbolCache()
	symbolCache = newARC(1000)

	// cacheID is used to prevent key conflicts between different
	// LangHandlers in the same process.
	cacheID int64
)

type cache interface {
	Get(key interface{}, fill func() interface{}) interface{}
	Purge()
}

func newTypecheckCache() *boundedCache {
	return &boundedCache{
		id: nextCacheID(),
		c:  typecheckCache,
	}
}

func newSymbolCache() *boundedCache {
	return &boundedCache{
		id: nextCacheID(),
		c:  symbolCache,
	}
}

type cacheKey struct {
	id int64
	k  interface{}
}

type cacheValue struct {
	ready chan struct{} // closed to broadcast readiness
	value interface{}
}

type boundedCache struct {
	mu sync.Mutex
	id int64
	c  *lru.ARCCache
}

func (c *boundedCache) Get(k interface{}, fill func() interface{}) interface{} {
	c.mu.Lock()
	key := cacheKey{c.id, k}
	var v *cacheValue
	if vi, ok := c.c.Get(key); ok {
		// cache hit, wait until ready
		c.mu.Unlock()
		v = vi.(*cacheValue)
		<-v.ready
	} else {
		// cache miss. Add unready result to cache and fill
		v = &cacheValue{ready: make(chan struct{})}
		c.c.Add(key, v)
		c.mu.Unlock()

		defer close(v.ready)
		v.value = fill()
	}

	return v.value
}

func (c *boundedCache) Purge() {
	// c.c is a process level cache. Since c.id is part of the cache keys,
	// we can just change its values to make it seem like we have purged
	// the cache.
	c.mu.Lock()
	c.id = nextCacheID()
	c.mu.Unlock()
}

// newARC is a wrapper around lru.NewARC which does not return an error.
func newARC(size int) *lru.ARCCache {
	c, err := lru.NewARC(1000)
	if err != nil {
		// This should never happen since our size is always > 0
		panic(err)
	}
	return c
}

func nextCacheID() int64 {
	return atomic.AddInt64(&cacheID, 1)
}
