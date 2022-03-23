package helpers

import (
	"fmt"
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
		// nolint:nilnil
		return nil, nil
	}
	regexComplied, err := regexp.Compile(flags.ExcludePatten)
	if err != nil {
		return nil, fmt.Errorf("cannot compile this regex %s : %w", expr, err)
	}
	return regexComplied, nil
}
