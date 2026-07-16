// Package logging provides a shared Zap logger factory.
package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a production-friendly console logger at the given level.
func New(level string) *zap.Logger {
	lvl := zap.InfoLevel
	_ = lvl.UnmarshalText([]byte(level))
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return log
}
