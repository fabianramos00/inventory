package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/domain"
)

// testClientDebt runs the derived debt cache port against PostgreSQL and
// SQLite: lock, recompute, debtor ordering, and integer-cents storage.
func testClientDebt(t *testing.T, repos *repositories) {
	t.Helper()
	ctx := context.Background()

	updatedAt := time.Date(2026, time.January, 3, 4, 5, 6, 0, time.UTC)
	client, err := repos.clients.Create(ctx, &domain.Client{
		Name:      "Debt Contract Client",
		CreatedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt: &updatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := repos.clients.Create(ctx, &domain.Client{
		Name:      "Debt Contract Sibling",
		CreatedAt: time.Date(2026, time.January, 2, 3, 4, 6, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	money := func(value string) any {
		if repos.gorm.Dialector.Name() == "sqlite" {
			return decimal.RequireFromString(value).Shift(2).IntPart()
		}
		return value
	}
	insertSale := func(createdAt time.Time, total, paid string, clientID *int64) {
		t.Helper()
		result := repos.gorm.Exec(
			`INSERT INTO sales (created_at, total_amount, amount_paid, payment_status, delivery_status, client_id)
			 VALUES (?, ?, ?, 'PENDING', 'PENDING', ?)`,
			gormstore.Timestamp(createdAt), money(total), money(paid), clientID,
		)
		if result.Error != nil {
			t.Fatal(result.Error)
		}
	}
	base := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	insertSale(base, "100.00", "0", &client.ID)
	insertSale(base.Add(time.Hour), "50.00", "0", &client.ID)
	insertSale(base.Add(2*time.Hour), "10.00", "10.00", &client.ID)
	insertSale(base, "20.00", "0", &client.ID)
	insertSale(base, "10.00", "20.00", &client.ID) // overpaid clamps to zero
	insertSale(base, "99.00", "0", nil)            // anonymous sale is ignored
	t.Cleanup(func() {
		repos.gorm.Exec("DELETE FROM sales WHERE client_id IN (?, ?)", client.ID, sibling.ID)
		repos.gorm.Exec("DELETE FROM sales WHERE client_id IS NULL AND total_amount = ?", money("99.00"))
		repos.gorm.Exec("DELETE FROM clients WHERE id IN (?, ?)", client.ID, sibling.ID)
	})

	if _, err := repos.clientDebt.Lock(ctx, client.ID); err != nil {
		t.Fatalf("Lock existing client error = %v", err)
	}
	if _, err := repos.clientDebt.Lock(ctx, client.ID+9999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Lock missing client error = %v, want ErrNotFound", err)
	}
	if err := repos.clientDebt.SetClientDebt(ctx, client.ID+9999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetClientDebt missing client error = %v, want ErrNotFound", err)
	}

	if err := repos.clientDebt.SetClientDebt(ctx, client.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repos.clients.Get(ctx, client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DebtAmount.Equal(decimal.RequireFromString("170.00")) {
		t.Fatalf("client debt = %s, want 170.00", got.DebtAmount)
	}
	if got.UpdatedAt == nil || !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated_at = %v, want unchanged %v", got.UpdatedAt, updatedAt)
	}

	if err := repos.clientDebt.SetClientDebt(ctx, sibling.ID); err != nil {
		t.Fatal(err)
	}
	siblingGot, err := repos.clients.Get(ctx, sibling.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !siblingGot.DebtAmount.IsZero() {
		t.Fatalf("sibling debt = %s, want 0", siblingGot.DebtAmount)
	}

	if repos.gorm.Dialector.Name() == "sqlite" {
		var cents int64
		if err := repos.gorm.Raw("SELECT debt_amount FROM clients WHERE id = ?", client.ID).Row().Scan(&cents); err != nil {
			t.Fatal(err)
		}
		if cents != 17000 {
			t.Fatalf("sqlite debt_amount = %d cents, want 17000", cents)
		}
	} else {
		var text string
		if err := repos.gorm.Raw("SELECT debt_amount::text FROM clients WHERE id = ?", client.ID).Row().Scan(&text); err != nil {
			t.Fatal(err)
		}
		if text != "170.00" {
			t.Fatalf("postgres debt_amount = %q, want 170.00", text)
		}
	}

	sum, err := repos.clientDebt.DebtSum(ctx, client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Equal(decimal.RequireFromString("170.00")) {
		t.Fatalf("client debt sum = %s, want 170.00", sum)
	}

	debtors, err := repos.clientDebt.DebtorSales(ctx, client.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(debtors) != 3 {
		t.Fatalf("debtor sales = %+v, want three", debtors)
	}
	for i := 1; i < len(debtors); i++ {
		previous := debtors[i-1].CreatedAt
		next := debtors[i].CreatedAt
		if previous.After(next) || (previous.Equal(next) && debtors[i-1].ID >= debtors[i].ID) {
			t.Fatalf("debtor order = %v/%d then %v/%d, want created_at ASC, id ASC",
				previous, debtors[i-1].ID, next, debtors[i].ID)
		}
	}
	wantDebt := []string{"100.00", "20.00", "50.00"}
	for i, debtor := range debtors {
		if !debtor.DebtAmount.Equal(decimal.RequireFromString(wantDebt[i])) {
			t.Fatalf("debtor %d debt = %s, want %s", i, debtor.DebtAmount, wantDebt[i])
		}
	}

	limited, err := repos.clientDebt.DebtorSales(ctx, client.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].ID != debtors[0].ID || limited[1].ID != debtors[1].ID {
		t.Fatalf("limited debtor sales = %+v, want the two oldest", limited)
	}

	empty, err := repos.clientDebt.DebtorSales(ctx, sibling.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("sibling debtor sales = %+v, want none", empty)
	}
}
