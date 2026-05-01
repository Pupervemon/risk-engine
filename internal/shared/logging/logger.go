package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewColorLogger builds a console logger with ANSI-colored levels for local terminal output.
func NewColorLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.CallerKey = "caller"
	return cfg.Build()
}