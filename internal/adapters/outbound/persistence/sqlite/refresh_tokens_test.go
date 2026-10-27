package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
)

// TestRefreshTokenExpiryRoundTrip covers the SQLite read path after the driver
// normalizes stored timestamps. modernc returns TEXT timestamps through
// time.RFC3339Nano, which trims zero fractions, so the adapter must accept
// whole-second and trailing-zero values instead of requiring nine digits.
func TestRefreshTokenExpiryRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "inventory.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}

	users := NewUserRepository(conn)
	tokens := NewRefreshTokenRepository(conn)
	user := &domain.User{FullName: "Expiry User", Email: "expiry@sqlite.test", HashedPassword: "hash", IsActive: true}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		nanos int
	}{
		{"zero nanos", 0},
		{"trailing-zero nanos", 123456780},
		{"nonzero nanos", 123456789},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := base
			expiresAt := base.Add(24 * time.Hour).Add(time.Duration(tc.nanos))
			tokenHash := fmt.Sprintf("token-%d", tc.nanos)

			if err := tokens.Create(ctx, user.ID, tokenHash, expiresAt); err != nil {
				t.Fatal(err)
			}

			// Simulate the driver's RFC3339Nano normalization: trailing zero
			// fractions disappear, and a zero fraction disappears entirely.
			normalized := expiresAt.Format(time.RFC3339Nano)
			if _, err := conn.ExecContext(ctx,
				"UPDATE refresh_tokens SET expires_at = ? WHERE token_hash = ?",
				normalized, tokenHash,
			); err != nil {
				t.Fatal(err)
			}

			found, err := tokens.FindValid(ctx, tokenHash, now)
			if err != nil {
				t.Fatalf("FindValid(%q): %v", normalized, err)
			}
			if found.UserID != user.ID {
				t.Fatalf("user id = %d, want %d", found.UserID, user.ID)
			}
			if !found.ExpiresAt.Equal(expiresAt) {
				t.Fatalf("expires_at = %s, want %s", found.ExpiresAt, expiresAt)
			}

			if _, err := tokens.FindValid(ctx, tokenHash, expiresAt.Add(time.Second)); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("expired token error = %v, want ErrNotFound", err)
			}

			rotatedHash := fmt.Sprintf("rotated-%d", tc.nanos)
			rotatedExpiry := base.Add(48 * time.Hour).Add(time.Duration(tc.nanos))
			if err := tokens.Rotate(ctx, tokenHash, rotatedHash, now, rotatedExpiry); err != nil {
				t.Fatalf("Rotate: %v", err)
			}
			if _, err := tokens.FindValid(ctx, tokenHash, now); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("old token after rotation error = %v, want ErrNotFound", err)
			}
			rotated, err := tokens.FindValid(ctx, rotatedHash, now)
			if err != nil {
				t.Fatalf("FindValid(rotated): %v", err)
			}
			if !rotated.ExpiresAt.Equal(rotatedExpiry) {
				t.Fatalf("rotated expires_at = %s, want %s", rotated.ExpiresAt, rotatedExpiry)
			}
		})
	}
}
