package gormstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type clientDebtStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.ClientDebtRepository = (*clientDebtStore)(nil)

func NewClientDebtStore(db *gorm.DB) outbound.ClientDebtRepository {
	return &clientDebtStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *clientDebtStore) Lock(ctx context.Context, id int64) (*domain.Client, error) {
	if s.sqlite {
		if err := s.db.WithContext(ctx).Exec("UPDATE clients SET id = id WHERE id = ?", id).Error; err != nil {
			return nil, fmt.Errorf("lock client: %w", err)
		}
	}
	query := clientSelectSQL(s.sqlite) + " WHERE id = ?"
	if !s.sqlite {
		query += " FOR UPDATE"
	}
	client, err := scanClient(s.db.WithContext(ctx).Raw(query, id).Row(), s.sqlite)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock client: %w", err)
	}
	return client, nil
}

func (s *clientDebtStore) SetClientDebt(ctx context.Context, clientID int64) error {
	derived := `COALESCE((SELECT SUM(` + s.saleDebtSQL("s") + `) FROM sales s WHERE s.client_id = clients.id), 0)`
	result := s.db.WithContext(ctx).Exec(
		"UPDATE clients SET debt_amount = "+derived+" WHERE id = ?", clientID,
	)
	if result.Error != nil {
		return storeError(s.db, "recompute client debt", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *clientDebtStore) DebtSum(ctx context.Context, clientID int64) (decimal.Decimal, error) {
	var raw any
	err := s.db.WithContext(ctx).Raw(
		`SELECT `+moneySQL(s.sqlite, "COALESCE(SUM("+s.saleDebtSQL("s")+`), 0)`)+
			` FROM sales s WHERE s.client_id = ?`,
		clientID,
	).Row().Scan(&raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("sum client debt: %w", err)
	}
	debt, err := moneyFromDB(s.sqlite, raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("decode client debt: %w", err)
	}
	return debt, nil
}

func (s *clientDebtStore) saleDebtSQL(alias string) string {
	if s.sqlite {
		return fmt.Sprintf(
			"CASE WHEN %[1]s.total_amount - %[1]s.amount_paid > 0 THEN %[1]s.total_amount - %[1]s.amount_paid ELSE 0 END",
			alias,
		)
	}
	return fmt.Sprintf("GREATEST(%[1]s.total_amount - %[1]s.amount_paid, 0)", alias)
}

func (s *clientDebtStore) DebtorSales(ctx context.Context, clientID int64, limit int) ([]*domain.Sale, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		`SELECT id, created_at, `+moneySQL(s.sqlite, "total_amount")+`, `+
			moneySQL(s.sqlite, "amount_paid")+`
		 FROM sales
		 WHERE client_id = ? AND amount_paid < total_amount
		 ORDER BY created_at ASC, id ASC
		 LIMIT ?`,
		clientID, limit,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("list client debtor sales: %w", err)
	}
	return s.scanDebtorSales(rows)
}

func (s *clientDebtStore) scanDebtorSales(rows *sql.Rows) ([]*domain.Sale, error) {
	defer rows.Close()
	sales := make([]*domain.Sale, 0)
	for rows.Next() {
		var (
			id        int64
			createdAt Timestamp
			totalRaw  any
			paidRaw   any
		)
		if err := rows.Scan(&id, &createdAt, &totalRaw, &paidRaw); err != nil {
			return nil, err
		}
		total, err := moneyFromDB(s.sqlite, totalRaw)
		if err != nil {
			return nil, fmt.Errorf("decode debtor sale total: %w", err)
		}
		paid, err := moneyFromDB(s.sqlite, paidRaw)
		if err != nil {
			return nil, fmt.Errorf("decode debtor sale amount paid: %w", err)
		}
		debt := total.Sub(paid)
		if debt.IsNegative() {
			debt = decimal.Zero
		}
		sales = append(sales, &domain.Sale{
			ID:          id,
			CreatedAt:   createdAt.Time(),
			TotalAmount: total,
			AmountPaid:  paid,
			DebtAmount:  debt,
		})
	}
	return sales, rows.Err()
}
