package dnsserver

import (
	"context"
	"expvar"
	"fmt"
	"net"
	"time"

	"go.uber.org/atomic"

	"github.com/denisdubovitskiy/blackhole/internal/cache"
	"github.com/denisdubovitskiy/blackhole/internal/history"
	"github.com/denisdubovitskiy/blackhole/internal/resolver"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type Config struct {
	BlockTTL           time.Duration
	UpstreamDNSServers []string
	Blacklist          Blacklist

	Logger  *zap.Logger
	History History
}

func New(config Config) *Server {
	s := &Server{
		blockTTLSeconds: uint32(config.BlockTTL.Seconds()),
		cache:           cache.NewMemoryCache(),
		resolver:        resolver.New(config.UpstreamDNSServers),
		blacklist:       config.Blacklist,
		logger:          config.Logger,
		history:         config.History,
		tcp: &dns.Server{
			Addr:         "0.0.0.0:53",
			Net:          "tcp",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		udp: &dns.Server{
			Addr:         "0.0.0.0:53",
			Net:          "udp",
			UDPSize:      65535,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},

		blocked:  atomic.NewInt32(0),
		resolved: atomic.NewInt32(0),
		failed:   atomic.NewInt32(0),
		cached:   atomic.NewInt32(0),
	}
	if s.logger == nil {
		s.logger = zap.NewNop()
	}
	if s.history == nil {
		s.history = history.NewNop()
	}

	tcpHandler := dns.NewServeMux()
	tcpHandler.HandleFunc(".", s.handler)
	s.tcp.Handler = tcpHandler

	udpHandler := dns.NewServeMux()
	udpHandler.HandleFunc(".", s.handler)
	s.udp.Handler = udpHandler

	expvar.Publish("blackhole_server", expvar.Func(func() any {
		return s.dumpStats()
	}))

	return s
}

type Blacklist interface {
	Has(ctx context.Context, server string) bool
}

type Resolver interface {
	Lookup(req *dns.Msg) (*dns.Msg, error)
}

type Cache interface {
	Get(reqType uint16, domain string) (dns.RR, bool)
	Set(reqType uint16, domain string, ip dns.RR)
}

type History interface {
	Save(entry history.Record)
}

type Server struct {
	tcp *dns.Server
	udp *dns.Server

	cache           Cache
	resolver        Resolver
	blacklist       Blacklist
	history         History
	blockTTLSeconds uint32
	logger          *zap.Logger

	blocked  *atomic.Int32
	resolved *atomic.Int32
	cached   *atomic.Int32
	failed   *atomic.Int32
}

func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errch := make(chan error, 2)

	go func() {
		if err := s.tcp.ListenAndServe(); err != nil {
			errch <- fmt.Errorf("unable to serve tcp: %v", err)
		}
	}()

	go func() {
		if err := s.udp.ListenAndServe(); err != nil {
			errch <- fmt.Errorf("unable to serve udp: %v", err)
		}
	}()

	for {
		select {
		case <-errch:
			cancel()
			s.shutdown()
			return nil
		case <-ctx.Done():
			s.shutdown()
			return nil
		}
	}
}

func (s *Server) shutdown() {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)

	s.tcp.ShutdownContext(shutdownCtx)
	s.udp.ShutdownContext(shutdownCtx)

	shutdownCancel()
}

func (s *Server) handler(w dns.ResponseWriter, req *dns.Msg) {
	defer w.Close()

	question := req.Question[0]

	// достаем из кеша
	if cached, ok := s.cache.Get(question.Qtype, question.Name); ok {
		s.respondFromCache(w, req, cached)
		s.history.Save(history.NewCached(w.RemoteAddr(), question))
		s.logger.Debug(
			"domain is cached",
			zap.String("client", w.RemoteAddr().String()),
			zap.String("domain", question.Name),
		)
		s.cached.Inc()
		return
	}

	// блокируем
	if (question.Qtype == dns.TypeA || question.Qtype == dns.TypeAAAA) &&
		s.blacklist.Has(context.Background(), question.Name) {

		s.blockDomain(w, question, req)
		s.history.Save(history.NewBlocked(w.RemoteAddr(), question))
		s.logger.Debug(
			"domain is blocked",
			zap.String("client", w.RemoteAddr().String()),
			zap.String("domain", question.Name),
		)
		s.blocked.Inc()
		return
	}

	resp, err := s.resolver.Lookup(req)

	if err != nil {
		s.fail(w, req)
		s.history.Save(history.NewFailed(w.RemoteAddr(), question))
		s.logger.Error(
			"failed to resolve a domain",
			zap.String("client", w.RemoteAddr().String()),
			zap.String("domain", question.Name),
			zap.Error(err),
		)
		s.failed.Inc()
		return
	}

	if len(resp.Answer) > 0 {
		s.cache.Set(question.Qtype, question.Name, resp.Answer[0])
	}

	s.writeMsg(w, resp)

	s.history.Save(history.NewResolved(w.RemoteAddr(), question))
	s.logger.Debug(
		"domain is resolved",
		zap.String("client", w.RemoteAddr().String()),
		zap.String("domain", question.Name),
	)
	s.resolved.Inc()
}

func (s *Server) writeMsg(w dns.ResponseWriter, msg *dns.Msg) {
	if err := w.WriteMsg(msg); err != nil {
		s.logger.Error(
			"unable to write a response",
			zap.String("client", w.RemoteAddr().String()),
			zap.String("domain", msg.Question[0].Name),
			zap.Error(err),
		)
	}
}

func (s *Server) blockDomain(w dns.ResponseWriter, question dns.Question, req *dns.Msg) {
	response := &dns.Msg{}
	response.SetReply(req)
	response.Answer = append(response.Answer, s.newBlockedRecord(question.Qtype, dns.RR_Header{
		Name:   question.Name,
		Rrtype: question.Qtype,
		Class:  dns.ClassINET,
		Ttl:    s.blockTTLSeconds,
	}))

	w.WriteMsg(response)
}

func (s *Server) newBlockedRecord(qtype uint16, head dns.RR_Header) dns.RR {
	if qtype == dns.TypeA {
		return &dns.A{
			Hdr: head,
			A:   blockedIPV4,
		}
	}

	return &dns.AAAA{
		Hdr:  head,
		AAAA: blockedIPV6,
	}
}

func (s *Server) fail(w dns.ResponseWriter, req *dns.Msg) {
	resp := &dns.Msg{}
	resp.SetRcode(req, dns.RcodeServerFailure)
	s.writeMsg(w, resp)
}

func (s *Server) respondFromCache(w dns.ResponseWriter, req *dns.Msg, cached dns.RR) {
	response := &dns.Msg{}
	response.SetReply(req)
	response.Answer = append(response.Answer, cached)

	w.WriteMsg(response)
}

type stats struct {
	Blocked  int32 `json:"blocked"`
	Resolved int32 `json:"resolved"`
	Failed   int32 `json:"failed"`
	Cached   int32 `json:"cached"`
}

func (s *Server) dumpStats() stats {
	return stats{
		Blocked:  s.blocked.Load(),
		Resolved: s.resolved.Load(),
		Failed:   s.failed.Load(),
		Cached:   s.cached.Load(),
	}
}

var (
	blockedIPV4 = net.ParseIP("127.0.0.1")
	blockedIPV6 = net.ParseIP("::1")
)
