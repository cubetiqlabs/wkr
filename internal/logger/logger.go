package logger

import (
	"github.com/cubetiqlabs/wkr/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var L *zap.Logger
var S *zap.SugaredLogger

func Init(cfg config.LogConfig) error {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config
	if cfg.Format == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	var err error
	L, err = zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return err
	}
	S = L.Sugar()
	return nil
}

func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}

// Convenience wrappers
func Info(msg string, fields ...zap.Field)  { L.Info(msg, fields...) }
func Error(msg string, fields ...zap.Field) { L.Error(msg, fields...) }
func Debug(msg string, fields ...zap.Field) { L.Debug(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { L.Warn(msg, fields...) }
func Fatal(msg string, fields ...zap.Field) { L.Fatal(msg, fields...) }
