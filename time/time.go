package time

import (
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

const (
	dayHour  = 24
	matchLen = 3
)

var durationRegex = regexp.MustCompile(`^(\d+)([smhd])$`)

// ParseDuration parses a duration string and returns the time.Duration.
func ParseDuration(duration string) (*time.Duration, error) {
	const base = 10
	const bitSize = 10
	matches := durationRegex.FindStringSubmatch(duration)
	if len(matches) != matchLen {
		return nil, errors.Errorf("invalid since format '%s'. expected format <duration><unit> (e.g. 3h)", duration)
	}
	amount, err := strconv.ParseInt(matches[1], base, bitSize)
	if err != nil {
		log.Fatal(err)
	}
	var unit time.Duration
	switch matches[2] {
	case "s":
		unit = time.Second
	case "m":
		unit = time.Minute
	case "h":
		unit = time.Hour
	case "d":
		unit = time.Hour * dayHour
	}
	dur := unit * time.Duration(amount)
	return &dur, nil
}

// ParseSince parses a duration string and returns a time.Time in history relative to current time.
func ParseSince(duration string) (*time.Time, error) {
	dur, err := ParseDuration(duration)
	if err != nil {
		return nil, err
	}
	since := time.Now().UTC().Add(-*dur)
	return &since, nil
}
