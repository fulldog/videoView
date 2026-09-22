package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"videoview/internal/config"
)

func New(cfg config.LogConfig, isDev bool) (*zap.Logger, error) {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	fileWriter, err := rotatelogs.New(
		filepath.Join(cfg.Dir, cfg.Filename+"-%Y-%m-%d.log"),
		rotatelogs.WithMaxAge(30*24*time.Hour),
		rotatelogs.WithRotationTime(24*time.Hour),
	)
	if err != nil {
		return nil, fmt.Errorf("init rotate logs: %w", err)
	}

	jsonEnc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	cores := []zapcore.Core{
		zapcore.NewCore(jsonEnc, zapcore.AddSync(fileWriter), level),
	}
	if isDev {
		consoleEnc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		cores = append(cores, zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), level))
	}

	lg := zap.New(zapcore.NewTee(cores...), zap.AddCaller())
	return lg, nil
}
