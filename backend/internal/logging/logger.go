package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger      *zap.Logger
	atomicLevel zap.AtomicLevel
)

// NewLogger creates a new logger instance with dynamic level support
func NewLogger(level string, format string, file string) (*zap.Logger, error) {
	// Parse log level
	logLevel := zapcore.InfoLevel
	switch level {
	case "debug":
		logLevel = zapcore.DebugLevel
	case "info":
		logLevel = zapcore.InfoLevel
	case "warn":
		logLevel = zapcore.WarnLevel
	case "error":
		logLevel = zapcore.ErrorLevel
	}

	// Create atomic level with initial value
	atomicLevel = zap.NewAtomicLevelAt(logLevel)

	// Create encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// Choose encoder based on format
	var encoder zapcore.Encoder
	if format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create write syncer
	var writeSyncer zapcore.WriteSyncer
	if file != "" {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		writeSyncer = zapcore.AddSync(f)
	} else {
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// Create core with atomic level
	core := zapcore.NewCore(encoder, writeSyncer, atomicLevel)

	// Create logger
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	Logger = logger
	return logger, nil
}

// SetLogLevel 动态设置日志级别
func SetLogLevel(level string) error {
	var zapLevel zapcore.Level
	switch strings.ToLower(level) {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn", "warning":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	case "dpanic":
		zapLevel = zapcore.DPanicLevel
	case "panic":
		zapLevel = zapcore.PanicLevel
	case "fatal":
		zapLevel = zapcore.FatalLevel
	default:
		return fmt.Errorf("invalid log level: %s", level)
	}

	atomicLevel.SetLevel(zapLevel)
	return nil
}

// GetLogLevel 获取当前日志级别
func GetLogLevel() string {
	return atomicLevel.Level().String()
}

// helper fields / errors
func ZapField(key string, val interface{}) zap.Field {
	return zap.Any(key, val)
}

func ZapErr(err error) zap.Field {
	return zap.Error(err)
}
