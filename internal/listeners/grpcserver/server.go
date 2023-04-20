package grpcserver

import (
	"context"
	"log"
	"net"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Server interface {
	Run(ctx context.Context)
}

func New(addr string, controller pb.BlackholeServer, log *zap.Logger) Server {
	return &server{
		addr:       addr,
		controller: controller,
		log:        log,
	}
}

type server struct {
	controller pb.BlackholeServer
	addr       string
	log        *zap.Logger
}

func (s *server) Run(ctx context.Context) {
	grpcServer := grpc.NewServer()
	pb.RegisterBlackholeServer(grpcServer, s.controller)

	go func() {
		s.log.Debug("running gRPC server", zap.String("address", s.addr))

		lis, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Fatal("unable to listen grpc", zap.Error(err))
		}

		if err := grpcServer.Serve(lis); err != nil {
			s.log.Error("grpc dnsserver error", zap.Error(err))
		}
	}()

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
}
