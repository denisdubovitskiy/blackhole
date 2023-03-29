package cache

import (
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	t.Run("empty.proto cache", func(t *testing.T) {
		cache := NewMemoryCache()

		// act
		_, hasDomain := cache.Get(dns.TypeA, "test.domain.com")

		// assert
		require.False(t, hasDomain)
	})

	t.Run("populated cache", func(t *testing.T) {
		want := &dns.AAAA{
			Hdr: dns.RR_Header{
				Name: "test.domain.com",
				Ttl:  10,
			},
			AAAA: net.ParseIP("10.160.104.10"),
		}
		cache := NewMemoryCache()
		cache.Set(dns.TypeAAAA, "test.domain.com", want)

		// act
		got, ok := cache.Get(dns.TypeAAAA, "test.domain.com")

		// assert
		require.True(t, ok)
		require.Equal(t, want, got)
	})

	t.Run("populated cache (BlockTTL exceeded)", func(t *testing.T) {
		want := &dns.AAAA{
			Hdr: dns.RR_Header{
				Name: "test.domain.com",
				Ttl:  0,
			},
			AAAA: net.ParseIP("10.160.104.10"),
		}
		cache := NewMemoryCache()
		cache.Set(dns.TypeAAAA, "test.domain.com", want)

		// act
		_, ok := cache.Get(dns.TypeAAAA, "test.domain.com")

		// assert
		require.False(t, ok)
	})
}
