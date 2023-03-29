package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	"github.com/denisdubovitskiy/blackhole/internal/blacklist"
	"github.com/denisdubovitskiy/blackhole/internal/configuration"
	"github.com/denisdubovitskiy/blackhole/internal/handler"
	"github.com/denisdubovitskiy/blackhole/internal/history"
	"github.com/denisdubovitskiy/blackhole/internal/logrotate"
	"github.com/denisdubovitskiy/blackhole/internal/server"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	swaggerui "github.com/swaggest/swgui/v3emb"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const blacklistBucketsCount = 256

func main() {
	config := configuration.Parse()
	fmt.Println(config)

	logConfig := zap.NewProductionEncoderConfig()
	logConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log := zap.New(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(logConfig),
			zapcore.AddSync(os.Stdout),
			zapcore.DebugLevel,
		),
	)
	defer log.Sync()

	bl := blacklist.New(blacklistBucketsCount)

	historyFile := logrotate.Open(logrotate.Options{
		Filename:         config.HistoryFile,
		MaxSizeMegabytes: config.HistoryMaxSizeMegabytes,
		MaxFiles:         config.HistoryMaxFiles,
		MaxAgeDays:       config.HistoryMaxAgeDays,
	})
	defer historyFile.Close()

	historyLogger := history.New(historyFile)
	defer historyLogger.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.Handle("/", swaggerui.New("Blackhole", "/swagger.json", "/"))
	mux.HandleFunc("/swagger.json", func(writer http.ResponseWriter, request *http.Request) {
		swg := bytes.ReplaceAll(pb.SwaggerUI, []byte(`"swagger": "2.0",`), []byte(`"swagger": "2.0","host": config.HttpAddr,`))

		if _, err := writer.Write(swg); err != nil {
			log.Error(
				"unable to write a swagger definition",
				zap.String("component", "http"),
				zap.Error(err),
			)
		}
	})

	openapiServer := http.Server{
		Handler: mux,
		Addr:    config.SwaggerAddr,
	}
	go func() {
		if err := openapiServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("http server error", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := openapiServer.Shutdown(shutdownCtx); err != nil {
			log.Error("unable to shut http server down gracefully", zap.Error(err))
		}
	}()

	grpcServer := grpc.NewServer()
	controller := handler.New(bl)

	pb.RegisterBlackholeServer(grpcServer, controller)

	go func() {
		lis, err := net.Listen("tcp", config.GrpcAddr)
		if err != nil {
			log.Fatal("unable to listen grpc", zap.Error(err))
		}
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("grpc server error", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	gwMux := runtime.NewServeMux()
	gwErr := pb.RegisterBlackholeHandlerFromEndpoint(
		ctx,
		gwMux,
		config.GrpcAddr,
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	)
	if gwErr != nil {
		log.Fatal("unable to register a gateway", zap.Error(gwErr))
	}
	gwServer := http.Server{
		Handler: cors.AllowAll().Handler(gwMux),
		Addr:    config.HttpAddr,
	}
	go func() {
		if err := gwServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("http server error", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := gwServer.Shutdown(shutdownCtx); err != nil {
			log.Error("unable to shut http server down gracefully", zap.Error(err))
		}
	}()

	dnsServer := server.New(server.Config{
		BlockTTL: 10 * time.Second,
		UpstreamDNSServers: []string{
			// google
			"8.8.8.8:53",
			"8.8.4.4:53",
			// cloudflare
			"1.1.1.1:53",
			"1.0.0.1:53",
			// control d
			"76.76.2.0:53",
			"76.76.10.0:53",
			// quad9
			"9.9.9.9:53",
			"149.112.112.112:53",
			// open dns home
			"208.67.222.222:53",
			"208.67.220.220:53",
		},
		Blacklist: bl,
		History:   historyLogger,
		Logger:    log,
	})
	if err := dnsServer.Run(ctx); err != nil {
		log.Error("DNS server listen error", zap.Error(err))
	}
}
