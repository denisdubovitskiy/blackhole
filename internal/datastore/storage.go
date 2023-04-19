package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Storage interface {
	Migrate(ctx context.Context) error
	ForEachDomain(ctx context.Context, f func(d string)) error
	ForEachSource(ctx context.Context, f func(d string)) error
	AddSource(ctx context.Context, url string) error
	AddDomains(ctx context.Context, domains []string) error
}

type storage struct {
	db *sql.DB
}

func Open(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

func New(db *sql.DB) Storage {
	return &storage{
		db: db,
	}
}

func (s *storage) AddDomains(ctx context.Context, domains []string) error {
	if len(domains) == 0 {
		return nil
	}

	q := `INSERT INTO domains (domain) VALUES `
	q += strings.TrimSuffix(strings.Repeat("(?),", len(domains)), ",")
	q += ` ON CONFLICT DO NOTHING;`

	args := make([]any, len(domains))
	for i, domain := range domains {
		args[i] = domain
	}

	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("storage: unable to perform migration: %v", err)
	}
	return nil
}

const migration = `
CREATE TABLE IF NOT EXISTS domains (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  domain TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS domains_domain_ux
ON domains (domain);

CREATE TABLE IF NOT EXISTS sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  url TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS sources_url_ux
ON sources (url);
`

func (s *storage) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migration); err != nil {
		return fmt.Errorf("storage: unable to perform migration: %v", err)
	}
	return nil
}

const addSourceQuery = `
INSERT INTO sources (url)
VALUES (?)
ON CONFLICT (url) DO NOTHING;
`

func (s *storage) AddSource(ctx context.Context, url string) error {
	if _, err := s.db.ExecContext(ctx, addSourceQuery, url); err != nil {
		return fmt.Errorf("storage: unable to add source: %v", err)
	}
	return nil
}

const forEachDomainChunkQuery = `
SELECT
	id, 
	domain
FROM domains 
WHERE id > ?
ORDER BY id
LIMIT ?
`

const chunkSize = 100

func (s *storage) ForEachDomain(ctx context.Context, f func(d string)) error {
	var lastID int64
	var handled int

	for {
		rows, err := s.db.QueryContext(ctx, forEachDomainChunkQuery, lastID, chunkSize)
		if err != nil {
			return fmt.Errorf("storage (ForEachDomain): unable to perform query: %v", err)
		}
		handled = 0

		rowsErr := func() error {
			defer rows.Close()

			var id int64
			var domain string

			for rows.Next() {
				if err := rows.Scan(&id, &domain); err != nil {
					return fmt.Errorf("storage (ForEachDomain): unable to scan domain: %v", err)
				}

				f(domain)

				lastID = id
				handled++
			}

			return nil
		}()

		if rowsErr != nil {
			if rowsErr == sql.ErrNoRows {
				return nil
			}
			return rowsErr
		}

		if handled < chunkSize {
			break
		}
	}

	return nil
}

const forEachSourceChunkQuery = `
SELECT
	id, 
	url
FROM sources 
WHERE id > ?
ORDER BY id
LIMIT ?
`

func (s *storage) ForEachSource(ctx context.Context, f func(d string)) error {
	var lastID int64
	var handled int

	for {
		rows, err := s.db.QueryContext(ctx, forEachSourceChunkQuery, lastID, chunkSize)
		if err != nil {
			return fmt.Errorf("storage (ForEachSource): unable to perform query: %v", err)
		}

		handled = 0

		rowsErr := func() error {
			defer rows.Close()

			var id int64
			var url string

			for rows.Next() {
				if err := rows.Scan(&id, &url); err != nil {
					return fmt.Errorf("storage (ForEachSource): unable to scan url: %v", err)
				}

				f(url)

				lastID = id
				handled++
			}

			return nil
		}()

		if rowsErr != nil {
			if rowsErr == sql.ErrNoRows {
				return nil
			}
			return rowsErr
		}

		if handled < chunkSize {
			break
		}
	}

	return nil
}
