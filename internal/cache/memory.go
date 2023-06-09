package cache

import (
	"context"
	"expvar"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/miekg/dns"
)

type Cache interface {
	Get(reqType uint16, domain string) (dns.RR, bool)
	Set(reqType uint16, domain string, ip dns.RR)
}

type Item struct {
	Ip  dns.RR
	Die time.Time
}

type MemoryCache struct {
	cache    map[uint16]map[string]*Item
	mu       sync.RWMutex
	cancel   context.CancelFunc
	hits     *atomic.Int32
	misses   *atomic.Int32
	cleanups *atomic.Int32
}

func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{
		cache:  make(map[uint16]map[string]*Item),
		cancel: func() {},

		hits:     atomic.NewInt32(0),
		misses:   atomic.NewInt32(0),
		cleanups: atomic.NewInt32(0),
	}

	expvar.Publish("blackhole_cache", expvar.Func(func() any {
		return cache.dumpStats()
	}))

	return cache
}

func (c *MemoryCache) RunPeriodicCleaner(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.cleanPeriodically(ctx)
}

func (c *MemoryCache) Close() error {
	c.cancel()
	return nil
}

func (c *MemoryCache) Get(reqType uint16, domain string) (dns.RR, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if m, ok := c.cache[reqType]; ok {
		if ip, ok := m[domain]; ok {
			if ip.Die.After(time.Now()) {
				c.hits.Inc()
				return ip.Ip, true
			}
		}
	}

	c.misses.Inc()
	return nil, false
}

func (c *MemoryCache) Set(reqType uint16, domain string, ip dns.RR) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var m map[string]*Item

	m, ok := c.cache[reqType]
	if !ok {
		m = make(map[string]*Item)
		c.cache[reqType] = m
	}

	m[domain] = &Item{
		Ip:  ip,
		Die: time.Now().Add(time.Duration(ip.Header().Ttl) * time.Second),
	}
}

func (c *MemoryCache) cleanPeriodically(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanNow()
			c.cleanups.Inc()
		case <-ctx.Done():
			return
		}
	}
}

func (c *MemoryCache) cleanNow() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	for _, v := range c.cache {
		for k, vv := range v {
			if vv.Die.Before(now) {
				delete(v, k)
			}
		}
	}
}

type stats struct {
	Cached   int   `json:"count"`
	Hits     int32 `json:"hits"`
	Misses   int32 `json:"misses"`
	Cleanups int32 `json:"cleanups"`
}

func (c *MemoryCache) dumpStats() any {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0

	for _, v := range c.cache {
		count += len(v)
	}

	return stats{
		Cached:   count,
		Hits:     c.hits.Load(),
		Misses:   c.misses.Load(),
		Cleanups: c.cleanups.Load(),
	}
}
