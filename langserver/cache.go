package langserver

import "sync"

type cache interface {
	Get(key interface{}, fill func() interface{}) interface{}
	Purge()
}

func newTypecheckCache() *unboundedCache {
	c := &unboundedCache{}
	c.Purge()
	return c
}

func newSymbolCache() *unboundedCache {
	c := &unboundedCache{}
	c.Purge()
	return c
}

type cacheValue struct {
	ready chan struct{} // closed to broadcast readiness
	value interface{}
}

type unboundedCache struct {
	mu sync.Mutex
	c  map[interface{}]*cacheValue
}

func (c *unboundedCache) Get(k interface{}, fill func() interface{}) interface{} {
	c.mu.Lock()
	v, ok := c.c[k]
	if ok {
		// cache hit, wait until ready
		c.mu.Unlock()
		<-v.ready
	} else {
		// cache miss. Add unready result to cache and fill
		v = &cacheValue{ready: make(chan struct{})}
		c.c[k] = v
		c.mu.Unlock()

		defer close(v.ready)
		v.value = fill()
	}

	return v.value
}

func (c *unboundedCache) Purge() {
	c.mu.Lock()
	c.c = make(map[interface{}]*cacheValue)
	c.mu.Unlock()
}
