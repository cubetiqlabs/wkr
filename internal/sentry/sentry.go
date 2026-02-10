package sentry

import (
	"time"

	"github.com/cubetiqlabs/cubis-wkr/internal/config"
	"github.com/cubetiqlabs/cubis-wkr/internal/logger"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
)

func Init(cfg config.SentryConfig) error {
	if !cfg.Enabled || cfg.DSN == "" {
		logger.Info("sentry disabled")
		return nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		TracesSampleRate: cfg.TracesSampleRate,
		Debug:            cfg.Debug,
	})
	if err != nil {
		return err
	}

	logger.Info("sentry initialized", zap.String("env", cfg.Environment))
	return nil
}

func Flush() {
	sentry.Flush(2 * time.Second)
}

func CaptureError(err error) {
	sentry.CaptureException(err)
}

func CaptureMessage(msg string) {
	sentry.CaptureMessage(msg)
}
