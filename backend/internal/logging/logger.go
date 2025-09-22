package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
)

func NewLogger(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if level != "" {
		if err := cfg.Level.UnmarshalText([]byte(level)); err != nil {
			return nil, err
		}
	}
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	Logger, _ = cfg.Build()
	return Logger, nil
}

// helper fields / errors
func ZapField(key string, val interface{}) zap.Field {
	return zap.Any(key, val)
}

func ZapErr(err error) zap.Field {
	return zap.Error(err)
}
