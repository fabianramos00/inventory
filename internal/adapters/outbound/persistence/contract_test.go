package persistence_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
	"inventory-desktop/tests/testdb"
)

// repositories is one backend wired to the port interfaces services consume.
type repositories struct {
	users      outbound.UserRepository
	tokens     outbound.RefreshTokenRepository
	categories outbound.CatalogRepository[domain.Category]
	brands     outbound.CatalogRepository[domain.Brand]
	materials  outbound.CatalogRepository[domain.Material]
	units      outbound.CatalogRepository[domain.MeasurementUnit]
	gorm       *gorm.DB
}

func TestRepositoryContract(t *testing.T) {
	backends := []struct {
		name  string
		setup func(*testing.T) *repositories
	}{
		{"sqlite", openSQLiteRepositories},
		{"postgres", openPostgresRepositories},
	}
	searchIDs := map[string]map[string][]int64{}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			repos := backend.setup(t)
			t.Run("users", func(t *testing.T) {
				testUsers(t, repos)
				testUserListAndDelete(t, repos)
			})
			t.Run("refresh tokens", func(t *testing.T) { testRefreshTokens(t, repos) })
			t.Run("catalog", func(t *testing.T) { testCatalog(t, repos, searchIDs, backend.name) })
		})
	}
	postgresIDs, postgresRan := searchIDs["postgres"]
	if !postgresRan {
		return
	}
	for query, want := range searchIDs["sqlite"] {
		if got := postgresIDs[query]; !slices.Equal(got, want) {
			t.Fatalf("search %q ids = %v on postgres, %v on sqlite", query, got, want)
		}
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
	return newRepositories(gormDB)
}

func newRepositories(gormDB *gorm.DB) *repositories {
	return &repositories{
		users:      gormstore.NewUserStore(gormDB),
		tokens:     gormstore.NewRefreshTokenStore(gormDB),
		categories: gormstore.NewCategoryStore(gormDB),
		brands:     gormstore.NewBrandStore(gormDB),
		materials:  gormstore.NewMaterialStore(gormDB),
		units:      gormstore.NewMeasurementUnitStore(gormDB),
		gorm:       gormDB,
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
	if _, err := conn.ExecContext(ctx, "TRUNCATE users, categories, materials, brands, measurement_units RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	return newRepositories(gormDB)
}

func testCatalog(t *testing.T, repos *repositories, searchIDs map[string]map[string][]int64, backend string) {
	t.Helper()
	ctx := context.Background()

	// Runs first so both freshly reset backends assign the same ids to these
	// rows; the parity check at the end compares those ids across backends.
	t.Run("literal wildcards", func(t *testing.T) {
		ids := map[string]int64{}
		for _, name := range []string{"100% cotton", "100 cotton", "a_b", "axb"} {
			record, err := repos.materials.Create(ctx, &domain.Material{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			ids[name] = record.ID
		}
		searchIDs[backend] = map[string][]int64{}

		for _, tt := range []struct{ name, query, want string }{
			{"percent", "%", "100% cotton"},
			{"underscore", "_", "a_b"},
		} {
			t.Run("search "+tt.name, func(t *testing.T) {
				items, hasNext, err := repos.materials.List(ctx, 0, 10, tt.query)
				if err != nil {
					t.Fatal(err)
				}
				if hasNext || len(items) != 1 || items[0].ID != ids[tt.want] {
					t.Fatalf("search %q = %d items has_next=%v, want %q only", tt.query, len(items), hasNext, tt.want)
				}
				searchIDs[backend][tt.query] = []int64{items[0].ID}
			})
		}
	})

	t.Run("category", func(t *testing.T) {
		code := "CAT-1"
		description := "Hand tools"
		created, err := repos.categories.Create(ctx, &domain.Category{Name: "Contract Category", Code: &code, Description: &description})
		if err != nil {
			t.Fatal(err)
		}
		if created.ID <= 0 {
			t.Fatalf("Create returned id %d, want a positive id", created.ID)
		}

		got, err := repos.categories.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertCategory(t, got, created)
		if _, err := repos.categories.Get(ctx, created.ID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get missing row error = %v, want ErrNotFound", err)
		}

		if _, err := repos.categories.Create(ctx, &domain.Category{Name: created.Name}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate name error = %v, want ErrConflict", err)
		}

		otherCode := "CAT-2"
		other, err := repos.categories.Create(ctx, &domain.Category{Name: "Contract Category Two", Code: &otherCode})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repos.categories.Create(ctx, &domain.Category{Name: "Contract Category Three", Code: &otherCode}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate code error = %v, want ErrConflict", err)
		}

		for _, query := range []string{"Hand tools", "CAT-2"} {
			items, hasNext, err := repos.categories.List(ctx, 0, 10, query)
			if err != nil {
				t.Fatal(err)
			}
			if hasNext || len(items) != 0 {
				t.Fatalf("search %q matched non-name fields: %+v", query, items)
			}
		}

		updatedDescription := "Updated description"
		if err := repos.categories.Update(ctx, created.ID, &domain.Category{Name: "Contract Category Updated", Description: &updatedDescription}); err != nil {
			t.Fatal(err)
		}
		got, err = repos.categories.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Contract Category Updated" || got.Code != nil || got.Description == nil || *got.Description != updatedDescription {
			t.Fatalf("updated category = %+v", got)
		}
		if err := repos.categories.Update(ctx, created.ID, &domain.Category{Name: got.Name, Code: got.Code, Description: got.Description}); err != nil {
			t.Fatalf("no-op update error = %v, want nil", err)
		}

		if err := repos.categories.Update(ctx, created.ID+1000, &domain.Category{Name: "Ghost"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing row error = %v, want ErrNotFound", err)
		}

		if _, err := repos.categories.Create(ctx, &domain.Category{Name: "Contract Category Four"}); err != nil {
			t.Fatal(err)
		}

		deleted, err := repos.categories.Delete(ctx, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertCategory(t, deleted, other)
		if _, err := repos.categories.Get(ctx, other.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
		}
		if _, err := repos.categories.Delete(ctx, other.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("second delete error = %v, want ErrNotFound", err)
		}
	})

	t.Run("brand", func(t *testing.T) {
		logo := "https://example.com/logo.png"
		created, err := repos.brands.Create(ctx, &domain.Brand{Name: "Contract Brand", LogoURL: &logo})
		if err != nil {
			t.Fatal(err)
		}
		got, err := repos.brands.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertBrand(t, got, created)

		if _, err := repos.brands.Create(ctx, &domain.Brand{Name: created.Name}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate name error = %v, want ErrConflict", err)
		}
		if err := repos.brands.Update(ctx, created.ID, &domain.Brand{Name: "Contract Brand Updated"}); err != nil {
			t.Fatal(err)
		}
		got, err = repos.brands.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Contract Brand Updated" || got.LogoURL != nil {
			t.Fatalf("updated brand = %+v", got)
		}
		if err := repos.brands.Update(ctx, created.ID+1000, &domain.Brand{Name: "Ghost"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing row error = %v, want ErrNotFound", err)
		}
		if _, err := repos.brands.Delete(ctx, created.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("material", func(t *testing.T) {
		created, err := repos.materials.Create(ctx, &domain.Material{Name: "Contract Material"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := repos.materials.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertMaterial(t, got, created)

		if _, err := repos.materials.Create(ctx, &domain.Material{Name: created.Name}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate name error = %v, want ErrConflict", err)
		}
		if err := repos.materials.Update(ctx, created.ID, &domain.Material{Name: "Contract Material Updated"}); err != nil {
			t.Fatal(err)
		}
		got, err = repos.materials.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Contract Material Updated" {
			t.Fatalf("updated material = %+v", got)
		}
		if _, err := repos.materials.Delete(ctx, created.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("measurement unit", func(t *testing.T) {
		created, err := repos.units.Create(ctx, &domain.MeasurementUnit{Name: "Contract Unit", Abbreviation: "cu"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := repos.units.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertMeasurementUnit(t, got, created)

		if _, err := repos.units.Create(ctx, &domain.MeasurementUnit{Name: created.Name, Abbreviation: "xx"}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate name error = %v, want ErrConflict", err)
		}
		if _, err := repos.units.Create(ctx, &domain.MeasurementUnit{Name: "Contract Unit Two", Abbreviation: created.Abbreviation}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate abbreviation error = %v, want ErrConflict", err)
		}
		if err := repos.units.Update(ctx, created.ID, &domain.MeasurementUnit{Name: "Contract Unit Updated", Abbreviation: "cu2"}); err != nil {
			t.Fatal(err)
		}
		got, err = repos.units.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Contract Unit Updated" || got.Abbreviation != "cu2" {
			t.Fatalf("updated unit = %+v", got)
		}
		if _, err := repos.units.Delete(ctx, created.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("delete referenced", func(t *testing.T) { testCatalogDeleteForeignKey(t, repos) })
	t.Run("pagination search and order", func(t *testing.T) {
		names := []string{"Paging Alpha", "Paging Beta", "Paging Gamma", "Paging Delta"}
		for _, name := range names {
			if _, err := repos.materials.Create(ctx, &domain.Material{Name: name}); err != nil {
				t.Fatal(err)
			}
		}

		all, hasNext, err := repos.materials.List(ctx, 0, 100, "paging")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(all) != len(names) {
			t.Fatalf("search paging = %d items has_next=%v, want %d and false", len(all), hasNext, len(names))
		}
		for i := 1; i < len(all); i++ {
			if all[i-1].ID <= all[i].ID {
				t.Fatalf("search results are not ordered id DESC: %d then %d", all[i-1].ID, all[i].ID)
			}
		}

		page, hasNext, err := repos.materials.List(ctx, 0, 2, "paging")
		if err != nil {
			t.Fatal(err)
		}
		if !hasNext || len(page) != 2 || page[0].ID != all[0].ID || page[1].ID != all[1].ID {
			t.Fatalf("first page = %+v has_next=%v, want %d/%d and true", page, hasNext, all[0].ID, all[1].ID)
		}

		last, hasNext, err := repos.materials.List(ctx, 2, 2, "paging")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(last) != 2 || last[0].ID != all[2].ID || last[1].ID != all[3].ID {
			t.Fatalf("last page = %+v has_next=%v, want %d/%d and false", last, hasNext, all[2].ID, all[3].ID)
		}

		past, hasNext, err := repos.materials.List(ctx, 4, 2, "paging")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(past) != 0 {
			t.Fatalf("past-the-end page = %d items has_next=%v, want 0 and false", len(past), hasNext)
		}

		accented, err := repos.materials.Create(ctx, &domain.Material{Name: "Árbol de Pino"})
		if err != nil {
			t.Fatal(err)
		}
		for _, term := range []string{"árbol", "ÁRBOL"} {
			items, hasNext, err := repos.materials.List(ctx, 0, 10, term)
			if err != nil {
				t.Fatal(err)
			}
			if hasNext || len(items) != 1 || items[0].ID != accented.ID {
				t.Fatalf("search %q = %+v has_next=%v, want the accented row", term, items, hasNext)
			}
		}
		none, hasNext, err := repos.materials.List(ctx, 0, 10, "nobody-matches-this")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(none) != 0 {
			t.Fatalf("no-match search = %d items has_next=%v, want 0 and false", len(none), hasNext)
		}
	})
}
func testCatalogDeleteForeignKey(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()

	category, err := repos.categories.Create(ctx, &domain.Category{Name: "Referenced Category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := repos.brands.Create(ctx, &domain.Brand{Name: "Referenced Brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := repos.materials.Create(ctx, &domain.Material{Name: "Referenced Material"})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := repos.units.Create(ctx, &domain.MeasurementUnit{Name: "Referenced Unit", Abbreviation: "ru"})
	if err != nil {
		t.Fatal(err)
	}

	const sku = "CONTRACT-CATALOG-1"
	if err := repos.gorm.WithContext(ctx).Exec(
		"INSERT INTO products (sku, name, price, cost, stock_quantity, low_stock_threshold, brand_id, category_id, material_id, measurement_unit_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		sku, "Referenced Product", 100, 50, 1, 1, brand.ID, category.ID, material.ID, unit.ID,
	).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = repos.gorm.WithContext(context.Background()).Exec("DELETE FROM products WHERE sku = ?", sku).Error
	})

	if _, err := repos.categories.Delete(ctx, category.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
		t.Fatalf("delete referenced category error = %v, want ErrForeignKeyViolation", err)
	}
	if _, err := repos.brands.Delete(ctx, brand.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
		t.Fatalf("delete referenced brand error = %v, want ErrForeignKeyViolation", err)
	}
	if _, err := repos.materials.Delete(ctx, material.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
		t.Fatalf("delete referenced material error = %v, want ErrForeignKeyViolation", err)
	}
	if _, err := repos.units.Delete(ctx, unit.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
		t.Fatalf("delete referenced unit error = %v, want ErrForeignKeyViolation", err)
	}

	if _, err := repos.categories.Get(ctx, category.ID); err != nil {
		t.Fatalf("referenced category must survive the failed delete: %v", err)
	}
}

func assertCategory(t *testing.T, got, want *domain.Category) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name || !equalStringPtr(got.Code, want.Code) || !equalStringPtr(got.Description, want.Description) {
		t.Fatalf("category = %+v, want %+v", got, want)
	}
}

func assertBrand(t *testing.T, got, want *domain.Brand) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name || !equalStringPtr(got.LogoURL, want.LogoURL) {
		t.Fatalf("brand = %+v, want %+v", got, want)
	}
}

func assertMaterial(t *testing.T, got, want *domain.Material) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name {
		t.Fatalf("material = %+v, want %+v", got, want)
	}
}

func assertMeasurementUnit(t *testing.T, got, want *domain.MeasurementUnit) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name || got.Abbreviation != want.Abbreviation {
		t.Fatalf("measurement unit = %+v, want %+v", got, want)
	}
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
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
