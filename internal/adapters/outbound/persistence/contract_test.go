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
			t.Run("users", func(t *testing.T) {
				testUsers(t, repos)
				testUserListAndDelete(t, repos)
			})
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

func testUserListAndDelete(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()

	grace := createUser(t, repos, "grace@contract.test")
	grace.FullName = "Grace Hopper"
	if err := repos.users.Update(ctx, grace); err != nil {
		t.Fatal(err)
	}
	linus := createUser(t, repos, "linus@contract.test")
	barbara := createUser(t, repos, "barbara@contract.test")
	jose := createUser(t, repos, "jose@contract.test")
	jose.FullName = "José Pérez"
	if err := repos.users.Update(ctx, jose); err != nil {
		t.Fatal(err)
	}
	// Ada already exists from testUsers with a lower id.
	all := []*domain.User{grace, linus, barbara, jose}

	ordered, hasNext, err := repos.users.List(ctx, 0, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if hasNext {
		t.Fatal("full list has_next = true, want false")
	}
	if len(ordered) < len(all)+1 {
		t.Fatalf("full list returned %d users, want at least %d", len(ordered), len(all)+1)
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].ID >= ordered[i].ID {
			t.Fatalf("list is not ordered by id: %d then %d", ordered[i-1].ID, ordered[i].ID)
		}
	}

	page, hasNext, err := repos.users.List(ctx, 1, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if !hasNext {
		t.Fatal("limited list has_next = false, want true")
	}
	if len(page) != 2 {
		t.Fatalf("limited list returned %d users, want 2", len(page))
	}
	if page[0].ID != ordered[1].ID || page[1].ID != ordered[2].ID {
		t.Fatalf("page ids = [%d %d], want [%d %d]", page[0].ID, page[1].ID, ordered[1].ID, ordered[2].ID)
	}

	lastPage, hasNext, err := repos.users.List(ctx, len(ordered), 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if hasNext || len(lastPage) != 0 {
		t.Fatalf("past-the-end list = %d items has_next=%v, want 0 and false", len(lastPage), hasNext)
	}

	searches := []struct {
		name  string
		query string
		want  *domain.User
	}{
		{name: "lowercase name", query: "grace", want: grace},
		{name: "uppercase name", query: "HOPPER", want: grace},
		{name: "lowercase email", query: "linus@contract", want: linus},
		{name: "uppercase email", query: "LINUS@CONTRACT.TEST", want: linus},
		{name: "lowercase accented name", query: "josé", want: jose},
		{name: "uppercase accented name", query: "JOSÉ PÉREZ", want: jose},
	}
	for _, tt := range searches {
		t.Run("search "+tt.name, func(t *testing.T) {
			items, hasNext, err := repos.users.List(ctx, 0, 10, tt.query)
			if err != nil {
				t.Fatal(err)
			}
			if hasNext || len(items) != 1 {
				t.Fatalf("search %q = %d items has_next=%v, want 1 and false", tt.query, len(items), hasNext)
			}
			assertUser(t, items[0], tt.want)
		})
	}

	none, hasNext, err := repos.users.List(ctx, 0, 10, "nobody-matches-this")
	if err != nil {
		t.Fatal(err)
	}
	if hasNext || len(none) != 0 {
		t.Fatalf("no-match search = %d items has_next=%v, want 0 and false", len(none), hasNext)
	}

	duplicate := &domain.User{
		FullName:       "Grace Copy",
		Email:          grace.Email,
		HashedPassword: "hash-copy",
		IsActive:       true,
	}
	if err := repos.users.Create(ctx, duplicate); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate email error = %v, want ErrConflict", err)
	}

	first := createUser(t, repos, "update-conflict-a@contract.test")
	second := createUser(t, repos, "update-conflict-b@contract.test")
	second.Email = first.Email
	if err := repos.users.Update(ctx, second); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("update to duplicate email error = %v, want ErrConflict", err)
	}

	deleted, err := repos.users.Delete(ctx, grace.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertUser(t, deleted, grace)
	if _, err := repos.users.GetByID(ctx, grace.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID after delete error = %v, want ErrNotFound", err)
	}
	if _, err := repos.users.Delete(ctx, grace.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
	if _, err := repos.users.Delete(ctx, grace.ID+1000); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing row error = %v, want ErrNotFound", err)
	}
}

// TestPostgresUserDeleteForeignKeyViolation covers the adapter-specific FK
// mapping. SQLite's offline schema has no non-cascading reference to users, so
// only PostgreSQL can exercise it.
func TestPostgresUserDeleteForeignKeyViolation(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the postgres FK test")
	}

	ctx := context.Background()
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	gormDB, conn, err := db.OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "TRUNCATE refresh_tokens, providers, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}

	users := gormstore.NewUserStore(gormDB)
	user := createUser(t, &repositories{users: users}, "referenced@contract.test")

	if _, err := conn.ExecContext(ctx,
		"INSERT INTO providers (name, created_by_id) VALUES ($1, $2)",
		"Proveedor con usuario", user.ID,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), "DELETE FROM providers WHERE created_by_id = $1", user.ID)
	})

	if _, err := users.Delete(ctx, user.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
		t.Fatalf("delete referenced user error = %v, want ErrForeignKeyViolation", err)
	}
	if _, err := users.GetByID(ctx, user.ID); err != nil {
		t.Fatalf("referenced user must survive the failed delete: %v", err)
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
	if ownerToken != owner.ID {
		t.Fatalf("owner token user = %d, want %d", ownerToken, owner.ID)
	}

	const otherHash = "other-token-hash"
	if err := repos.tokens.Create(ctx, other.ID, otherHash, ownerExpiry); err != nil {
		t.Fatal(err)
	}
	otherToken, err := repos.tokens.FindValid(ctx, otherHash, now)
	if err != nil {
		t.Fatal(err)
	}
	if otherToken != other.ID {
		t.Fatalf("other token user = %d, want %d", otherToken, other.ID)
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
	if rotated != owner.ID {
		t.Fatalf("rotated token user = %d, want %d", rotated, owner.ID)
	}

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

func assertUser(t *testing.T, got, want *domain.User) {
	t.Helper()
	if got.ID != want.ID || got.FullName != want.FullName || got.Email != want.Email ||
		got.HashedPassword != want.HashedPassword || got.IsActive != want.IsActive ||
		got.IsSuperuser != want.IsSuperuser {
		t.Fatalf("user = %+v, want %+v", got, want)
	}
}
