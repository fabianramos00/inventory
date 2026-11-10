package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

// setRawClientDebt writes clients.debt_amount directly, bypassing the
// recompute path, to simulate a stale derived-debt cache.
func setRawClientDebt(t *testing.T, b *siBackend, clientID int64, value string) {
	t.Helper()
	var stored any = value
	if b.gorm.Dialector.Name() == "sqlite" {
		stored = decimal.RequireFromString(value).Shift(2).IntPart()
	}
	if err := b.gorm.Exec("UPDATE clients SET debt_amount = ? WHERE id = ?", stored, clientID).Error; err != nil {
		t.Fatal(err)
	}
}

// The batch payment must validate against the debt derived from the sales
// ledger inside the transaction, never against the cached clients.debt_amount.
func TestClientBatchPaymentUsesDerivedDebtNotCache(t *testing.T) {
	forEachClientDebtBackend(t, func(t *testing.T, b *siBackend) {
		t.Run("stale low cache still allows the full payment", func(t *testing.T) {
			client := createSiClient(t, b, "Stale low cache "+b.name)
			createSiDebtSale(t, b, client.ID, decimal.RequireFromString("100.00"), 1)
			setRawClientDebt(t, b, client.ID, "10.00")

			status, raw := clientPayment(t, b, client.ID, 100.00, domain.PaymentMethodCash)
			if status != http.StatusOK {
				t.Fatalf("full payment against low cache status = %d body = %s, want 200", status, raw)
			}
			result := siDecode[siClientPayment](t, raw)
			assertNumber(t, result.RemainingDebt, "0")
			assertClientDebt(t, b, client.ID, "0")
		})

		t.Run("stale high cache cannot authorize an overpayment", func(t *testing.T) {
			client := createSiClient(t, b, "Stale high cache "+b.name)
			createSiDebtSale(t, b, client.ID, decimal.RequireFromString("100.00"), 1)
			setRawClientDebt(t, b, client.ID, "500.00")

			status, raw := clientPayment(t, b, client.ID, 100.01, domain.PaymentMethodCash)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("overpayment against high cache status = %d body = %s, want 422", status, raw)
			}
			if got := clientPaymentCount(t, b, client.ID); got != 0 {
				t.Fatalf("payments after rejected overpayment = %d, want 0", got)
			}
			assertDerivedClientDebt(t, b, client.ID, decimal.RequireFromString("100.00"))

			status, raw = clientPayment(t, b, client.ID, 100.00, domain.PaymentMethodCash)
			if status != http.StatusOK {
				t.Fatalf("full payment against high cache status = %d body = %s, want 200", status, raw)
			}
			result := siDecode[siClientPayment](t, raw)
			assertNumber(t, result.RemainingDebt, "0")
			assertClientDebt(t, b, client.ID, "0")
		})
	})
}
