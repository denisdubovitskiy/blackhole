package main

import (
	"bytes"
	"context"
	"expvar"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/denisdubovitskiy/blackhole/internal/externalsource"
	"github.com/denisdubovitskiy/blackhole/internal/provider/sources"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	"github.com/denisdubovitskiy/blackhole/internal/blacklist"
	"github.com/denisdubovitskiy/blackhole/internal/configuration"
	"github.com/denisdubovitskiy/blackhole/internal/datastore"
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

	historyLogger := history.NewLogger(historyFile)
	defer historyLogger.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Debug("opening a database file")
	database, err := datastore.Open("./blackhole.sqlite3")
	if err != nil {
		log.Fatal("unable to open a database", zap.Error(err))
	}
	log.Debug("database is opened")

	storage := datastore.New(database)

	migrateCtx, migrateCancel := context.WithTimeout(ctx, 5*time.Second)
	defer migrateCancel()

	log.Debug("performing database migration")
	// Автомиграция бд (идемпотентная)
	if err := storage.Migrate(migrateCtx); err != nil {
		log.Fatal("unable to migrate a database", zap.Error(err))
	}
	log.Debug("database schema is up to date")

	mux := http.NewServeMux()
	mux.Handle("/", swaggerui.New("Blackhole", "/swagger.json", "/"))
	mux.HandleFunc("/swagger.json", func(writer http.ResponseWriter, request *http.Request) {
		swg := bytes.ReplaceAll(pb.SwaggerUI, []byte(`"swagger": "2.0",`), []byte(`"swagger": "2.0","host": "`+config.HttpAddr+`",`))

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
		log.Debug("running Swagger UI", zap.String("address", config.SwaggerAddr))
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

	debugServer := http.Server{
		Handler: expvar.Handler(),
		Addr:    config.DebugAddr,
	}
	go func() {
		log.Debug("running debug server", zap.String("address", config.DebugAddr))
		if err := debugServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("debug server error", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := debugServer.Shutdown(shutdownCtx); err != nil {
			log.Error("unable to shut debug server down gracefully", zap.Error(err))
		}
	}()

	downloader := externalsource.NewDownloader(http.DefaultClient)
	sourceProvider := sources.NewProvider(storage)
	sourceProvider.OnRefreshSource(func(url string) {
		go func() {
			downloadCtx, downloadCancel := context.WithTimeout(ctx, time.Minute)
			defer downloadCancel()

			log.Debug("downloader: starting update from list", zap.String("url", url))

			chunkSize := 100
			chunk := make([]string, 0, chunkSize)

			// Добавлен новый источник доменов для блокировки
			err := downloader.ForEach(downloadCtx, url, func(domain string) {
				chunk = append(chunk, domain)
				if len(chunk) == chunkSize {
					if err := storage.AddDomains(ctx, chunk); err != nil {
						log.Error("downloader: unable to add domains", zap.Error(err))
					}

					chunk = chunk[:0]
				}
			})
			if err != nil {
				log.Error("downloader: unable to perform update from the list", zap.String("url", url), zap.Error(err))
			} else {
				log.Debug("downloader: update finished", zap.String("url", url))
			}
			if len(chunk) > 0 {
				if err := storage.AddDomains(ctx, chunk); err != nil {
					log.Error("downloader: unable to add domains", zap.Error(err))
				}
			}
		}()
	})
	controller := handler.New(bl, sourceProvider)

	grpcServer := grpc.NewServer()
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

	go func() {
		log.Debug("populating blacklist from the database")
		// Прогрев черного списка на старте из базы данных
		forEachErr := storage.ForEachDomain(ctx, func(domain string) {
			bl.Add(ctx, domain)
		})
		if forEachErr != nil {
			log.Fatal("unable to populate blacklist from the database", zap.Error(forEachErr))
		}
		log.Debug("blacklist is up to date")
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
	log.Debug("starting DNS server")
	if err := dnsServer.Run(ctx); err != nil {
		log.Error("DNS server listen error", zap.Error(err))
	}
}
