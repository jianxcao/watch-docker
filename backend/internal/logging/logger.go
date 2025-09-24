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

	// 配置 Encoder
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",                           // 时间字段的键
		LevelKey:       "level",                          // 日志级别字段的键
		NameKey:        "logger",                         // 日志名称字段的键
		CallerKey:      "caller",                         // 调用者字段的键
		MessageKey:     "msg",                            // 消息字段的键
		StacktraceKey:  "stacktrace",                     // 堆栈字段的键
		LineEnding:     zapcore.DefaultLineEnding,        // 行结尾
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // 日志级别格式化
		EncodeTime:     zapcore.ISO8601TimeEncoder,       // 时间格式化
		EncodeDuration: zapcore.StringDurationEncoder,    // 时长格式化
		EncodeCaller:   zapcore.ShortCallerEncoder,       // 调用者格式化
	}
	jsonEncoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",                         // 时间字段的键
		LevelKey:       "level",                        // 日志级别字段的键
		NameKey:        "logger",                       // 日志名称字段的键
		CallerKey:      "caller",                       // 调用者字段的键
		MessageKey:     "msg",                          // 消息字段的键
		LineEnding:     zapcore.DefaultLineEnding,      // 行结尾
		EncodeLevel:    zapcore.CapitalLevelEncoder,    // 日志级别格式化
		EncodeTime:     zapcore.EpochMillisTimeEncoder, // 时间格式化
		EncodeDuration: zapcore.StringDurationEncoder,  // 时长格式化
		EncodeCaller:   zapcore.ShortCallerEncoder,     // 调用者格式化
	}

	// Create atomic level with initial value
	atomicLevel = zap.NewAtomicLevelAt(logLevel)

	jsonEncoder := zapcore.NewJSONEncoder(jsonEncoderConfig)
	// Choose encoder based on format
	var encoder zapcore.Encoder
	if format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = jsonEncoder
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
	channelCore := zapcore.NewCore(jsonEncoder, zapcore.AddSync(&ChannelWriter{}), atomicLevel)
	teeCore := zapcore.NewTee(core, channelCore)
	// Create logger
	logger := zap.New(teeCore, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
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
