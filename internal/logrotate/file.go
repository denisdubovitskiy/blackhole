package logrotate

import (
	"io"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	Filename         string
	MaxSizeMegabytes int
	MaxFiles         int
	MaxAgeDays       int
}

func Open(opts Options) io.WriteCloser {
	return &lumberjack.Logger{
		Filename:   opts.Filename,
		MaxSize:    opts.MaxSizeMegabytes,
		MaxBackups: opts.MaxFiles,
		MaxAge:     opts.MaxAgeDays,
	}
}
