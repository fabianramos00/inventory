package testdb

import (
	"strings"
	"testing"
)

func TestValidateTestDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr string
	}{
		{
			name: "test database is accepted",
			dsn:  "postgresql://inventory:inventory@localhost:5433/inventory_desktop_test",
		},
		{
			name:    "development database is rejected",
			dsn:     "postgresql://inventory:inventory@localhost:5433/inventory_desktop",
			wantErr: "inventory_desktop",
		},
		{
			name:    "malformed DSN is rejected",
			dsn:     "this is not a dsn",
			wantErr: "invalid test database DSN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTestDSN(tt.dsn)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTestDSN(%q) = %v, want nil", tt.dsn, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTestDSN(%q) = nil, want error containing %q", tt.dsn, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateTestDSN(%q) = %q, want error containing %q", tt.dsn, err.Error(), tt.wantErr)
			}
			if tt.wantErr == "inventory_desktop" && !strings.Contains(err.Error(), "_test") {
				t.Fatalf("ValidateTestDSN(%q) = %q, want error mentioning the \"_test\" suffix", tt.dsn, err.Error())
			}
		})
	}
}
