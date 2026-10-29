package gormstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type supplierCatalogStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.SupplierCatalogRepository = (*supplierCatalogStore)(nil)

func NewSupplierCatalogStore(db *gorm.DB) outbound.SupplierCatalogRepository {
	return &supplierCatalogStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *supplierCatalogStore) Get(ctx context.Context, id int64) (*domain.SupplierCatalog, error) {
	catalog, err := s.scanCatalog(s.db.WithContext(ctx).Raw(s.selectSQL()+" WHERE sc.id = ?", id).Row())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get supplier catalog: %w", err)
	}
	return catalog, nil
}

func (s *supplierCatalogStore) List(ctx context.Context, skip, limit int, providerID, productID *int64) ([]*domain.SupplierCatalog, bool, error) {
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if providerID != nil && *providerID > 0 {
		conditions = append(conditions, "sc.provider_id = ?")
		args = append(args, *providerID)
	}
	if productID != nil && *productID > 0 {
		conditions = append(conditions, "sc.product_id = ?")
		args = append(args, *productID)
	}

	query := s.selectSQL()
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY sc.id DESC LIMIT ? OFFSET ?"
	args = append(args, limit+1, skip)

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("list supplier catalogs: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.SupplierCatalog, 0)
	for rows.Next() {
		catalog, err := s.scanCatalog(rows)
		if err != nil {
			return nil, false, fmt.Errorf("list supplier catalogs: %w", err)
		}
		items = append(items, catalog)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list supplier catalogs: %w", err)
	}

	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return items, hasNext, nil
}

func (s *supplierCatalogStore) selectSQL() string {
	return `SELECT
		sc.id, sc.product_id, sc.provider_id, sc.supplier_sku, ` + s.moneySQL("sc.last_quoted_cost") + `, sc.lead_time,
		pv.id, pv.name, pv.contact_info, pv.email, pv.phone, pv.created_by_id,
		p.id, p.sku, p.name, p.size_value, p.measurement_unit_id,
		` + s.moneySQL("p.price") + `, ` + s.moneySQL("p.cost") + `,
		p.stock_quantity, p.low_stock_threshold, p.brand_id, p.category_id, p.material_id
	FROM supplier_catalogs sc
	LEFT JOIN providers pv ON pv.id = sc.provider_id
	LEFT JOIN products p ON p.id = sc.product_id`
}

func (s *supplierCatalogStore) moneySQL(column string) string {
	if s.sqlite {
		return column
	}
	return column + "::text"
}

func (s *supplierCatalogStore) scanCatalog(row rowScanner) (*domain.SupplierCatalog, error) {
	var (
		id            int64
		productID     sql.NullInt64
		providerID    sql.NullInt64
		supplierSKU   sql.NullString
		costRaw       any
		leadTime      sql.NullInt64
		providerJoin  sql.NullInt64
		providerName  sql.NullString
		contactInfo   sql.NullString
		email         sql.NullString
		phone         sql.NullString
		createdByID   sql.NullInt64
		productJoin   sql.NullInt64
		sku           sql.NullString
		name          sql.NullString
		sizeValue     sql.NullString
		unitID        sql.NullInt64
		priceRaw      any
		productCost   any
		stockQuantity sql.NullInt64
		lowStock      sql.NullInt64
		brandID       sql.NullInt64
		categoryID    sql.NullInt64
		materialID    sql.NullInt64
	)
	if err := row.Scan(
		&id, &productID, &providerID, &supplierSKU, &costRaw, &leadTime,
		&providerJoin, &providerName, &contactInfo, &email, &phone, &createdByID,
		&productJoin, &sku, &name, &sizeValue, &unitID, &priceRaw, &productCost,
		&stockQuantity, &lowStock, &brandID, &categoryID, &materialID,
	); err != nil {
		return nil, err
	}

	cost, err := s.moneyFromDB(costRaw)
	if err != nil {
		return nil, fmt.Errorf("decode last quoted cost: %w", err)
	}
	catalog := &domain.SupplierCatalog{
		ID:             id,
		ProductID:      productID.Int64,
		ProviderID:     providerID.Int64,
		SupplierSKU:    nullString(supplierSKU),
		LastQuotedCost: cost,
		LeadTime:       nullInt64(leadTime),
	}
	if providerJoin.Valid {
		catalog.Provider = &domain.Provider{
			ID:          providerJoin.Int64,
			Name:        providerName.String,
			ContactInfo: nullString(contactInfo),
			Email:       nullString(email),
			Phone:       nullString(phone),
			CreatedByID: nullInt64(createdByID),
		}
	}
	if productJoin.Valid {
		price, err := s.moneyFromDB(priceRaw)
		if err != nil {
			return nil, fmt.Errorf("decode product price: %w", err)
		}
		cost, err := s.moneyFromDB(productCost)
		if err != nil {
			return nil, fmt.Errorf("decode product cost: %w", err)
		}
		catalog.Product = &domain.Product{
			ID:                productJoin.Int64,
			SKU:               sku.String,
			Name:              name.String,
			SizeValue:         nullString(sizeValue),
			MeasurementUnitID: nullInt64(unitID),
			Price:             price,
			Cost:              cost,
			StockQuantity:     int(stockQuantity.Int64),
			LowStockThreshold: int(lowStock.Int64),
			BrandID:           brandID.Int64,
			CategoryID:        categoryID.Int64,
			MaterialID:        materialID.Int64,
		}
	}
	return catalog, nil
}

func (s *supplierCatalogStore) moneyFromDB(value any) (decimal.Decimal, error) {
	switch raw := value.(type) {
	case int64:
		if s.sqlite {
			return decimal.New(raw, -2), nil
		}
		return decimal.New(raw, 0), nil
	case string:
		return decimal.NewFromString(raw)
	case []byte:
		return decimal.NewFromString(string(raw))
	default:
		return decimal.Zero, fmt.Errorf("unsupported money value %T", value)
	}
}
