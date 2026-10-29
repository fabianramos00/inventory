package persistence_test

import (
	"context"
	"errors"
	"testing"

	"inventory-desktop/internal/domain"
)

func testProviders(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()
	str := func(value string) *string { return &value }

	t.Run("create get update and no-op", func(t *testing.T) {
		provider, err := repos.providers.Create(ctx, &domain.Provider{
			Name:        "Distribuidora Metálica",
			ContactInfo: str("Juan Pérez"),
			Email:       str("ventas@example.com"),
			Phone:       str("3001234567"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if provider.ID <= 0 {
			t.Fatalf("Create returned id %d, want positive", provider.ID)
		}
		got, err := repos.providers.Get(ctx, provider.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertProvider(t, got, provider)
		if _, err := repos.providers.Get(ctx, provider.ID+1000); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get missing error = %v, want ErrNotFound", err)
		}

		duplicate, err := repos.providers.Create(ctx, &domain.Provider{Name: provider.Name})
		if err != nil {
			t.Fatalf("duplicate provider name must be allowed: %v", err)
		}

		provider.Name = "Distribuidora Actualizada"
		provider.ContactInfo = nil
		provider.Email = str("nuevo@example.com")
		provider.Phone = nil
		if err := repos.providers.Update(ctx, provider.ID, provider); err != nil {
			t.Fatal(err)
		}
		got, err = repos.providers.Get(ctx, provider.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != provider.Name || got.ContactInfo != nil || got.Phone != nil ||
			got.Email == nil || *got.Email != "nuevo@example.com" {
			t.Fatalf("updated provider = %+v", got)
		}

		if err := repos.providers.Update(ctx, provider.ID, got); err != nil {
			t.Fatalf("no-op update error = %v, want nil", err)
		}
		if err := repos.providers.Update(ctx, provider.ID+1000, got); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing error = %v, want ErrNotFound", err)
		}

		deleted, err := repos.providers.Delete(ctx, duplicate.ID)
		if err != nil {
			t.Fatal(err)
		}
		if deleted.ID != duplicate.ID || deleted.Name != duplicate.Name {
			t.Fatalf("deleted = %+v, want %+v", deleted, duplicate)
		}
		if _, err := repos.providers.Get(ctx, duplicate.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
		}
		if _, err := repos.providers.Delete(ctx, duplicate.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("second delete error = %v, want ErrNotFound", err)
		}
	})

	t.Run("search fields accents and wildcards", func(t *testing.T) {
		provider, err := repos.providers.Create(ctx, &domain.Provider{
			Name:        "Proveedor Árbol",
			ContactInfo: str("SoloContactoNoBuscado"),
			Email:       str("Arbol@Example.com"),
			Phone:       str("+57 3001"),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{"proveedor árbol", "PROVEEDOR ÁRBOL", "arbol@example", "3001"} {
			items, hasNext, err := repos.providers.List(ctx, 0, 100, query)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range items {
				if item.ID == provider.ID {
					found = true
				}
			}
			if hasNext || !found {
				t.Fatalf("search %q = %+v has_next=%v, want provider %d", query, items, hasNext, provider.ID)
			}
		}
		items, hasNext, err := repos.providers.List(ctx, 0, 100, "SoloContactoNoBuscado")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(items) != 0 {
			t.Fatalf("contact_info must not be searched: %+v", items)
		}

		percent, err := repos.providers.Create(ctx, &domain.Provider{Name: "100% Proveedor"})
		if err != nil {
			t.Fatal(err)
		}
		underscore, err := repos.providers.Create(ctx, &domain.Provider{Name: "a_b Proveedor"})
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct {
			query string
			want  int64
		}{{"%", percent.ID}, {"_", underscore.ID}} {
			items, hasNext, err := repos.providers.List(ctx, 0, 100, tt.query)
			if err != nil {
				t.Fatal(err)
			}
			if hasNext || len(items) != 1 || items[0].ID != tt.want {
				t.Fatalf("search %q = %+v has_next=%v, want only id %d", tt.query, items, hasNext, tt.want)
			}
		}
	})

	t.Run("pagination and order", func(t *testing.T) {
		ids := make([]int64, 0, 4)
		for _, name := range []string{"Paging Provider Alpha", "Paging Provider Beta", "Paging Provider Gamma", "Paging Provider Delta"} {
			provider, err := repos.providers.Create(ctx, &domain.Provider{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, provider.ID)
		}

		all, hasNext, err := repos.providers.List(ctx, 0, 100, "Paging Provider")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(all) != len(ids) {
			t.Fatalf("all = %d items has_next=%v, want %d and false", len(all), hasNext, len(ids))
		}
		for i := 1; i < len(all); i++ {
			if all[i-1].ID <= all[i].ID {
				t.Fatalf("list is not id DESC: %d then %d", all[i-1].ID, all[i].ID)
			}
		}

		first, hasNext, err := repos.providers.List(ctx, 0, 2, "Paging Provider")
		if err != nil {
			t.Fatal(err)
		}
		if !hasNext || len(first) != 2 || first[0].ID != ids[3] || first[1].ID != ids[2] {
			t.Fatalf("first page = %+v has_next=%v, want %d/%d and true", first, hasNext, ids[3], ids[2])
		}

		last, hasNext, err := repos.providers.List(ctx, 2, 2, "Paging Provider")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(last) != 2 || last[0].ID != ids[1] || last[1].ID != ids[0] {
			t.Fatalf("last page = %+v has_next=%v, want %d/%d and false", last, hasNext, ids[1], ids[0])
		}

		past, hasNext, err := repos.providers.List(ctx, 4, 2, "Paging Provider")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(past) != 0 {
			t.Fatalf("past-the-end page = %d items has_next=%v, want 0 and false", len(past), hasNext)
		}
	})

	t.Run("delete referenced by catalog and order", func(t *testing.T) {
		provider, err := repos.providers.Create(ctx, &domain.Provider{Name: "Proveedor Referenciado"})
		if err != nil {
			t.Fatal(err)
		}
		cost := any("1.00")
		if repos.gorm.Dialector.Name() == "sqlite" {
			cost = int64(100)
		}
		var catalogID int64
		if err := repos.gorm.Raw(
			"INSERT INTO supplier_catalogs (provider_id, last_quoted_cost) VALUES (?, ?) RETURNING id",
			provider.ID, cost,
		).Scan(&catalogID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := repos.providers.Delete(ctx, provider.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
			t.Fatalf("delete provider referenced by catalog error = %v, want ErrForeignKeyViolation", err)
		}
		if _, err := repos.providers.Get(ctx, provider.ID); err != nil {
			t.Fatalf("referenced provider must survive failed delete: %v", err)
		}
		if err := repos.gorm.Exec("DELETE FROM supplier_catalogs WHERE id = ?", catalogID).Error; err != nil {
			t.Fatal(err)
		}

		var orderID int64
		if err := repos.gorm.Raw(
			"INSERT INTO purchase_orders (provider_id, status, order_date, total_amount, payment_status) VALUES (?, 'PENDING', ?, ?, 'PENDING') RETURNING id",
			provider.ID, "2026-01-01T00:00:00.000000000Z", cost,
		).Scan(&orderID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := repos.providers.Delete(ctx, provider.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
			t.Fatalf("delete provider referenced by order error = %v, want ErrForeignKeyViolation", err)
		}
		if err := repos.gorm.Exec("DELETE FROM purchase_orders WHERE id = ?", orderID).Error; err != nil {
			t.Fatal(err)
		}

		if _, err := repos.providers.Delete(ctx, provider.ID); err != nil {
			t.Fatalf("delete unreferenced provider error = %v", err)
		}
	})
}

func assertProvider(t *testing.T, got, want *domain.Provider) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name ||
		!equalStringPtr(got.ContactInfo, want.ContactInfo) ||
		!equalStringPtr(got.Email, want.Email) ||
		!equalStringPtr(got.Phone, want.Phone) {
		t.Fatalf("provider = %+v, want %+v", got, want)
	}
}
