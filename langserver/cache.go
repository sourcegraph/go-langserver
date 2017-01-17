package langserver

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
)

type cache interface {
	Get(key interface{}, fill func() interface{}) interface{}
	Purge()
}

func newTypecheckCache() *boundedCache {
	c, err := lru.NewARC(1000)
	if err != nil {
		// This should never happen since our size is always > 0
		panic(err)
	}
	return &boundedCache{
		c: c,
	}
}

func newSymbolCache() *boundedCache {
	c, err := lru.NewARC(1000)
	if err != nil {
		// This should never happen since our size is always > 0
		panic(err)
	}
	return &boundedCache{
		c: c,
	}
}

type cacheValue struct {
	ready chan struct{} // closed to broadcast readiness
	value interface{}
}

type boundedCache struct {
	mu sync.Mutex
	c  *lru.ARCCache
}

func (c *boundedCache) Get(k interface{}, fill func() interface{}) interface{} {
	c.mu.Lock()
	var v *cacheValue
	if vi, ok := c.c.Get(k); ok {
		// cache hit, wait until ready
		c.mu.Unlock()
		v = vi.(*cacheValue)
		<-v.ready
	} else {
		// cache miss. Add unready result to cache and fill
		v = &cacheValue{ready: make(chan struct{})}
		c.c.Add(k, v)
		c.mu.Unlock()

		defer close(v.ready)
		v.value = fill()
	}

	return v.value
}

func (c *boundedCache) Purge() {
	c.c.Purge()
}
