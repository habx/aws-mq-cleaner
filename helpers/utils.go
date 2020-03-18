package helpers

import (
	"os"
	"regexp"

	"github.com/habx/aws-mq-cleaner/flags"
)

func GetEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func InitExcludePattern(expr string) (*regexp.Regexp, error) {
	if expr == "" {
		return nil, nil
	}
	return regexp.Compile(flags.ExcludePatten)
}
