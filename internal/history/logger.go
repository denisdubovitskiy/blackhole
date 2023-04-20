package history

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/denisdubovitskiy/blackhole/internal/datastore"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type Status string

const (
	chunkSize = 100

	StatusCached   Status = "cached"
	StatusBlocked  Status = "blocked"
	StatusFailed   Status = "failed"
	StatusResolved Status = "resolved"
)

type Record struct {
	Qtype      string
	Name       string
	Status     Status
	ClientAddr string
}

func NewRecord(remoteAddr net.Addr, question dns.Question, status Status) Record {
	return Record{
		Qtype:      dns.TypeToString[question.Qtype],
		Name:       question.Name,
		ClientAddr: stripRemoteAddr(remoteAddr.String()),
		Status:     status,
	}
}

func stripRemoteAddr(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

func NewCached(remoteAddr net.Addr, question dns.Question) Record {
	return NewRecord(remoteAddr, question, StatusCached)
}

func NewBlocked(remoteAddr net.Addr, question dns.Question) Record {
	return NewRecord(remoteAddr, question, StatusBlocked)
}

func NewFailed(remoteAddr net.Addr, question dns.Question) Record {
	return NewRecord(remoteAddr, question, StatusFailed)
}

func NewResolved(remoteAddr net.Addr, question dns.Question) Record {
	return NewRecord(remoteAddr, question, StatusResolved)
}

type Logger interface {
	Save(entry Record)
	Run(ctx context.Context)
}

type Storage interface {
	AddHistoryRecords(ctx context.Context, records []datastore.HistoryRecord) error
}

func NewLogger(storage Storage, log *zap.Logger) Logger {
	return &logger{
		storage: storage,
		events:  make(chan Record, chunkSize),
		log:     log,
	}
}

type logger struct {
	events  chan Record
	storage Storage
	log     *zap.Logger
}

func (l *logger) Close() error {
	return nil
}

func (l *logger) Save(entry Record) {
	l.events <- entry
}

func (l *logger) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)

	go func() {
		defer ticker.Stop()

		chunk := make([]Record, 0, chunkSize)

		for {
			select {
			case <-ctx.Done():
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := l.saveChunk(ctx, chunk); err != nil {
					l.log.Error("history: unable to save chunk", zap.Error(err))
				}
				cancel()
				return

			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := l.saveChunk(ctx, chunk); err != nil {
					l.log.Error("history: unable to save chunk", zap.Error(err))
				}
				cancel()
				chunk = chunk[:0]
				continue

			case e, ok := <-l.events:
				if !ok {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := l.saveChunk(ctx, chunk); err != nil {
						l.log.Error("history: unable to save chunk", zap.Error(err))
					}
					cancel()
					return
				}

				chunk = append(chunk, e)
				if len(chunk) == chunkSize {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := l.saveChunk(ctx, chunk); err != nil {
						l.log.Error("history: unable to save chunk", zap.Error(err))
					}
					cancel()
					chunk = chunk[:0]
				}
				continue
			}
		}
	}()
}

func (l *logger) saveChunk(ctx context.Context, chunk []Record) error {
	records := make([]datastore.HistoryRecord, len(chunk))

	for i, rec := range chunk {
		records[i] = datastore.HistoryRecord{
			Type:       rec.Qtype,
			Domain:     rec.Name,
			Status:     string(rec.Status),
			ClientAddr: rec.ClientAddr,
		}
	}

	if err := l.storage.AddHistoryRecords(ctx, records); err != nil {
		return fmt.Errorf("unable to save records: %v", err)
	}

	return nil
}

var nilStore Storage = &nilStorage{}

type nilStorage struct {
}

func (n nilStorage) AddHistoryRecords(ctx context.Context, records []datastore.HistoryRecord) error {
	return nil
}

func NewNop() Logger { return NewLogger(nilStore, zap.NewNop()) }
