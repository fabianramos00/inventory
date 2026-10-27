package persistence_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
	"inventory-desktop/tests/testdb"
)

// repositories is one backend wired to the port interfaces services consume.
type repositories struct {
	users  outbound.UserRepository
	tokens outbound.RefreshTokenRepository
}

func TestRepositoryContract(t *testing.T) {
	backends := []struct {
		name  string
		setup func(*testing.T) *repositories
	}{
		{"sqlite", openSQLiteRepositories},
		{"postgres", openPostgresRepositories},
	}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			repos := backend.setup(t)
			t.Run("users", func(t *testing.T) { testUsers(t, repos) })
			t.Run("refresh tokens", func(t *testing.T) { testRefreshTokens(t, repos) })
		})
	}
}

func openSQLiteRepositories(t *testing.T) *repositories {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	gormDB, conn, err := db.OpenSQLiteGORM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}
	return &repositories{
		users:  gormstore.NewUserStore(gormDB),
		tokens: gormstore.NewRefreshTokenStore(gormDB),
	}
}

func openPostgresRepositories(t *testing.T) *repositories {
	t.Helper()
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the postgres contract suite")
	}

	ctx := context.Background()
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	gormDB, conn, err := db.OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "TRUNCATE refresh_tokens, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	return &repositories{
		users:  gormstore.NewUserStore(gormDB),
		tokens: gormstore.NewRefreshTokenStore(gormDB),
	}
}

func testUsers(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()

	user := &domain.User{
		FullName:       "Ada Lovelace",
		Email:          "ada@contract.test",
		HashedPassword: "hash-ada",
		IsActive:       true,
		IsSuperuser:    false,
	}
	if err := repos.users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if user.ID <= 0 {
		t.Fatalf("Create returned id %d, want a positive id", user.ID)
	}

	byID, err := repos.users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertUser(t, byID, user)

	byEmail, err := repos.users.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	assertUser(t, byEmail, user)

	if _, err := repos.users.GetByID(ctx, user.ID+1000); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID missing row error = %v, want ErrNotFound", err)
	}
	if _, err := repos.users.GetByEmail(ctx, "missing@contract.test"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByEmail missing row error = %v, want ErrNotFound", err)
	}

	user.FullName = "Ada Updated"
	user.Email = "ada.updated@contract.test"
	user.HashedPassword = "hash-ada-updated"
	user.IsActive = false
	user.IsSuperuser = true
	if err := repos.users.Update(ctx, user); err != nil {
		t.Fatal(err)
	}
	updated, err := repos.users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertUser(t, updated, user)

	missing := &domain.User{ID: user.ID + 1000, FullName: "Ghost", Email: "ghost@contract.test", HashedPassword: "hash"}
	if err := repos.users.Update(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update missing row error = %v, want ErrNotFound", err)
	}
}

func testRefreshTokens(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()
	// Fixed, sub-second timestamp keeps the suite deterministic. Nanoseconds are
	// deliberately non-zero to exercise fractional-second storage.
	now := time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC)

	owner := createUser(t, repos, "owner@contract.test")
	other := createUser(t, repos, "other@contract.test")

	const ownerHash = "owner-token-hash"
	ownerExpiry := now.Add(time.Hour)
	if err := repos.tokens.Create(ctx, owner.ID, ownerHash, ownerExpiry); err != nil {
		t.Fatal(err)
	}
	ownerToken, err := repos.tokens.FindValid(ctx, ownerHash, now)
	if err != nil {
		t.Fatal(err)
	}
	if ownerToken.UserID != owner.ID {
		t.Fatalf("owner token user = %d, want %d", ownerToken.UserID, owner.ID)
	}
	if ownerToken.TokenHash != ownerHash {
		t.Fatalf("token hash = %q, want %q", ownerToken.TokenHash, ownerHash)
	}
	if ownerToken.Revoked {
		t.Fatal("new token is marked revoked")
	}
	assertInstant(t, ownerToken.ExpiresAt, ownerExpiry)

	const otherHash = "other-token-hash"
	if err := repos.tokens.Create(ctx, other.ID, otherHash, ownerExpiry); err != nil {
		t.Fatal(err)
	}
	otherToken, err := repos.tokens.FindValid(ctx, otherHash, now)
	if err != nil {
		t.Fatal(err)
	}
	if otherToken.UserID != other.ID {
		t.Fatalf("other token user = %d, want %d", otherToken.UserID, other.ID)
	}

	const rotatedHash = "owner-rotated-hash"
	rotatedExpiry := now.Add(2 * time.Hour)
	if err := repos.tokens.Rotate(ctx, ownerHash, rotatedHash, now, rotatedExpiry); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.tokens.FindValid(ctx, ownerHash, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("old token after rotation error = %v, want ErrNotFound", err)
	}
	rotated, err := repos.tokens.FindValid(ctx, rotatedHash, now)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.UserID != owner.ID {
		t.Fatalf("rotated token user = %d, want %d", rotated.UserID, owner.ID)
	}
	assertInstant(t, rotated.ExpiresAt, rotatedExpiry)

	if err := repos.tokens.Rotate(ctx, ownerHash, "unused-hash", now, rotatedExpiry); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotating an already used token error = %v, want ErrNotFound", err)
	}

	if err := repos.tokens.Revoke(ctx, rotatedHash); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.tokens.FindValid(ctx, rotatedHash, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked token error = %v, want ErrNotFound", err)
	}
	if err := repos.tokens.Revoke(ctx, rotatedHash); err != nil {
		t.Fatalf("repeated revoke error = %v, want nil", err)
	}

	const expiredHash = "expired-token-hash"
	if err := repos.tokens.Create(ctx, owner.ID, expiredHash, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.tokens.FindValid(ctx, expiredHash, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired token error = %v, want ErrNotFound", err)
	}
	if err := repos.tokens.Rotate(ctx, expiredHash, "expired-rotated", now, rotatedExpiry); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotating an expired token error = %v, want ErrNotFound", err)
	}

	if err := repos.tokens.Revoke(ctx, expiredHash); err != nil {
		t.Fatalf("revoking an expired token error = %v, want nil", err)
	}
}

func createUser(t *testing.T, repos *repositories, email string) *domain.User {
	t.Helper()
	user := &domain.User{FullName: "Contract User", Email: email, HashedPassword: "hash", IsActive: true}
	if err := repos.users.Create(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

// assertInstant allows for the microsecond precision PostgreSQL stores; SQLite
// keeps the full nanosecond value.
func assertInstant(t *testing.T, got, want time.Time) {
	t.Helper()
	if delta := got.Sub(want); delta < -time.Microsecond || delta > time.Microsecond {
		t.Fatalf("expires_at = %s, want %s", got, want)
	}
}

func assertUser(t *testing.T, got, want *domain.User) {
	t.Helper()
	if got.ID != want.ID || got.FullName != want.FullName || got.Email != want.Email ||
		got.HashedPassword != want.HashedPassword || got.IsActive != want.IsActive ||
		got.IsSuperuser != want.IsSuperuser {
		t.Fatalf("user = %+v, want %+v", got, want)
	}
}
