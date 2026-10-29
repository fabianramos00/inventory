package gormstore

import (
	"context"

	"gorm.io/gorm"

	"inventory-desktop/internal/ports/outbound"
)

type unitOfWork struct {
	db *gorm.DB
}

var _ outbound.UnitOfWork = (*unitOfWork)(nil)

func NewUnitOfWork(db *gorm.DB) outbound.UnitOfWork {
	return &unitOfWork{db: db}
}

func (u *unitOfWork) WithinTx(ctx context.Context, fn func(context.Context, outbound.TxRepos) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, outbound.TxRepos{
			Sales:     NewSaleStore(tx),
			Products:  NewProductStore(tx),
			Clients:   NewClientStore(tx),
			Stock:     NewStockStore(tx),
			Orders:    NewPurchaseOrderStore(tx),
			Catalogs:  NewSupplierCatalogStore(tx),
			Providers: NewProviderStore(tx),
		})
	})
}
