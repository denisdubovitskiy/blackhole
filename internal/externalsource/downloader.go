package externalsource

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
)

type HTTP interface {
	Do(r *http.Request) (*http.Response, error)
}
type Downloader interface {
	ForEach(ctx context.Context, url string, f func(d string) error) error
}

func NewDownloader(http HTTP) Downloader {
	return &downloader{http: http}
}

type downloader struct {
	http HTTP
}

func (d *downloader) ForEach(ctx context.Context, url string, f func(d string) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("downloader: unable to compose request for %s: %v", url, err)
	}

	res, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("downloader: unable to perform a request: %s: %v", url, err)
	}
	defer res.Body.Close()

	scanner := bufio.NewScanner(res.Body)

	for scanner.Scan() {
		row := strings.TrimSpace(scanner.Text())
		if len(row) == 0 {
			continue
		}
		if strings.HasPrefix(row, "#") {
			continue
		}
		for _, prefix := range knownPrefixes {
			row = strings.TrimPrefix(row, prefix)
		}
		row = strings.TrimSpace(row)
		if isKnown(row) {
			continue
		}
		if err := f(row); err != nil {
			return err
		}
	}
	return nil
}

func isKnown(domain string) bool {
	switch domain {
	case
		"localhost",
		"local",
		"localhost.localdomain",
		"broadcasthost",
		"ip6-localhost",
		"ip6-loopback",
		"ip6-localnet",
		"ip6-mcastprefix",
		"ip6-allnodes",
		"ip6-allrouters",
		"ip6-allhosts",
		"0.0.0.0":
		return true
	default:
		return false
	}
}

var knownPrefixes = []string{
	"127.0.0.1",
	"255.255.255.255",
	"::1",
	"fe80::1%lo0",
	"ff00::0",
	"ff00::0",
	"ff02::1",
	"ff02::2",
	"ff02::3",
	"0.0.0.0",
}
