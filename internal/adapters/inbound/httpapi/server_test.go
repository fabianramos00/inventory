package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"inventory-desktop/internal/adapters/inbound/httpapi"
)

func TestNewServerRejectsNonLoopbackAddresses(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	addresses := []string{"0.0.0.0:8787", ":8787", "[::]:8787", "192.0.2.1:8787", "not-an-address"}
	for _, addr := range addresses {
		t.Run(addr, func(t *testing.T) {
			if _, err := httpapi.NewServer(addr, http.NotFoundHandler(), logger); err == nil {
				t.Fatalf("NewServer(%q) accepted a non-loopback address", addr)
			}
		})
	}
}

func TestNewServerAcceptsLoopbackEphemeralAddress(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server, err := httpapi.NewServer("127.0.0.1:0", http.NotFoundHandler(), logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	}()

	host, _, err := net.SplitHostPort(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Fatalf("server bound to non-loopback address %q", server.Addr())
	}
	if got, want := server.BaseURL(), "http://"+server.Addr()+"/api/v1"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
}
