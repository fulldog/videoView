package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestLevelFilter(t *testing.T) {
	min := zapcore.InfoLevel
	debug := levelFilter(min, zapcore.DebugLevel)
	info := levelFilter(min, zapcore.InfoLevel)
	warn := levelFilter(min, zapcore.WarnLevel)
	errf := levelFilter(min, zapcore.ErrorLevel)

	if debug.Enabled(zapcore.DebugLevel) {
		t.Fatal("debug file should be disabled when min is info")
	}
	if !info.Enabled(zapcore.InfoLevel) || info.Enabled(zapcore.WarnLevel) {
		t.Fatal("info file should accept only info")
	}
	if !warn.Enabled(zapcore.WarnLevel) || warn.Enabled(zapcore.InfoLevel) {
		t.Fatal("warn file should accept only warn")
	}
	if !errf.Enabled(zapcore.ErrorLevel) || !errf.Enabled(zapcore.FatalLevel) || errf.Enabled(zapcore.WarnLevel) {
		t.Fatal("error file should accept error and above")
	}
}
