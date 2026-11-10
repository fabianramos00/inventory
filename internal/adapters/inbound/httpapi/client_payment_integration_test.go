package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/domain"
)

// Client payment and derived debt cache coverage against both real backends.

type siClient struct {
	ID           int64       `json:"id"`
	Name         string      `json:"name"`
	IdentityCard *string     `json:"identity_card"`
	Email        *string     `json:"email"`
	Phone        *string     `json:"phone"`
	DebtAmount   json.Number `json:"debt_amount"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    *string     `json:"updated_at"`
}

type siClientPayment struct {
	ClientID      int64        `json:"client_id"`
	Amount        json.Number  `json:"amount"`
	RemainingDebt json.Number  `json:"remaining_debt"`
	Allocations   []siAllocate `json:"allocations"`
}

type siAllocate struct {
	SaleID int64       `json:"sale_id"`
	Amount json.Number `json:"amount"`
}

func forEachClientDebtBackend(t *testing.T, fn func(t *testing.T, b *siBackend)) {
	t.Helper()
	for _, backend := range []struct {
		name string
		open func(*testing.T) *siBackend
	}{
		{"sqlite", newSISQLiteBackend},
		{"postgres", newSIPostgresBackend},
	} {
		t.Run(backend.name, func(t *testing.T) {
			fn(t, backend.open(t))
		})
	}
}

func createSiClient(t *testing.T, b *siBackend, name string) siClient {
	t.Helper()
	status, raw := b.request(t, http.MethodPost, "/api/v1/sales/clients", map[string]any{"name": name})
	if status != http.StatusOK {
		t.Fatalf("create client %q status = %d body = %s", name, status, raw)
	}
	return siDecode[siClient](t, raw)
}

func getSiClient(t *testing.T, b *siBackend, id int64) siClient {
	t.Helper()
	status, raw := b.request(t, http.MethodGet, "/api/v1/sales/clients/"+itoa(id), nil)
	if status != http.StatusOK {
		t.Fatalf("get client %d status = %d body = %s", id, status, raw)
	}
	return siDecode[siClient](t, raw)
}

func createSiDebtSale(t *testing.T, b *siBackend, clientID int64, unitPrice decimal.Decimal, quantity int) siSale {
	t.Helper()
	status, raw := b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
		"client_id": clientID,
		"items": []map[string]any{{
			"product_id": b.productIDs[2],
			"quantity":   quantity,
			"unit_price": unitPrice,
		}},
	})
	if status != http.StatusOK {
		t.Fatalf("create debt sale status = %d body = %s", status, raw)
	}
	sale := siDecode[siSale](t, raw)
	if sale.ClientID == nil || *sale.ClientID != clientID {
		t.Fatalf("sale client = %v, want %d", sale.ClientID, clientID)
	}
	return sale
}

func clientPayment(t *testing.T, b *siBackend, clientID int64, amount any, method string) (int, []byte) {
	t.Helper()
	return b.request(t, http.MethodPost, "/api/v1/sales/clients/"+itoa(clientID)+"/payments", map[string]any{
		"amount":         amount,
		"payment_method": method,
	})
}

func rawClientPayment(t *testing.T, b *siBackend, clientID int64, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(
		http.MethodPost,
		b.server.URL+"/api/v1/sales/clients/"+itoa(clientID)+"/payments",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+b.token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func clientPaymentCount(t *testing.T, b *siBackend, clientID int64) int {
	t.Helper()
	return siScalarInt(t, b, `SELECT COUNT(*) FROM sale_payments p
		JOIN sales s ON s.id = p.sale_id WHERE s.client_id = `+itoa(clientID))
}

func assertClientDebt(t *testing.T, b *siBackend, clientID int64, want string) {
	t.Helper()
	expected := decimal.RequireFromString(want)
	client := getSiClient(t, b, clientID)
	if got := decimal.RequireFromString(string(client.DebtAmount)); !got.Equal(expected) {
		t.Fatalf("client %d debt API = %s, want %s", clientID, got, expected)
	}
	if b.gorm.Dialector.Name() == "sqlite" {
		var cents int64
		if err := b.gorm.Raw("SELECT debt_amount FROM clients WHERE id = ?", clientID).Row().Scan(&cents); err != nil {
			t.Fatal(err)
		}
		if stored := decimal.New(cents, -2); !stored.Equal(expected) {
			t.Fatalf("client %d sqlite debt cents = %d (%s), want %s", clientID, cents, stored, expected)
		}
	} else {
		var text string
		if err := b.gorm.Raw("SELECT debt_amount::text FROM clients WHERE id = ?", clientID).Row().Scan(&text); err != nil {
			t.Fatal(err)
		}
		if stored := decimal.RequireFromString(text); !stored.Equal(expected) {
			t.Fatalf("client %d postgres debt = %q, want %s", clientID, text, expected)
		}
	}
	assertDerivedClientDebt(t, b, clientID, expected)
}

func assertDerivedClientDebt(t *testing.T, b *siBackend, clientID int64, expected decimal.Decimal) {
	t.Helper()
	if b.gorm.Dialector.Name() == "sqlite" {
		var cents int64
		if err := b.gorm.Raw(`SELECT COALESCE(SUM(CASE
			WHEN total_amount - amount_paid > 0 THEN total_amount - amount_paid ELSE 0 END), 0)
			FROM sales WHERE client_id = ?`, clientID).Row().Scan(&cents); err != nil {
			t.Fatal(err)
		}
		if stored := decimal.New(cents, -2); !stored.Equal(expected) {
			t.Fatalf("client %d derived sqlite debt = %d cents (%s), want %s", clientID, cents, stored, expected)
		}
		return
	}
	var text string
	if err := b.gorm.Raw(`SELECT COALESCE(SUM(GREATEST(total_amount - amount_paid, 0)), 0)::text
		FROM sales WHERE client_id = ?`, clientID).Row().Scan(&text); err != nil {
		t.Fatal(err)
	}
	if stored := decimal.RequireFromString(text); !stored.Equal(expected) {
		t.Fatalf("client %d derived postgres debt = %q, want %s", clientID, text, expected)
	}
}

func TestClientDebtCacheReconcilesWithLedger(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		created := createSiDebtSale(t, b, b.clientID, decimal.RequireFromString("10.50"), 2)
		assertClientDebt(t, b, b.clientID, "21.00")

		status, raw := b.request(t, http.MethodPost, "/api/v1/sales/"+itoa(created.ID)+"/items", map[string]any{
			"product_id": b.productIDs[1], "quantity": 3,
		})
		if status != http.StatusOK {
			t.Fatalf("add item status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, b.clientID, "21.60")

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(created.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("list items status = %d body = %s", status, raw)
		}
		var nailsID int64
		for _, item := range siDecode[siList[siItem]](t, raw).Items {
			if item.ProductID == b.productIDs[1] {
				nailsID = item.ID
			}
		}
		if nailsID == 0 {
			t.Fatal("added item not found")
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(created.ID)+"/items/"+itoa(nailsID), map[string]any{"quantity": 5})
		if status != http.StatusOK {
			t.Fatalf("update item status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, b.clientID, "22.00")

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales/"+itoa(created.ID)+"/payments", map[string]any{
			"amount": 5.00, "payment_method": "cash",
		})
		if status != http.StatusOK {
			t.Fatalf("create payment status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, b.clientID, "17.00")

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(created.ID)+"/payments", nil)
		if status != http.StatusOK {
			t.Fatalf("list payments status = %d body = %s", status, raw)
		}
		payments := siDecode[siList[siPayment]](t, raw)
		if len(payments.Items) != 1 {
			t.Fatalf("payments = %+v, want one", payments.Items)
		}
		status, raw = b.request(t, http.MethodDelete,
			"/api/v1/sales/"+itoa(created.ID)+"/payments/"+itoa(payments.Items[0].ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete payment status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, b.clientID, "22.00")

		status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(created.ID)+"/items/"+itoa(nailsID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete item status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, b.clientID, "21.00")

		other := createSiClient(t, b, "Reassignment target "+b.name)
		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(created.ID), map[string]any{"client_id": other.ID})
		if status != http.StatusOK {
			t.Fatalf("reassign client status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, b.clientID, "0")
		assertClientDebt(t, b, other.ID, "21.00")

		status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(created.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete sale status = %d body = %s", status, raw)
		}
		assertClientDebt(t, b, other.ID, "0")
	})
}

func TestAnonymousFullyPaidSaleLeavesNoClientDebt(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		status, raw := b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"payment_method": "cash",
			"amount_paid":    10.50,
			"items": []map[string]any{{
				"product_id": b.productIDs[0], "quantity": 1,
			}},
		})
		if status != http.StatusOK {
			t.Fatalf("anonymous paid sale status = %d body = %s", status, raw)
		}
		if got := siDecode[siSale](t, raw); got.ClientID != nil || !decimal.RequireFromString(string(got.DebtAmount)).IsZero() {
			t.Fatalf("anonymous paid sale = client %v debt %s", got.ClientID, got.DebtAmount)
		}
		assertClientDebt(t, b, b.clientID, "0")
	})
}

func TestAnonymousDebtSaleIsRejected(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		status, raw := b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"items": []map[string]any{{"product_id": b.productIDs[0], "quantity": 1}},
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("anonymous debt sale status = %d body = %s, want 422", status, raw)
		}
		if !contains(raw, "client_id is required") {
			t.Fatalf("anonymous debt sale body = %s", raw)
		}
		if got := siScalarInt(t, b, "SELECT COUNT(*) FROM sales"); got != 0 {
			t.Fatalf("sales after rejected create = %d, want 0", got)
		}
		if got := b.stock(t, b.productIDs[0]); got != 10 {
			t.Fatalf("stock after rejected create = %d, want 10", got)
		}
	})
}

func TestClientBatchPaymentAllocatesOldestFirst(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		newer := createSiDebtSale(t, b, b.clientID, decimal.RequireFromString("50.00"), 1)
		older := createSiDebtSale(t, b, b.clientID, decimal.RequireFromString("100.00"), 1)

		// The second sale is newer by id but older by created_at on purpose.
		olderAt := gormstore.Timestamp(time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC))
		newerAt := gormstore.Timestamp(time.Date(2026, time.January, 2, 10, 0, 0, 0, time.UTC))
		if err := b.gorm.Exec("UPDATE sales SET created_at = ? WHERE id = ?", olderAt, older.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := b.gorm.Exec("UPDATE sales SET created_at = ? WHERE id = ?", newerAt, newer.ID).Error; err != nil {
			t.Fatal(err)
		}

		status, raw := clientPayment(t, b, b.clientID, 120.00, domain.PaymentMethodCash)
		if status != http.StatusOK {
			t.Fatalf("batch payment status = %d body = %s", status, raw)
		}
		result := siDecode[siClientPayment](t, raw)
		if result.ClientID != b.clientID {
			t.Fatalf("client_id = %d, want %d", result.ClientID, b.clientID)
		}
		assertNumber(t, result.Amount, "120.00")
		assertNumber(t, result.RemainingDebt, "30.00")
		if len(result.Allocations) != 2 {
			t.Fatalf("allocations = %+v, want two", result.Allocations)
		}
		if result.Allocations[0].SaleID != older.ID || result.Allocations[1].SaleID != newer.ID {
			t.Fatalf("allocation order = %+v, want oldest %d then %d", result.Allocations, older.ID, newer.ID)
		}
		assertNumber(t, result.Allocations[0].Amount, "100.00")
		assertNumber(t, result.Allocations[1].Amount, "20.00")

		for _, sale := range []struct {
			id   int64
			paid string
			debt string
		}{
			{older.ID, "100.00", "0"},
			{newer.ID, "20.00", "30.00"},
		} {
			status, raw := b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.id), nil)
			if status != http.StatusOK {
				t.Fatalf("get sale %d status = %d body = %s", sale.id, status, raw)
			}
			got := siDecode[siSale](t, raw)
			assertNumber(t, got.AmountPaid, sale.paid)
			assertNumber(t, got.DebtAmount, sale.debt)
		}

		for _, saleID := range []int64{older.ID, newer.ID} {
			var method string
			var creator int64
			if err := b.gorm.Raw(
				`SELECT payment_method, created_by_id FROM sale_payments WHERE sale_id = ? ORDER BY id DESC LIMIT 1`,
				saleID,
			).Row().Scan(&method, &creator); err != nil {
				t.Fatal(err)
			}
			if method != strings.ToUpper(domain.PaymentMethodCash) || creator != b.userID {
				t.Fatalf("sale %d payment = %q by %d, want cash by %d", saleID, method, creator, b.userID)
			}
		}
		assertClientDebt(t, b, b.clientID, "30.00")

		status, raw = clientPayment(t, b, b.clientID, 30.00, domain.PaymentMethodCash)
		if status != http.StatusOK {
			t.Fatalf("full batch payment status = %d body = %s", status, raw)
		}
		settled := siDecode[siClientPayment](t, raw)
		assertNumber(t, settled.RemainingDebt, "0")
		if len(settled.Allocations) != 1 || settled.Allocations[0].SaleID != newer.ID {
			t.Fatalf("settling allocations = %+v, want only %d", settled.Allocations, newer.ID)
		}
		assertNumber(t, settled.Allocations[0].Amount, "30.00")
		assertClientDebt(t, b, b.clientID, "0")
	})
}

func TestClientBatchPaymentLeavesClientMetadataUntouched(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		client := createSiClient(t, b, "Metadata client "+b.name)
		createSiDebtSale(t, b, client.ID, decimal.RequireFromString("10.00"), 1)

		status, raw := clientPayment(t, b, client.ID, 4.00, domain.PaymentMethodDebit)
		if status != http.StatusOK {
			t.Fatalf("payment status = %d body = %s", status, raw)
		}
		got := getSiClient(t, b, client.ID)
		if got.UpdatedAt != nil {
			t.Fatalf("updated_at after payment = %v, want nil", *got.UpdatedAt)
		}
		if got.Name != client.Name || got.Email != nil || got.Phone != nil || got.IdentityCard != nil {
			t.Fatalf("client metadata after payment = %+v", got)
		}
	})
}

func TestClientBatchPaymentValidatesInput(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		noDebt := createSiClient(t, b, "No debt client "+b.name)
		debtClient := createSiClient(t, b, "Debt client "+b.name)
		createSiDebtSale(t, b, debtClient.ID, decimal.RequireFromString("100.00"), 1)

		check := func(name string, clientID int64, amount any, method string, want int) {
			t.Helper()
			t.Run(name, func(t *testing.T) {
				before := clientPaymentCount(t, b, debtClient.ID)
				status, raw := clientPayment(t, b, clientID, amount, method)
				if status != want {
					t.Fatalf("status = %d body = %s, want %d", status, raw, want)
				}
				if want != http.StatusOK {
					if after := clientPaymentCount(t, b, debtClient.ID); after != before {
						t.Fatalf("sale_payments after rejected payment = %d, want %d", after, before)
					}
					assertClientDebt(t, b, debtClient.ID, "100.00")
				}
			})
		}

		check("unknown client", 999999, 10.00, "cash", http.StatusNotFound)
		check("no debt", noDebt.ID, 10.00, "cash", http.StatusUnprocessableEntity)
		check("zero amount", debtClient.ID, 0, "cash", http.StatusUnprocessableEntity)
		check("negative amount", debtClient.ID, -1, "cash", http.StatusUnprocessableEntity)
		check("overpayment", debtClient.ID, 100.01, "cash", http.StatusUnprocessableEntity)
		check("invalid method", debtClient.ID, 10.00, "cheque", http.StatusUnprocessableEntity)
		check("missing amount", debtClient.ID, nil, "cash", http.StatusUnprocessableEntity)
		check("three decimals", debtClient.ID, 10.001, "cash", http.StatusUnprocessableEntity)
	})
}

func TestClientBatchPaymentRejectsMalformedAmounts(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"exponent", `{"amount":1e2,"payment_method":"cash"}`},
		{"string amount", `{"amount":"100.00","payment_method":"cash"}`},
		{"NaN", `{"amount":NaN,"payment_method":"cash"}`},
		{"Infinity", `{"amount":Infinity,"payment_method":"cash"}`},
		{"malformed", `{"amount":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
				client := createSiClient(t, b, "Malformed client "+b.name)
				createSiDebtSale(t, b, client.ID, decimal.RequireFromString("100.00"), 1)

				status, raw := rawClientPayment(t, b, client.ID, tc.body)
				if status != http.StatusUnprocessableEntity {
					t.Fatalf("status = %d body = %s, want 422", status, raw)
				}
				if got := clientPaymentCount(t, b, client.ID); got != 0 {
					t.Fatalf("sale_payments after malformed amount = %d, want 0", got)
				}
				assertClientDebt(t, b, client.ID, "100.00")
			})
		})
	}
}

func TestClientBatchPaymentRejectsAmountOverMoneyCeiling(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		client := createSiClient(t, b, "Ceiling client "+b.name)
		createSiDebtSale(t, b, client.ID, decimal.RequireFromString("9999999999.99"), 1)
		assertClientDebt(t, b, client.ID, "9999999999.99")

		status, raw := rawClientPayment(t, b, client.ID, `{"amount":10000000000,"payment_method":"cash"}`)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("over-ceiling payment status = %d body = %s, want 422", status, raw)
		}
		if got := clientPaymentCount(t, b, client.ID); got != 0 {
			t.Fatalf("sale_payments after over-ceiling amount = %d, want 0", got)
		}
		assertClientDebt(t, b, client.ID, "9999999999.99")
	})
}

func TestClientBatchPaymentCapsOpenSales(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		client := createSiClient(t, b, "Cap client "+b.name)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Microsecond)
		money := any(int64(100))
		zero := any(int64(0))
		if b.gorm.Dialector.Name() == "postgres" {
			money = "1.00"
			zero = "0"
		}
		for i := 0; i < 1001; i++ {
			if err := b.gorm.Exec(
				`INSERT INTO sales (created_at, total_amount, amount_paid, payment_status, delivery_status, client_id)
				 VALUES (?, ?, ?, 'PENDING', 'PENDING', ?)`,
				gormstore.Timestamp(now.Add(time.Duration(i)*time.Millisecond)), money, zero, client.ID,
			).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := gormstore.NewClientDebtStore(b.gorm).SetClientDebt(ctx, client.ID); err != nil {
			t.Fatal(err)
		}
		assertClientDebt(t, b, client.ID, "1001.00")

		status, raw := clientPayment(t, b, client.ID, 1001.00, domain.PaymentMethodCash)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("over-cap payment status = %d body = %s, want 422", status, raw)
		}
		if got := clientPaymentCount(t, b, client.ID); got != 0 {
			t.Fatalf("sale_payments after over-cap amount = %d, want 0", got)
		}
		assertClientDebt(t, b, client.ID, "1001.00")
	})
}
