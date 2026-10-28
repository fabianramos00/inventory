package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"inventory-desktop/internal/adapters/inbound/httpapi"
)

func TestHealthzReportsBackend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		name         string
		handler      http.Handler
		wantDatabase string
	}{
		{
			name:         "explicit backend",
			handler:      httpapi.NewRouter(httpapi.Deps{Logger: logger, Backend: "sqlite"}),
			wantDatabase: "sqlite",
		},
		{
			name:         "no backend keeps the fallback shape",
			handler:      httpapi.NewRouter(httpapi.Deps{Logger: logger}),
			wantDatabase: "unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			response, err := http.Get(server.URL + "/healthz")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()

			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.StatusCode)
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["status"] != "ok" {
				t.Fatalf("status field = %q, want ok", body["status"])
			}
			if body["database"] != tc.wantDatabase {
				t.Fatalf("database field = %q, want %q", body["database"], tc.wantDatabase)
			}
		})
	}
}
