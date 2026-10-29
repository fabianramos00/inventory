package outbound

import "context"

type TxRepos struct {
	Sales    SaleRepository
	Products ProductRepository
	Clients  ClientRepository
	Stock    StockRepository
}

type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(context.Context, TxRepos) error) error
}
