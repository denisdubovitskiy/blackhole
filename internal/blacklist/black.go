package blacklist

import (
	"context"
	"expvar"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/atomic"
)

func NewBucket() *Bucket {
	return &Bucket{data: make(map[string]struct{})}
}

type Bucket struct {
	mu   sync.RWMutex
	data map[string]struct{}
}

func (c *Bucket) Has(domain string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	domain = clean(domain)
	if domain == "" {
		return false
	}

	_, ok := c.data[domain]
	return ok
}

func (c *Bucket) Add(domain string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[domain] = struct{}{}
}

func (c *Bucket) Remove(domain string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, domain)
}

func (c *Bucket) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.data)
}

type BlackList struct {
	buckets      []*Bucket
	domainsCount *atomic.Int32
	hits         *atomic.Int32
	misses       *atomic.Int32
}

func New(bucketsCount int) *BlackList {
	buckets := make([]*Bucket, bucketsCount)
	for i := 0; i < bucketsCount; i++ {
		buckets[i] = NewBucket()
	}
	b := &BlackList{
		buckets:      buckets,
		domainsCount: atomic.NewInt32(0),
		hits:         atomic.NewInt32(0),
		misses:       atomic.NewInt32(0),
	}
	expvar.Publish("blackhole_blacklist", expvar.Func(func() any {
		return b.dumpStats()
	}))
	return b
}

type stats struct {
	Domains int32    `json:"domains"`
	Hits    int32    `json:"hits"`
	Misses  int32    `json:"misses"`
	Buckets []string `json:"buckets"`
}

func (b *BlackList) dumpStats() stats {
	buckets := make([]string, len(b.buckets))

	for i, bucket := range b.buckets {
		buckets[i] = formatBucket(i, len(buckets), bucket.Len())
	}

	return stats{
		Domains: b.domainsCount.Load(),
		Hits:    b.hits.Load(),
		Misses:  b.misses.Load(),
		Buckets: buckets,
	}
}

func formatBucket(index, count, bucketLength int) string {
	strIdx := strconv.Itoa(index)
	strCount := strconv.Itoa(count)

	if len(strIdx) < len(strCount) {
		strIdx = strings.Repeat("0", len(strCount)-len(strIdx)) + strIdx
	}
	return strIdx + ":" + strconv.Itoa(bucketLength)
}

func (b *BlackList) calcBucketIndex(domain string) int {
	var sum int
	for _, r := range domain {
		sum += int(r)
	}
	return sum % len(b.buckets)
}

func (b *BlackList) remove(domain string) bool {
	domain = clean(domain)
	if domain == "" {
		return false
	}
	idx := b.calcBucketIndex(domain)
	b.buckets[idx].Remove(domain)
	return true
}

func (b *BlackList) add(domain string) bool {
	domain = clean(domain)
	if domain == "" {
		return false
	}
	idx := b.calcBucketIndex(domain)
	b.buckets[idx].Add(domain)
	return true
}

func clean(domain string) string {
	domain = strings.TrimSpace(domain)
	if len(domain) == 0 {
		return ""
	}

	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}

	return domain
}

func (b *BlackList) Add(ctx context.Context, domains ...string) (count int) {
	for _, domain := range domains {
		if b.add(domain) {
			count++
			b.domainsCount.Inc()
		}
	}

	return
}

func (b *BlackList) Remove(ctx context.Context, domains ...string) (count int) {
	for _, domain := range domains {
		if b.remove(domain) {
			count++
			b.domainsCount.Dec()
		}
	}

	return
}

func (b *BlackList) Has(ctx context.Context, domain string) bool {
	domain = clean(domain)
	idx := b.calcBucketIndex(domain)
	has := b.buckets[idx].Has(domain)
	if has {
		b.hits.Inc()
	} else {
		b.misses.Inc()
	}
	return has
}
