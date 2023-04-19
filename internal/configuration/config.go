package configuration

import "github.com/spf13/pflag"

type Config struct {
	HistoryFile             string
	HistoryMaxAgeDays       int
	HistoryMaxFiles         int
	HistoryMaxSizeMegabytes int
	GrpcAddr                string
	HttpAddr                string
	SwaggerAddr             string
	DebugAddr               string
}

func Parse() Config {
	var c Config

	pflag.StringVar(&c.HistoryFile, "history-file", "history.jsonlines", "")
	pflag.StringVar(&c.GrpcAddr, "grpc-addr", "127.0.0.1:8082", "")
	pflag.StringVar(&c.HttpAddr, "http-addr", "127.0.0.1:8080", "")
	pflag.StringVar(&c.DebugAddr, "debug-addr", "127.0.0.1:8083", "")
	pflag.StringVar(&c.SwaggerAddr, "swagger-addr", "127.0.0.1:8081", "")
	pflag.IntVar(&c.HistoryMaxAgeDays, "history-max-age-days", 7, "")
	pflag.IntVar(&c.HistoryMaxFiles, "history-max-files", 100, "")
	pflag.IntVar(&c.HistoryMaxSizeMegabytes, "history-max-size-mb", 500, "")
	pflag.Parse()
	return c
}
