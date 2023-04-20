package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

type Storage interface {
	Migrate(ctx context.Context) error
	ForEachDomain(ctx context.Context, f func(d string)) error
	ForEachSource(ctx context.Context, f func(d string)) error
	AddSource(ctx context.Context, url string) error
	AddDomains(ctx context.Context, domains []string) error
	AddHistoryRecords(ctx context.Context, records []HistoryRecord) error
	Cleanup(ctx context.Context, skip int) error
	RunPeriodicCleanup(ctx context.Context)
}

type storage struct {
	db          *sql.DB
	historySize int
	log         *zap.Logger
}

func Open(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

func New(db *sql.DB, historySize int, log *zap.Logger) Storage {
	return &storage{
		db:          db,
		historySize: historySize,
		log:         log,
	}
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

CREATE TABLE IF NOT EXISTS history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT,
    type TEXT,
    status TEXT,
    client_addr TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func (s *storage) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migration); err != nil {
		return fmt.Errorf("storage: unable to perform migration: %v", err)
	}
	return nil
}

const idForRemovalQuery = `
SELECT id
FROM history
ORDER BY id DESC
LIMIT 1
OFFSET ?;
`

const cleanupQuery = `
DELETE
FROM history
WHERE id < ?;
`

func (s *storage) Cleanup(ctx context.Context, skip int) error {
	row := s.db.QueryRowContext(ctx, idForRemovalQuery, skip)
	var id int64

	if err := row.Scan(&id); err != nil {
		return err
	}

	if err := row.Err(); err != nil {
		return err
	}

	if id == 0 {
		return nil
	}

	if _, err := s.db.ExecContext(ctx, cleanupQuery, id); err != nil {
		return err
	}

	return nil
}

type HistoryRecord struct {
	Type       string
	Domain     string
	Status     string
	ClientAddr string
}

func (s *storage) AddHistoryRecords(ctx context.Context, records []HistoryRecord) error {
	if len(records) == 0 {
		return nil
	}

	q := `INSERT INTO history (domain, type, status, client_addr) VALUES `
	q += strings.TrimSuffix(strings.Repeat("(?, ?, ?, ?),", len(records)), ",")
	q += ` ON CONFLICT DO NOTHING;`

	args := make([]any, 0, len(records)*4)
	for _, record := range records {
		args = append(args, record.Domain, record.Type, record.Status, record.ClientAddr)
	}

	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("storage: unable to save history: %v", err)
	}

	return nil
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
		return fmt.Errorf("storage: unable to add domains: %v", err)
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

func (s *storage) RunPeriodicCleanup(ctx context.Context) {
	ticker := time.NewTimer(5 * time.Minute)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				s.log.Debug("storage: running periodic history cleanup")
				if err := s.Cleanup(ctx, s.historySize); err != nil {
					s.log.Error("storage: unable to cleanup history", zap.Error(err))
				}
			}
		}
	}()
}
