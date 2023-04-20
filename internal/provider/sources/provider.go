package sources

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

type Provider interface {
	AddSource(ctx context.Context, url string) error
	OnRefreshSource(f func(url string))
	RefreshSources(ctx context.Context) error
	RefreshFromSource(ctx context.Context, url string) error
}

type Downloader interface {
	ForEach(ctx context.Context, url string, f func(d string) error) error
}

type Storage interface {
	AddSource(ctx context.Context, url string) error
	AddDomains(ctx context.Context, domains []string) error
	ForEachSource(ctx context.Context, f func(d string)) error
}

type provider struct {
	storage         Storage
	onRefreshSource func(url string)
	downloader      Downloader
}

func NewProvider(
	storage Storage,
	downloader Downloader,
) Provider {
	return &provider{
		storage:    storage,
		downloader: downloader,
	}
}

func (p *provider) OnRefreshSource(f func(url string)) {
	p.onRefreshSource = f
}

func (p *provider) RefreshSources(ctx context.Context) error {
	if p.onRefreshSource == nil {
		return nil
	}

	err := p.storage.ForEachSource(ctx, func(url string) {
		p.onRefreshSource(url)
	})
	if err != nil {
		return fmt.Errorf("source: unable to fetch sources from the database: %v", err)
	}

	return nil
}

func (p *provider) RefreshFromSource(ctx context.Context, url string) error {
	downloadCtx, downloadCancel := context.WithTimeout(ctx, time.Minute)
	defer downloadCancel()

	chunkSize := 100
	chunk := make([]string, 0, chunkSize)

	// Добавлен новый источник доменов для блокировки
	err := p.downloader.ForEach(downloadCtx, url, func(domain string) error {
		chunk = append(chunk, domain)
		if len(chunk) == chunkSize {
			if err := p.storage.AddDomains(ctx, chunk); err != nil {
				return fmt.Errorf("downloader: unable to add domains: %v", err)
			}

			chunk = chunk[:0]
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("storage: unable to add domains: %v", err)
	}

	if len(chunk) > 0 {
		if err := p.storage.AddDomains(ctx, chunk); err != nil {
			return fmt.Errorf("downloader: unable to add domains: %v", err)
		}
	}
	return nil
}

func (p *provider) AddSource(ctx context.Context, u string) error {
	if _, err := url.Parse(u); err != nil {
		return fmt.Errorf("source: url %s is not valid", u)
	}

	if err := p.storage.AddSource(ctx, u); err != nil {
		return fmt.Errorf("source: unable to add source %s: %v", u, err)
	}

	if p.onRefreshSource != nil {
		p.onRefreshSource(u)
	}

	return nil
}
