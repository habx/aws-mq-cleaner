package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

func initLogger(logLevel string) (*zap.Logger, error) {
	// nolint:wrapcheck
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(func(level string) zapcore.Level {
		if level == "debug" {
			return zap.DebugLevel
		}
		return zap.InfoLevel
	}(logLevel))
	return config.Build()
}

// GetLogger returns a named logger.
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
