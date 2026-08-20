package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func NewLogger(logDir string, level string) (*zap.Logger, error) {
	if logDir == "" {
		return nil, fmt.Errorf("log dir 为空")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	lvl, err := zapcore.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zapcore.InfoLevel
	}

	consoleEncoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		CallerKey:      "caller",
		MessageKey:     "msg",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(logDir, "server.log"),
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     30,
		Compress:   true,
	})
	consoleWriter := zapcore.AddSync(os.Stdout)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, fileWriter, lvl),
		zapcore.NewCore(consoleEncoder, consoleWriter, lvl),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
	return logger, nil
}

func NewAuditLogger(logDir string, retentionDays int) (*zap.Logger, error) {
	if logDir == "" {
		return nil, fmt.Errorf("log dir 为空")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	if retentionDays <= 0 {
		retentionDays = 180
	}
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		CallerKey:      "caller",
		MessageKey:     "msg",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})
	writer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(logDir, "audit.log"),
		MaxSize:    100,
		MaxBackups: 20,
		MaxAge:     retentionDays,
		Compress:   true,
	})
	core := zapcore.NewCore(encoder, writer, zapcore.InfoLevel)
	return zap.New(core, zap.AddCaller()), nil
}