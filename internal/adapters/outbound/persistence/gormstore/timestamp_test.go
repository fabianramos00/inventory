package gormstore

import (
	"testing"
	"time"
)

func TestTimestampRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		nanos int
	}{
		{"zero nanos", 0},
		{"trailing zero nanos", 123456780},
		{"full nanos", 123456789},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := time.Date(2026, time.October, 26, 12, 34, 56, tc.nanos, time.UTC)
			value, err := Timestamp(want).Value()
			if err != nil {
				t.Fatal(err)
			}
			formatted, ok := value.(string)
			if !ok {
				t.Fatalf("Value returned %T, want string", value)
			}
			if len(formatted) != len(timestampLayout) {
				t.Fatalf("formatted timestamp %q is not fixed width", formatted)
			}

			var got Timestamp
			if err := got.Scan(formatted); err != nil {
				t.Fatal(err)
			}
			if !got.Time().Equal(want) {
				t.Fatalf("Scan(%q) = %s, want %s", formatted, got.Time(), want)
			}
		})
	}
}

// TestTimestampScanNormalizedStrings covers the shapes SQLite returns when a
// stored timestamp loses its trailing zero fraction or its fraction entirely.
func TestTimestampScanNormalizedStrings(t *testing.T) {
	cases := []struct {
		name  string
		nanos int
	}{
		{"zero nanos", 0},
		{"trailing zero nanos", 123456780},
		{"full nanos", 123456789},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := time.Date(2026, time.October, 26, 12, 34, 56, tc.nanos, time.UTC)
			var got Timestamp
			if err := got.Scan(want.Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			if !got.Time().Equal(want) {
				t.Fatalf("Scan = %s, want %s", got.Time(), want)
			}
		})
	}
}
