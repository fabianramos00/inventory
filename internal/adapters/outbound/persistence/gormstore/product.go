package gormstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type productStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.ProductRepository = (*productStore)(nil)

func NewProductStore(db *gorm.DB) outbound.ProductRepository {
	return &productStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *productStore) Get(ctx context.Context, id int64) (*domain.ProductDetail, error) {
	row := s.db.WithContext(ctx).Raw(s.selectSQL()+" WHERE p.id = ?", id).Row()
	detail, err := s.scanDetail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	return detail, nil
}

func (s *productStore) List(ctx context.Context, filter outbound.ProductFilter) ([]*domain.ProductDetail, bool, error) {
	search := strings.TrimSpace(filter.Search)
	filter.Search = search
	searchInSQL := search != "" && !s.sqlite
	where, args := s.filterSQL(filter, searchInSQL)

	query := s.selectSQL() + where + " ORDER BY p.id DESC"
	paginateInSQL := search == "" || searchInSQL
	if paginateInSQL {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit+1, filter.Skip)
	}

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("list products: %w", err)
	}
	items, err := s.scanDetails(rows)
	if err != nil {
		return nil, false, fmt.Errorf("list products: %w", err)
	}

	if !paginateInSQL {
		items = filterProductSearch(items, search)
		if filter.Skip >= len(items) {
			items = nil
		} else {
			items = items[filter.Skip:]
		}
	}

	hasNext := len(items) > filter.Limit
	if hasNext {
		items = items[:filter.Limit]
	}
	return items, hasNext, nil
}

func (s *productStore) Create(ctx context.Context, product *domain.Product) (*domain.Product, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(
		`INSERT INTO products
			(sku, name, size_value, measurement_unit_id, price, cost, stock_quantity, low_stock_threshold, brand_id, category_id, material_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		product.SKU,
		product.Name,
		product.SizeValue,
		product.MeasurementUnitID,
		moneyToDB(s.sqlite, product.Price),
		moneyToDB(s.sqlite, product.Cost),
		product.StockQuantity,
		product.LowStockThreshold,
		product.BrandID,
		product.CategoryID,
		product.MaterialID,
	).Scan(&id).Error
	if err != nil {
		return nil, storeError(s.db, "create product", err)
	}
	product.ID = id
	return product, nil
}

func (s *productStore) Update(ctx context.Context, id int64, product *domain.Product) error {
	result := s.db.WithContext(ctx).Exec(
		`UPDATE products SET
			sku = ?, name = ?, size_value = ?, measurement_unit_id = ?, price = ?, cost = ?,
			stock_quantity = ?, low_stock_threshold = ?, brand_id = ?, category_id = ?, material_id = ?
		 WHERE id = ?`,
		product.SKU,
		product.Name,
		product.SizeValue,
		product.MeasurementUnitID,
		moneyToDB(s.sqlite, product.Price),
		moneyToDB(s.sqlite, product.Cost),
		product.StockQuantity,
		product.LowStockThreshold,
		product.BrandID,
		product.CategoryID,
		product.MaterialID,
		id,
	)
	if result.Error != nil {
		return storeError(s.db, "update product", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *productStore) Delete(ctx context.Context, id int64) (*domain.Product, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	result := s.db.WithContext(ctx).Exec("DELETE FROM products WHERE id = ?", id)
	if result.Error != nil {
		return nil, storeError(s.db, "delete product", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	product := detail.Product
	return &product, nil
}

func (s *productStore) Stats(ctx context.Context) (*domain.ProductStats, error) {
	valueExpression := "COALESCE(SUM(p.stock_quantity * p.price), 0)"
	if !s.sqlite {
		valueExpression += "::text"
	}
	var (
		totalProducts int64
		totalStock    int64
		lowStock      int64
		valueRaw      any
	)
	err := s.db.WithContext(ctx).Raw(
		`SELECT COUNT(p.id),
			COALESCE(SUM(p.stock_quantity), 0),
			COALESCE(SUM(CASE WHEN p.stock_quantity > 0 AND p.stock_quantity <= p.low_stock_threshold THEN 1 ELSE 0 END), 0),
			`+valueExpression+`
		 FROM products p`,
	).Row().Scan(&totalProducts, &totalStock, &lowStock, &valueRaw)
	if err != nil {
		return nil, fmt.Errorf("product stats: %w", err)
	}
	value, err := moneyFromDB(s.sqlite, valueRaw)
	if err != nil {
		return nil, fmt.Errorf("product stats value: %w", err)
	}
	return &domain.ProductStats{
		TotalProducts:       totalProducts,
		TotalStock:          totalStock,
		LowStockCount:       lowStock,
		TotalInventoryValue: value,
	}, nil
}

func (s *productStore) selectSQL() string {
	return `SELECT
		p.id, p.sku, p.name, p.size_value, p.measurement_unit_id, ` + moneySQL(s.sqlite, "p.price") + `, ` + moneySQL(s.sqlite, "p.cost") + `,
		p.stock_quantity, p.low_stock_threshold, p.brand_id, p.category_id, p.material_id,
		c.id, c.name, c.code, c.description,
		m.id, m.name,
		b.id, b.name, b.logo_url,
		u.id, u.name, u.abbreviation
	FROM products p
	LEFT JOIN categories c ON c.id = p.category_id
	LEFT JOIN materials m ON m.id = p.material_id
	LEFT JOIN brands b ON b.id = p.brand_id
	LEFT JOIN measurement_units u ON u.id = p.measurement_unit_id`
}

func (s *productStore) filterSQL(filter outbound.ProductFilter, includeSearch bool) (string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 7)
	if filter.CategoryID != nil {
		conditions = append(conditions, "p.category_id = ?")
		args = append(args, *filter.CategoryID)
	}
	if filter.BrandID != nil {
		conditions = append(conditions, "p.brand_id = ?")
		args = append(args, *filter.BrandID)
	}
	if filter.MaterialID != nil {
		conditions = append(conditions, "p.material_id = ?")
		args = append(args, *filter.MaterialID)
	}
	switch filter.StockStatus {
	case "in_stock":
		conditions = append(conditions, "p.stock_quantity > 0")
	case "low_stock":
		conditions = append(conditions, "p.stock_quantity > 0 AND p.stock_quantity <= p.low_stock_threshold")
	case "out_of_stock":
		conditions = append(conditions, "p.stock_quantity = 0")
	}
	if includeSearch {
		conditions = append(conditions, "(strpos(lower(p.name), lower(?)) > 0 OR strpos(lower(p.sku), lower(?)) > 0)")
		args = append(args, filter.Search, filter.Search)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// ponytail: SQLite search folds case in Go over the filtered full scan; add
// FTS5 if the product table becomes large enough for the scan to matter.
func filterProductSearch(items []*domain.ProductDetail, search string) []*domain.ProductDetail {
	needle := strings.ToLower(search)
	matched := items[:0]
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), needle) || strings.Contains(strings.ToLower(item.SKU), needle) {
			matched = append(matched, item)
		}
	}
	return matched
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *productStore) scanDetails(rows *sql.Rows) ([]*domain.ProductDetail, error) {
	defer rows.Close()
	items := make([]*domain.ProductDetail, 0)
	for rows.Next() {
		detail, err := s.scanDetail(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, detail)
	}
	return items, rows.Err()
}

func (s *productStore) scanDetail(row rowScanner) (*domain.ProductDetail, error) {
	var (
		id                int64
		sku               string
		name              string
		sizeValue         sql.NullString
		measurementUnitID sql.NullInt64
		priceRaw          any
		costRaw           any
		stockQuantity     int64
		lowStockThreshold int64
		brandID           sql.NullInt64
		categoryID        sql.NullInt64
		materialID        sql.NullInt64
		categoryJoinID    sql.NullInt64
		categoryName      sql.NullString
		categoryCode      sql.NullString
		categoryDesc      sql.NullString
		materialJoinID    sql.NullInt64
		materialName      sql.NullString
		brandJoinID       sql.NullInt64
		brandName         sql.NullString
		brandLogoURL      sql.NullString
		unitJoinID        sql.NullInt64
		unitName          sql.NullString
		unitAbbreviation  sql.NullString
	)
	err := row.Scan(
		&id, &sku, &name, &sizeValue, &measurementUnitID, &priceRaw, &costRaw,
		&stockQuantity, &lowStockThreshold, &brandID, &categoryID, &materialID,
		&categoryJoinID, &categoryName, &categoryCode, &categoryDesc,
		&materialJoinID, &materialName,
		&brandJoinID, &brandName, &brandLogoURL,
		&unitJoinID, &unitName, &unitAbbreviation,
	)
	if err != nil {
		return nil, err
	}
	price, err := moneyFromDB(s.sqlite, priceRaw)
	if err != nil {
		return nil, fmt.Errorf("decode product price: %w", err)
	}
	cost, err := moneyFromDB(s.sqlite, costRaw)
	if err != nil {
		return nil, fmt.Errorf("decode product cost: %w", err)
	}

	product := domain.Product{
		ID:                id,
		SKU:               sku,
		Name:              name,
		SizeValue:         nullString(sizeValue),
		MeasurementUnitID: nullInt64(measurementUnitID),
		Price:             price,
		Cost:              cost,
		StockQuantity:     int(stockQuantity),
		LowStockThreshold: int(lowStockThreshold),
		BrandID:           brandID.Int64,
		CategoryID:        categoryID.Int64,
		MaterialID:        materialID.Int64,
	}
	detail := &domain.ProductDetail{Product: product}
	if categoryJoinID.Valid {
		detail.Category = &domain.Category{
			ID:          categoryJoinID.Int64,
			Name:        categoryName.String,
			Code:        nullString(categoryCode),
			Description: nullString(categoryDesc),
		}
	}
	if materialJoinID.Valid {
		detail.Material = &domain.Material{ID: materialJoinID.Int64, Name: materialName.String}
	}
	if brandJoinID.Valid {
		detail.Brand = &domain.Brand{ID: brandJoinID.Int64, Name: brandName.String, LogoURL: nullString(brandLogoURL)}
	}
	if unitJoinID.Valid {
		detail.MeasurementUnit = &domain.MeasurementUnit{
			ID:           unitJoinID.Int64,
			Name:         unitName.String,
			Abbreviation: unitAbbreviation.String,
		}
	}
	return detail, nil
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
