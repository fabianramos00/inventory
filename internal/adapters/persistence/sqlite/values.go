package sqlite

import (
	"fmt"
	"time"
)

const timestampLayout = "2006-01-02T15:04:05.000000000Z"

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(timestampLayout)
}

func parseTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	return parsed.UTC(), nil
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
