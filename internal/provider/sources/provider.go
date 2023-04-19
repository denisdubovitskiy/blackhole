package sources

import (
	"context"
	"fmt"
	"net/url"
)

type Provider interface {
	AddSource(ctx context.Context, url string) error
	OnRefreshSource(f func(url string))
	RefreshSources(ctx context.Context) error
}

type Storage interface {
	AddSource(ctx context.Context, url string) error
	ForEachSource(ctx context.Context, f func(d string)) error
}

type provider struct {
	storage         Storage
	onRefreshSource func(url string)
}

func NewProvider(storage Storage) Provider {
	return &provider{storage: storage}
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
		fmt.Errorf("source: unable to fetch sources from the database: %v", err)
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
