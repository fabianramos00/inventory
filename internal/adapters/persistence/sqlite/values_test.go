package sqlite

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
			formatted := formatTimestamp(want)
			if len(formatted) != len("2006-01-02T15:04:05.000000000Z") {
				t.Fatalf("formatted timestamp %q is not fixed width", formatted)
			}
			got, err := parseTimestamp(formatted)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(want) {
				t.Fatalf("parseTimestamp(%q) = %s, want %s", formatted, got, want)
			}
		})
	}
}
