package outbound

import (
	"context"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

// ClientDebtRepository maintains the derived clients.debt_amount cache. It is
// separate from ClientRepository so debt mutation stays an explicit
// transaction-bound capability.
type ClientDebtRepository interface {
	// Lock loads the client and takes a write lock on its row. Callers use it
	// before any sale mutation that changes the client's debt.
	Lock(ctx context.Context, id int64) (*domain.Client, error)
	// SetClientDebt recomputes clients.debt_amount from the sale ledger and
	// writes only that column, never clients.updated_at.
	SetClientDebt(ctx context.Context, clientID int64) error
	// DebtSum returns the derived client debt from the sale ledger without
	// loading sale rows.
	DebtSum(ctx context.Context, clientID int64) (decimal.Decimal, error)
	// DebtorSales lists up to limit of the client's sales that still carry
	// debt, oldest first, using created_at ASC, id ASC.
	DebtorSales(ctx context.Context, clientID int64, limit int) ([]*domain.Sale, error)
}
