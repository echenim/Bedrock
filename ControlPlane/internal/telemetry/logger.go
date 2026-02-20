package telemetry

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a structured logger configured for the given environment.
// mode should be "development" or "production".
func NewLogger(mode string) (*zap.Logger, error) {
	switch mode {
	case "development", "dev":
		cfg := zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		return cfg.Build()

	case "production", "prod":
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		return cfg.Build()

	default:
		return nil, fmt.Errorf("telemetry: unknown logger mode %q (want 'development' or 'production')", mode)
	}
}

// NewNopLogger returns a no-op logger (useful for tests).
func NewNopLogger() *zap.Logger {
	return zap.NewNop()
}
