package outbound

import "context"

type StockRepository interface {
	LockForUpdate(ctx context.Context, ids []int64) (map[int64]int64, error)
	ApplyDelta(ctx context.Context, id int64, delta int64) error
}
