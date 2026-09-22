package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func hashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type zapGormLogger struct {
	log   *zap.Logger
	level gormlogger.LogLevel
}

func NewZapGormLogger(log *zap.Logger, level gormlogger.LogLevel) gormlogger.Interface {
	return &zapGormLogger{log: log.Named("gorm"), level: level}
}

func (l *zapGormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	cp := *l
	cp.level = level
	return &cp
}

func (l *zapGormLogger) Info(_ context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Info {
		l.log.Info(fmt.Sprintf(msg, data...))
	}
}

func (l *zapGormLogger) Warn(_ context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Warn {
		l.log.Warn(fmt.Sprintf(msg, data...))
	}
}

func (l *zapGormLogger) Error(_ context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Error {
		l.log.Error(fmt.Sprintf(msg, data...))
	}
}

func (l *zapGormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.level >= gormlogger.Error:
		l.log.Error("sql", zap.Error(err), zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
	case elapsed > 200*time.Millisecond && l.level >= gormlogger.Warn:
		l.log.Warn("slow sql", zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
	case l.level >= gormlogger.Info:
		l.log.Debug("sql", zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
	}
}
