package gormstore

import (
	"database/sql/driver"
	"fmt"
	"time"
)

const timestampLayout = "2006-01-02T15:04:05.000000000Z"

// Timestamp keeps one UTC instant in a format PostgreSQL and SQLite compare
// identically. SQLite stores TEXT, so the fixed width is what makes
// lexicographic expiry checks chronological.
type Timestamp time.Time

func (t Timestamp) Value() (driver.Value, error) {
	return time.Time(t).UTC().Format(timestampLayout), nil
}

func (t *Timestamp) Scan(value any) error {
	switch v := value.(type) {
	case time.Time:
		*t = Timestamp(v.UTC())
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	case nil:
		*t = Timestamp(time.Time{})
	default:
		return fmt.Errorf("scan timestamp: unsupported type %T", value)
	}
	return nil
}

func (t Timestamp) Time() time.Time {
	return time.Time(t)
}

func (t *Timestamp) parse(value string) error {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	*t = Timestamp(parsed.UTC())
	return nil
}
