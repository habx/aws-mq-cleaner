package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
)

func initLogger(logLevel string) (*zap.Logger, error) {
	return zap.Config{
		Level: zap.NewAtomicLevelAt(func(level string) zapcore.Level {
			if level == "debug" {
				return zap.DebugLevel
			}
			return zap.InfoLevel
		}(logLevel)),
		Encoding:    "console",
		OutputPaths: []string{"stdout"},
		EncoderConfig: func(level string) zapcore.EncoderConfig {
			if level == "debug" {
				return zapcore.EncoderConfig{
					MessageKey:   "message", // <---
					LevelKey:     "level",
					EncodeLevel:  zapcore.CapitalLevelEncoder,
					TimeKey:      "time",
					EncodeTime:   zapcore.ISO8601TimeEncoder,
					CallerKey:    "caller",
					EncodeCaller: zapcore.ShortCallerEncoder,
				}
			}
			return zapcore.EncoderConfig{
				MessageKey: "message", // <---
			}
		}(logLevel),
	}.Build()
}

// GetLogger returns a named logger
func GetLogger(logLevel string) *zap.Logger {
	if Logger == nil {
		l, err := initLogger(logLevel)
		if err != nil {
			log.Fatal("Cannot load logger")
		}
		return l
	}
	return Logger
}
