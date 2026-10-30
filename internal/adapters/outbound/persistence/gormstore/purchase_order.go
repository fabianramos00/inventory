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

type purchaseOrderStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.PurchaseOrderRepository = (*purchaseOrderStore)(nil)

func NewPurchaseOrderStore(db *gorm.DB) outbound.PurchaseOrderRepository {
	return &purchaseOrderStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *purchaseOrderStore) Get(ctx context.Context, id int64) (*domain.PurchaseOrder, error) {
	row := s.db.WithContext(ctx).Raw(s.selectSQL()+" WHERE po.id = ?", id).Row()
	order, err := s.scanOrder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get purchase order: %w", err)
	}
	return order, nil
}

func (s *purchaseOrderStore) GetForUpdate(ctx context.Context, id int64) (*domain.PurchaseOrder, error) {
	query := "SELECT id FROM purchase_orders WHERE id = ?"
	if !s.sqlite {
		query += " FOR UPDATE"
	}
	var locked int64
	err := s.db.WithContext(ctx).Raw(query, id).Row().Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock purchase order: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *purchaseOrderStore) List(ctx context.Context, filter outbound.PurchaseOrderFilter) ([]*domain.PurchaseOrder, bool, error) {
	where, args := s.filterSQL(filter)
	query := s.selectSQL() + where + " ORDER BY po.id DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit+1, filter.Skip)

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("list purchase orders: %w", err)
	}
	defer rows.Close()

	orders := make([]*domain.PurchaseOrder, 0)
	for rows.Next() {
		order, err := s.scanOrder(rows)
		if err != nil {
			return nil, false, fmt.Errorf("list purchase orders: %w", err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list purchase orders: %w", err)
	}

	hasNext := len(orders) > filter.Limit
	if hasNext {
		orders = orders[:filter.Limit]
	}
	return orders, hasNext, nil
}

func (s *purchaseOrderStore) Create(ctx context.Context, order *domain.PurchaseOrder, items []*domain.OrderItem) (*domain.PurchaseOrder, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(
		`INSERT INTO purchase_orders
			(provider_id, status, order_date, total_amount, created_by_id, payment_status)
		 VALUES (?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		order.ProviderID,
		dbOrderStatus(order.Status),
		Timestamp(order.OrderDate),
		moneyToDB(s.sqlite, order.TotalAmount),
		nullableID(order.CreatedByID),
		dbPurchasePaymentStatus(order.PaymentStatus),
	).Scan(&id).Error
	if err != nil {
		return nil, storeError(s.db, "create purchase order", err)
	}
	order.ID = id
	for _, item := range items {
		item.OrderID = id
		if _, err := s.CreateItem(ctx, item); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func (s *purchaseOrderStore) Update(ctx context.Context, id int64, order *domain.PurchaseOrder) error {
	result := s.db.WithContext(ctx).Exec(
		`UPDATE purchase_orders SET
			provider_id = ?, status = ?, payment_status = ?, total_amount = ?
		 WHERE id = ?`,
		order.ProviderID,
		dbOrderStatus(order.Status),
		dbPurchasePaymentStatus(order.PaymentStatus),
		moneyToDB(s.sqlite, order.TotalAmount),
		id,
	)
	if result.Error != nil {
		return storeError(s.db, "update purchase order", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *purchaseOrderStore) Delete(ctx context.Context, id int64) error {
	result := s.db.WithContext(ctx).Exec("DELETE FROM purchase_orders WHERE id = ?", id)
	if result.Error != nil {
		return storeError(s.db, "delete purchase order", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *purchaseOrderStore) ListItems(ctx context.Context, orderID int64, skip, limit int) ([]*domain.OrderItem, bool, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		s.itemsSQL()+" WHERE oi.order_id = ? ORDER BY oi.id ASC LIMIT ? OFFSET ?",
		orderID, limit+1, skip,
	).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("list order items: %w", err)
	}
	items, err := s.scanItems(rows)
	if err != nil {
		return nil, false, fmt.Errorf("list order items: %w", err)
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return items, hasNext, nil
}

func (s *purchaseOrderStore) ListAllItems(ctx context.Context, orderID int64) ([]*domain.OrderItem, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		s.itemsSQL()+" WHERE oi.order_id = ? ORDER BY oi.id ASC", orderID,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("load order items: %w", err)
	}
	items, err := s.scanItems(rows)
	if err != nil {
		return nil, fmt.Errorf("load order items: %w", err)
	}
	return items, nil
}

func (s *purchaseOrderStore) GetItem(ctx context.Context, orderID, itemID int64) (*domain.OrderItem, error) {
	row := s.db.WithContext(ctx).Raw(
		s.itemsSQL()+" WHERE oi.order_id = ? AND oi.id = ?", orderID, itemID,
	).Row()
	item, err := s.scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order item: %w", err)
	}
	return item, nil
}

func (s *purchaseOrderStore) GetItemByProduct(ctx context.Context, orderID, productID int64) (*domain.OrderItem, error) {
	row := s.db.WithContext(ctx).Raw(
		s.itemsSQL()+" WHERE oi.order_id = ? AND oi.product_id = ?", orderID, productID,
	).Row()
	item, err := s.scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order item by product: %w", err)
	}
	return item, nil
}

func (s *purchaseOrderStore) CreateItem(ctx context.Context, item *domain.OrderItem) (*domain.OrderItem, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(
		`INSERT INTO order_items
			(order_id, product_id, quantity, unit_cost, subtotal, supplier_sku)
		 VALUES (?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		item.OrderID,
		item.ProductID,
		item.Quantity,
		moneyToDB(s.sqlite, item.UnitCost),
		moneyToDB(s.sqlite, item.Subtotal),
		item.SupplierSKU,
	).Scan(&id).Error
	if err != nil {
		return nil, storeError(s.db, "create order item", err)
	}
	item.ID = id
	return item, nil
}

func (s *purchaseOrderStore) UpdateItem(ctx context.Context, item *domain.OrderItem) error {
	result := s.db.WithContext(ctx).Exec(
		`UPDATE order_items SET
			quantity = ?, unit_cost = ?, subtotal = ?, supplier_sku = ?
		 WHERE id = ? AND order_id = ?`,
		item.Quantity,
		moneyToDB(s.sqlite, item.UnitCost),
		moneyToDB(s.sqlite, item.Subtotal),
		item.SupplierSKU,
		item.ID,
		item.OrderID,
	)
	if result.Error != nil {
		return storeError(s.db, "update order item", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *purchaseOrderStore) DeleteItem(ctx context.Context, orderID, itemID int64) error {
	result := s.db.WithContext(ctx).Exec("DELETE FROM order_items WHERE id = ? AND order_id = ?", itemID, orderID)
	if result.Error != nil {
		return storeError(s.db, "delete order item", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *purchaseOrderStore) DeleteItems(ctx context.Context, orderID int64) error {
	if err := s.db.WithContext(ctx).Exec("DELETE FROM order_items WHERE order_id = ?", orderID).Error; err != nil {
		return storeError(s.db, "delete order items", err)
	}
	return nil
}

func (s *purchaseOrderStore) selectSQL() string {
	return `SELECT
		po.id, po.provider_id, po.status, po.payment_status, po.order_date,
		` + moneySQL(s.sqlite, "po.total_amount") + `, po.created_by_id,
		pv.id, pv.name, pv.contact_info, pv.email, pv.phone, pv.created_by_id
	FROM purchase_orders po
	LEFT JOIN providers pv ON pv.id = po.provider_id`
}

func (s *purchaseOrderStore) itemsSQL() string {
	return `SELECT
		oi.id, oi.order_id, oi.product_id, oi.quantity,
		` + moneySQL(s.sqlite, "oi.unit_cost") + `, ` + moneySQL(s.sqlite, "oi.subtotal") + `,
		oi.supplier_sku, p.name, p.sku, b.name, m.name, c.name
	FROM order_items oi
	JOIN products p ON p.id = oi.product_id
	LEFT JOIN brands b ON b.id = p.brand_id
	LEFT JOIN materials m ON m.id = p.material_id
	LEFT JOIN categories c ON c.id = p.category_id`
}

func (s *purchaseOrderStore) filterSQL(filter outbound.PurchaseOrderFilter) (string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if filter.ProviderID != nil && *filter.ProviderID > 0 {
		conditions = append(conditions, "po.provider_id = ?")
		args = append(args, *filter.ProviderID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "po.status = ?")
		args = append(args, dbOrderStatus(filter.Status))
	}
	if filter.PaymentStatus != "" {
		conditions = append(conditions, "po.payment_status = ?")
		args = append(args, dbPurchasePaymentStatus(filter.PaymentStatus))
	}
	if filter.MinDate != nil {
		conditions = append(conditions, "po.order_date >= ?")
		args = append(args, Timestamp(*filter.MinDate))
	}
	if filter.MaxDate != nil {
		conditions = append(conditions, "po.order_date <= ?")
		args = append(args, Timestamp(*filter.MaxDate))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func (s *purchaseOrderStore) scanOrder(row rowScanner) (*domain.PurchaseOrder, error) {
	var (
		id             int64
		providerID     sql.NullInt64
		status         string
		paymentStatus  string
		orderDate      Timestamp
		totalRaw       any
		createdByID    sql.NullInt64
		providerJoin   sql.NullInt64
		providerName   sql.NullString
		contactInfo    sql.NullString
		email          sql.NullString
		phone          sql.NullString
		providerUserID sql.NullInt64
	)
	if err := row.Scan(
		&id, &providerID, &status, &paymentStatus, &orderDate, &totalRaw, &createdByID,
		&providerJoin, &providerName, &contactInfo, &email, &phone, &providerUserID,
	); err != nil {
		return nil, err
	}
	total, err := moneyFromDB(s.sqlite, totalRaw)
	if err != nil {
		return nil, fmt.Errorf("decode order total: %w", err)
	}
	order := &domain.PurchaseOrder{
		ID:            id,
		Status:        apiOrderStatus(status),
		PaymentStatus: apiPurchasePaymentStatus(paymentStatus),
		OrderDate:     orderDate.Time(),
		TotalAmount:   total,
	}
	if providerID.Valid {
		order.ProviderID = providerID.Int64
	}
	if createdByID.Valid {
		order.CreatedByID = createdByID.Int64
	}
	if providerJoin.Valid {
		order.Provider = &domain.Provider{
			ID:          providerJoin.Int64,
			Name:        providerName.String,
			ContactInfo: nullString(contactInfo),
			Email:       nullString(email),
			Phone:       nullString(phone),
			CreatedByID: nullInt64(providerUserID),
		}
	}
	return order, nil
}

func (s *purchaseOrderStore) scanItems(rows *sql.Rows) ([]*domain.OrderItem, error) {
	defer rows.Close()
	items := make([]*domain.OrderItem, 0)
	for rows.Next() {
		item, err := s.scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *purchaseOrderStore) scanItem(row rowScanner) (*domain.OrderItem, error) {
	var (
		id           int64
		orderID      int64
		productID    int64
		quantity     int64
		unitCostRaw  any
		subtotalRaw  any
		supplierSKU  sql.NullString
		productName  string
		productSKU   string
		brandName    sql.NullString
		materialName sql.NullString
		categoryName sql.NullString
	)
	if err := row.Scan(
		&id, &orderID, &productID, &quantity, &unitCostRaw, &subtotalRaw,
		&supplierSKU, &productName, &productSKU, &brandName, &materialName, &categoryName,
	); err != nil {
		return nil, err
	}
	unitCost, err := moneyFromDB(s.sqlite, unitCostRaw)
	if err != nil {
		return nil, fmt.Errorf("decode order item unit cost: %w", err)
	}
	subtotal, err := moneyFromDB(s.sqlite, subtotalRaw)
	if err != nil {
		return nil, fmt.Errorf("decode order item subtotal: %w", err)
	}
	return &domain.OrderItem{
		ID:           id,
		OrderID:      orderID,
		ProductID:    productID,
		Quantity:     int(quantity),
		UnitCost:     unitCost,
		Subtotal:     subtotal,
		SupplierSKU:  nullString(supplierSKU),
		ProductName:  productName,
		ProductSKU:   productSKU,
		BrandName:    nullString(brandName),
		MaterialName: nullString(materialName),
		CategoryName: nullString(categoryName),
	}, nil
}

func dbOrderStatus(value string) string {
	return strings.ToUpper(value)
}

func apiOrderStatus(value string) string {
	return strings.ToLower(value)
}

func dbPurchasePaymentStatus(value string) string {
	return strings.ToUpper(value)
}

func apiPurchasePaymentStatus(value string) string {
	return strings.ToLower(value)
}
