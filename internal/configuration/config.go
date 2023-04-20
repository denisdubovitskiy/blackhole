package configuration

import "github.com/spf13/pflag"

type Config struct {
	GrpcAddr              string
	HttpAddr              string
	SwaggerAddr           string
	DebugAddr             string
	BlacklistBucketsCount int
	HistorySize           int
}

func Parse() Config {
	var c Config

	pflag.StringVar(&c.GrpcAddr, "grpc-addr", "127.0.0.1:8082", "")
	pflag.StringVar(&c.HttpAddr, "http-addr", "127.0.0.1:8080", "")
	pflag.StringVar(&c.DebugAddr, "debug-addr", "127.0.0.1:8083", "")
	pflag.StringVar(&c.SwaggerAddr, "swagger-addr", "127.0.0.1:8081", "")
	pflag.IntVar(&c.HistorySize, "history-size", 100, "")
	pflag.IntVar(&c.BlacklistBucketsCount, "blacklist-buckets-count", 512, "")
	pflag.Parse()
	return c
}
