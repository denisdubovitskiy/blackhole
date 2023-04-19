package blacklist

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thanhpk/randstr"
)

func TestBlacklist(t *testing.T) {
	const (
		bucketsCount = 256
		domainsCount = 300_000
	)

	domains := make([]string, domainsCount)
	for i := range domains {
		domains[i] = randstr.Hex(50)
	}

	bl := New(bucketsCount)

	wg := sync.WaitGroup{}
	wg.Add(len(domains))

	for _, domain := range domains {
		go func(d string) {
			defer wg.Done()
			bl.add(d)
		}(domain)
	}

	wg.Wait()

	wg2 := sync.WaitGroup{}
	wg2.Add(len(domains))

	for _, domain := range domains {
		go func(d string) {
			defer wg2.Done()
			require.True(t, bl.Has(context.Background(), d))
		}(domain)
	}

	wg2.Wait()

	require.Equal(t, 0, bl.Add(context.Background(), " "))
	require.Equal(t, 0, bl.Add(context.Background(), ""))
	require.False(t, bl.Has(context.Background(), " "))
	require.False(t, bl.Has(context.Background(), ""))
}
