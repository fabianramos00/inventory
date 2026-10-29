package gormstore

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type stockStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.StockRepository = (*stockStore)(nil)

func NewStockStore(db *gorm.DB) outbound.StockRepository {
	return &stockStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *stockStore) LockForUpdate(ctx context.Context, ids []int64) (map[int64]int64, error) {
	quantities := make(map[int64]int64, len(ids))
	ids = sortedUniqueIDs(ids)
	if len(ids) == 0 {
		return quantities, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf(
		"SELECT id, stock_quantity FROM products WHERE id IN (%s) ORDER BY id",
		placeholders,
	)
	if !s.sqlite {
		query += " FOR UPDATE"
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("lock products: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, quantity int64
		if err := rows.Scan(&id, &quantity); err != nil {
			return nil, fmt.Errorf("scan locked product: %w", err)
		}
		quantities[id] = quantity
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read locked products: %w", err)
	}
	return quantities, nil
}

func (s *stockStore) ApplyDelta(ctx context.Context, id int64, delta int64) error {
	result := s.db.WithContext(ctx).Exec(
		"UPDATE products SET stock_quantity = stock_quantity + ? WHERE id = ? AND stock_quantity + ? >= 0",
		delta, id, delta,
	)
	if result.Error != nil {
		return storeError("apply stock delta", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrInsufficientStock
	}
	return nil
}

func sortedUniqueIDs(ids []int64) []int64 {
	unique := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := unique[id]; ok {
			continue
		}
		unique[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
