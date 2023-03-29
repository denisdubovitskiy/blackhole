package blacklist

import (
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

	bl := New(bucketsCount, domains)

	wg := sync.WaitGroup{}
	wg.Add(len(domains))

	for _, domain := range domains {
		go func(d string) {
			defer wg.Done()
			require.True(t, bl.Has(d))
		}(domain)
	}

	wg.Wait()

	require.Equal(t, 0, bl.Add(" "))
	require.Equal(t, 0, bl.Add(""))
	require.False(t, bl.Has(" "))
	require.False(t, bl.Has(""))
}
