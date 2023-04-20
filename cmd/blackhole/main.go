package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/denisdubovitskiy/blackhole/internal/blacklist"
	"github.com/denisdubovitskiy/blackhole/internal/configuration"
	"github.com/denisdubovitskiy/blackhole/internal/datastore"
	"github.com/denisdubovitskiy/blackhole/internal/externalsource"
	"github.com/denisdubovitskiy/blackhole/internal/handler"
	"github.com/denisdubovitskiy/blackhole/internal/history"
	"github.com/denisdubovitskiy/blackhole/internal/listeners/debug"
	"github.com/denisdubovitskiy/blackhole/internal/listeners/dnsserver"
	"github.com/denisdubovitskiy/blackhole/internal/listeners/grpcgateway"
	"github.com/denisdubovitskiy/blackhole/internal/listeners/grpcserver"
	"github.com/denisdubovitskiy/blackhole/internal/listeners/swagger"
	"github.com/denisdubovitskiy/blackhole/internal/provider/sources"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

	bl := blacklist.New(config.BlacklistBucketsCount)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Debug("opening a database file")
	database, err := datastore.Open("./blackhole.sqlite3")
	if err != nil {
		log.Fatal("unable to open a database", zap.Error(err))
	}
	log.Debug("database is opened")

	storage := datastore.New(database, config.HistorySize, log)
	storage.RunPeriodicCleanup(ctx)

	migrateCtx, migrateCancel := context.WithTimeout(ctx, 5*time.Second)
	defer migrateCancel()

	log.Debug("performing database migration")
	// Автомиграция бд (идемпотентная)
	if err := storage.Migrate(migrateCtx); err != nil {
		log.Fatal("unable to migrate a database", zap.Error(err))
	}
	log.Debug("database schema is up to date")

	downloader := externalsource.NewDownloader(http.DefaultClient)
	sourceProvider := sources.NewProvider(storage, downloader)
	historyLogger := history.NewLogger(storage, log)
	historyLogger.Run(ctx)

	sourceProvider.OnRefreshSource(func(url string) {
		go func() {
			downloadCtx, downloadCancel := context.WithTimeout(ctx, time.Minute)
			defer downloadCancel()

			log.Debug("downloader: starting update from list", zap.String("url", url))
			if err := sourceProvider.RefreshFromSource(downloadCtx, url); err != nil {
				log.Error("downloader: unable to refresh from source", zap.Error(err))
			}
			log.Debug("downloader: update finished", zap.String("url", url))
		}()
	})

	controller := handler.New(bl, sourceProvider)

	ui := swagger.NewUI(config.SwaggerAddr, config.HttpAddr, log)
	ui.Run(ctx)

	ds := debug.NewServer(config.DebugAddr, log)
	ds.Run(ctx)

	gs := grpcserver.New(config.GrpcAddr, controller, log)
	gs.Run(ctx)

	gw := grpcgateway.NewServer(config.GrpcAddr, config.HttpAddr, log)
	gw.Run(ctx)

	go func() {
		log.Debug("migration: populating blacklist from the database")
		// Прогрев черного списка на старте из базы данных
		forEachErr := storage.ForEachDomain(ctx, func(domain string) {
			bl.Add(ctx, domain)
		})
		if forEachErr != nil {
			log.Fatal("migration: unable to populate blacklist from the database", zap.Error(forEachErr))
		}
		log.Debug("migration: blacklist is up to date")
	}()

	dnsServer := dnsserver.New(dnsserver.Config{
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
		log.Error("DNS dnsserver listen error", zap.Error(err))
	}
}
