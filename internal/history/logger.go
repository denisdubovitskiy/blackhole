package history

import (
	"io"
	"net"

	"go.uber.org/zap/zapcore"

	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type Status string

const (
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
		ClientAddr: remoteAddr.String(),
		Status:     status,
	}
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
	io.Closer
}

func NewLogger(w io.Writer) Logger {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(config),
		zapcore.AddSync(w),
		zapcore.InfoLevel,
	)

	return &logger{logger: zap.New(core)}
}

type logger struct {
	events chan Record
	logger *zap.Logger
}

func (l *logger) Close() error {
	return l.logger.Sync()
}

func (l *logger) Save(entry Record) {
	l.logger.Info(
		"incoming request",
		zap.String("domain", entry.Name),
		zap.String("type", entry.Qtype),
		zap.String("status", string(entry.Status)),
		zap.String("client", entry.ClientAddr),
	)
}

func NewNop() Logger { return NewLogger(io.Discard) }
