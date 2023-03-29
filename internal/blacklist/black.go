package blacklist

import (
	"strings"
	"sync"

	"github.com/cespare/xxhash"
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

type BlackList struct {
	buckets []*Bucket
}

func New(bucketsCount int) *BlackList {
	buckets := make([]*Bucket, bucketsCount)
	for i := 0; i < bucketsCount; i++ {
		buckets[i] = NewBucket()
	}
	return &BlackList{buckets: buckets}
}

func (b *BlackList) calcBucketIndex(domain string) int {
	hash := xxhash.Sum64String(domain)
	bucket := hash % uint64(len(b.buckets))
	return int(bucket)
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

func (b *BlackList) Add(domains ...string) (count int) {
	for _, domain := range domains {
		if b.add(domain) {
			count++
		}
	}

	return
}

func (b *BlackList) Remove(domains ...string) (count int) {
	for _, domain := range domains {
		if b.remove(domain) {
			count++
		}
	}

	return
}

func (b *BlackList) Has(domain string) bool {
	domain = clean(domain)
	idx := b.calcBucketIndex(domain)
	return b.buckets[idx].Has(domain)
}
