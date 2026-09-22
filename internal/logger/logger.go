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

	minLevel := zapcore.InfoLevel
	if err := minLevel.UnmarshalText([]byte(cfg.Level)); err != nil {
		minLevel = zapcore.InfoLevel
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

	levels := []zapcore.Level{
		zapcore.DebugLevel,
		zapcore.InfoLevel,
		zapcore.WarnLevel,
		zapcore.ErrorLevel,
	}

	var cores []zapcore.Core
	for _, lv := range levels {
		w, err := newRotateWriter(cfg, lv.String())
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(jsonEnc, zapcore.AddSync(w), levelFilter(minLevel, lv)))
	}
	if isDev {
		consoleEnc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		cores = append(cores, zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), minLevel))
	}

	return zap.New(zapcore.NewTee(cores...), zap.AddCaller()), nil
}

func newRotateWriter(cfg config.LogConfig, levelName string) (*rotatelogs.RotateLogs, error) {
	pattern := filepath.Join(cfg.Dir, cfg.Filename+"-"+levelName+"-%Y-%m-%d.log")
	w, err := rotatelogs.New(
		pattern,
		rotatelogs.WithMaxAge(30*24*time.Hour),
		rotatelogs.WithRotationTime(24*time.Hour),
	)
	if err != nil {
		return nil, fmt.Errorf("init rotate logs %s: %w", levelName, err)
	}
	return w, nil
}

func levelFilter(min, target zapcore.Level) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		if lvl < min {
			return false
		}
		if target >= zapcore.ErrorLevel {
			return lvl >= zapcore.ErrorLevel
		}
		return lvl == target
	})
}
