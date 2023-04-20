package swagger

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"time"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	swaggerui "github.com/swaggest/swgui/v3emb"
	"go.uber.org/zap"
)

type UI interface {
	Run(ctx context.Context)
}

type ui struct {
	targetAddr string
	uiAddr     string
	log        *zap.Logger
}

func NewUI(
	uiAddr string,
	targetAddr string,
	logger *zap.Logger,
) UI {
	return &ui{
		targetAddr: targetAddr,
		uiAddr:     uiAddr,
		log:        logger,
	}
}

func (u *ui) newServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/", swaggerui.New("Blackhole", "/swagger.json", "/"))
	mux.HandleFunc("/swagger.json", func(writer http.ResponseWriter, request *http.Request) {
		swg := bytes.ReplaceAll(pb.SwaggerUI, []byte(`"swagger": "2.0",`), []byte(`"swagger": "2.0","host": "`+u.targetAddr+`",`))

		if _, err := writer.Write(swg); err != nil {
			u.log.Error(
				"unable to write a swagger definition",
				zap.String("component", "http"),
				zap.Error(err),
			)
		}
	})

	return &http.Server{
		Handler: mux,
		Addr:    u.uiAddr,
	}
}

func (u *ui) Run(ctx context.Context) {
	server := u.newServer()

	go func() {
		u.log.Debug("running Swagger UI", zap.String("address", u.uiAddr))
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("http dnsserver error", zap.Error(err))
		}
	}()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			u.log.Error("unable to shut http dnsserver down gracefully", zap.Error(err))
		}
	}()
}
