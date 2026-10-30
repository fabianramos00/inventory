package application

import "time"

// PostgreSQL timestamptz keeps microseconds; truncating keeps the write
// response and a later read identical on both backends.
func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
