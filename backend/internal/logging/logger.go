package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger      *zap.Logger = zap.NewNop()
	atomicLevel             = zap.NewAtomicLevelAt(zapcore.InfoLevel)
)

// customTimeEncoder 自定义时间编码器，格式为 YYYY-MM-DD HH:mm:ss
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

// NewLogger creates a new logger instance with dynamic level support
func NewLogger(level string, file string) (*zap.Logger, error) {
	// Parse log level
	logLevel := zapcore.InfoLevel
	if parsed, err := ParseLevel(level); err == nil {
		logLevel = parsed
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
		EncodeTime:     customTimeEncoder,                // 时间格式化：YYYY-MM-DD HH:mm:ss
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
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

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

// ParseLevel 解析日志级别字符串（支持 debug/info/warn/warning/error/dpanic/panic/fatal，忽略大小写与前后空白）
func ParseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "dpanic":
		return zapcore.DPanicLevel, nil
	case "panic":
		return zapcore.PanicLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("invalid log level: %s", level)
	}
}

// SetLogLevel 动态设置日志级别
func SetLogLevel(level string) error {
	zapLevel, err := ParseLevel(level)
	if err != nil {
		return err
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

func ZapString(key, val string) zap.Field {
	return zap.String(key, val)
}
