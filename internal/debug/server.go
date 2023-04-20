package debug

import (
	"context"
	"expvar"
	"log"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Server interface {
	Run(ctx context.Context)
}

type server struct {
	addr string
	log  *zap.Logger
}

func NewServer(addr string, log *zap.Logger) Server {
	return &server{
		addr: addr,
		log:  log,
	}
}

func (s *server) Run(ctx context.Context) {
	debugServer := http.Server{
		Handler: expvar.Handler(),
		Addr:    s.addr,
	}
	go func() {
		s.log.Debug("running debug dnsserver", zap.String("address", s.addr))
		if err := debugServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("debug dnsserver error", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := debugServer.Shutdown(shutdownCtx); err != nil {
			s.log.Error("unable to shut debug dnsserver down gracefully", zap.Error(err))
		}
	}()
}
