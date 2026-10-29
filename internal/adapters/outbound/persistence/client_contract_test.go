package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inventory-desktop/internal/domain"
)

func testClients(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()

	newClient := func(name string, identityCard, email, phone *string) *domain.Client {
		t.Helper()
		client, err := repos.clients.Create(ctx, &domain.Client{
			Name:         name,
			IdentityCard: identityCard,
			Email:        email,
			Phone:        phone,
			CreatedAt:    time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	str := func(value string) *string { return &value }

	t.Run("create get and timestamps", func(t *testing.T) {
		// PostgreSQL timestamptz keeps microseconds, SQLite keeps nanoseconds;
		// microsecond-aligned values round-trip exactly on both.
		createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC)
		created, err := repos.clients.Create(ctx, &domain.Client{
			Name:         "Timestamp Client",
			IdentityCard: str("TS-1"),
			Email:        str("ts@example.com"),
			Phone:        str("3001"),
			CreatedAt:    createdAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if created.ID <= 0 {
			t.Fatalf("Create returned id %d, want a positive id", created.ID)
		}
		got, err := repos.clients.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Timestamp Client" || got.IdentityCard == nil || *got.IdentityCard != "TS-1" ||
			got.Email == nil || *got.Email != "ts@example.com" || got.Phone == nil || *got.Phone != "3001" {
			t.Fatalf("client = %+v", got)
		}
		if !got.CreatedAt.Equal(createdAt) || got.UpdatedAt != nil {
			t.Fatalf("timestamps = %v/%v, want %v/nil", got.CreatedAt, got.UpdatedAt, createdAt)
		}

		updatedAt := time.Date(2026, time.January, 2, 4, 5, 6, 987654000, time.UTC)
		updated := *got
		updated.Name = "Timestamp Client Updated"
		updated.IdentityCard = nil
		updated.UpdatedAt = &updatedAt
		if err := repos.clients.Update(ctx, updated.ID, &updated); err != nil {
			t.Fatal(err)
		}
		got, err = repos.clients.Get(ctx, updated.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Timestamp Client Updated" || got.IdentityCard != nil {
			t.Fatalf("updated client = %+v", got)
		}
		if !got.CreatedAt.Equal(createdAt) || got.UpdatedAt == nil || !got.UpdatedAt.Equal(updatedAt) {
			t.Fatalf("updated timestamps = %v/%v, want %v/%v", got.CreatedAt, got.UpdatedAt, createdAt, updatedAt)
		}
		if got.Email == nil || *got.Email != "ts@example.com" || got.Phone == nil || *got.Phone != "3001" {
			t.Fatalf("omitted fields changed: %+v", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := repos.clients.Get(ctx, 999999); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get missing row error = %v, want ErrNotFound", err)
		}
		missing := &domain.Client{ID: 999999, Name: "Ghost", CreatedAt: time.Now().UTC()}
		if err := repos.clients.Update(ctx, missing.ID, missing); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Update missing row error = %v, want ErrNotFound", err)
		}
		if _, err := repos.clients.Delete(ctx, 999999); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Delete missing row error = %v, want ErrNotFound", err)
		}
	})

	t.Run("duplicate identity card", func(t *testing.T) {
		first := newClient("Duplicate One", str("DUP-1"), nil, nil)
		if _, err := repos.clients.Create(ctx, &domain.Client{
			Name: "Duplicate Two", IdentityCard: str("DUP-1"), CreatedAt: time.Now().UTC(),
		}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate identity error = %v, want ErrConflict", err)
		}
		third := newClient("Duplicate Three", str("DUP-3"), nil, nil)
		third.IdentityCard = first.IdentityCard
		third.UpdatedAt = ptrTime(time.Now().UTC())
		if err := repos.clients.Update(ctx, third.ID, third); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("update to duplicate identity error = %v, want ErrConflict", err)
		}
	})

	t.Run("null identity card is not unique", func(t *testing.T) {
		newClient("No Card A", nil, nil, nil)
		newClient("No Card B", nil, nil, nil)
	})

	t.Run("search all fields and accents", func(t *testing.T) {
		client := newClient("María Árbol", str("CC-1000"), str("Arbol@Example.com"), str("+57 3001"))
		for _, query := range []string{"maría", "ÁRBOL", "árbol", "cc-1000", "CC-1000", "arbol@example", "3001"} {
			items, hasNext, err := repos.clients.List(ctx, 0, 10, query)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range items {
				if item.ID == client.ID {
					found = true
				}
			}
			if hasNext || !found {
				t.Fatalf("search %q = %+v has_next=%v, want client %d", query, items, hasNext, client.ID)
			}
		}
	})

	t.Run("literal wildcards", func(t *testing.T) {
		percent := newClient("100% Steel", nil, nil, nil)
		newClient("100 Steel", nil, nil, nil)
		underscore := newClient("a_b Steel", nil, nil, nil)
		newClient("axb Steel", nil, nil, nil)

		for _, tt := range []struct {
			query string
			want  int64
		}{{"%", percent.ID}, {"_", underscore.ID}} {
			items, hasNext, err := repos.clients.List(ctx, 0, 10, tt.query)
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
		for _, name := range []string{"Paging Client Alpha", "Paging Client Beta", "Paging Client Gamma", "Paging Client Delta"} {
			ids = append(ids, newClient(name, nil, nil, nil).ID)
		}

		all, hasNext, err := repos.clients.List(ctx, 0, 100, "Paging Client")
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

		first, hasNext, err := repos.clients.List(ctx, 0, 2, "Paging Client")
		if err != nil {
			t.Fatal(err)
		}
		if !hasNext || len(first) != 2 || first[0].ID != ids[3] || first[1].ID != ids[2] {
			t.Fatalf("first page = %+v has_next=%v, want %d/%d and true", first, hasNext, ids[3], ids[2])
		}

		last, hasNext, err := repos.clients.List(ctx, 2, 2, "Paging Client")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(last) != 2 || last[0].ID != ids[1] || last[1].ID != ids[0] {
			t.Fatalf("last page = %+v has_next=%v, want %d/%d and false", last, hasNext, ids[1], ids[0])
		}

		past, hasNext, err := repos.clients.List(ctx, 4, 2, "Paging Client")
		if err != nil {
			t.Fatal(err)
		}
		if hasNext || len(past) != 0 {
			t.Fatalf("past-the-end page = %d items has_next=%v, want 0 and false", len(past), hasNext)
		}
	})

	t.Run("delete referenced", func(t *testing.T) {
		client := newClient("Referenced Client", str("REF-1"), nil, nil)
		money := any("0.00")
		if repos.gorm.Dialector.Name() == "sqlite" {
			money = int64(0)
		}
		var saleID int64
		if err := repos.gorm.Raw(
			"INSERT INTO sales (created_at, total_amount, amount_paid, client_id) VALUES (?, ?, ?, ?) RETURNING id",
			"2026-01-01T00:00:00.000000000Z", money, money, client.ID,
		).Scan(&saleID).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = repos.gorm.Exec("DELETE FROM sales WHERE id = ?", saleID).Error
		})

		if _, err := repos.clients.Delete(ctx, client.ID); !errors.Is(err, domain.ErrForeignKeyViolation) {
			t.Fatalf("delete referenced client error = %v, want ErrForeignKeyViolation", err)
		}
		if _, err := repos.clients.Get(ctx, client.ID); err != nil {
			t.Fatalf("referenced client must survive the failed delete: %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		client := newClient("Delete Client", nil, nil, nil)
		deleted, err := repos.clients.Delete(ctx, client.ID)
		if err != nil {
			t.Fatal(err)
		}
		if deleted.ID != client.ID || deleted.Name != client.Name {
			t.Fatalf("deleted = %+v, want %+v", deleted, client)
		}
		if _, err := repos.clients.Get(ctx, client.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
		}
		if _, err := repos.clients.Delete(ctx, client.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("second delete error = %v, want ErrNotFound", err)
		}
	})
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
