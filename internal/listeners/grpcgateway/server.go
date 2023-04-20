package grpcgateway

import (
	"context"
	"log"
	"net/http"
	"time"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Server interface {
	Run(ctx context.Context)
}

type server struct {
	grpcAddr string
	httpAddr string
	log      *zap.Logger
}

func NewServer(
	grpcAddr string,
	httpAddr string,
	log *zap.Logger,
) Server {
	return &server{
		grpcAddr: grpcAddr,
		httpAddr: httpAddr,
		log:      log,
	}
}

func (s *server) Run(ctx context.Context) {
	gwMux := runtime.NewServeMux()
	gwErr := pb.RegisterBlackholeHandlerFromEndpoint(
		ctx,
		gwMux,
		s.grpcAddr,
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	)
	if gwErr != nil {
		log.Fatal("unable to register a gateway", zap.Error(gwErr))
	}
	gwServer := http.Server{
		Handler: cors.AllowAll().Handler(gwMux),
		Addr:    s.httpAddr,
	}
	go func() {
		s.log.Debug("running gRPC gateway server", zap.String("address", s.httpAddr))
		if err := gwServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("http dnsserver error", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := gwServer.Shutdown(shutdownCtx); err != nil {
			s.log.Error("unable to shut http dnsserver down gracefully", zap.Error(err))
		}
	}()
}
