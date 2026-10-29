package gormstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type saleStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.SaleRepository = (*saleStore)(nil)

func NewSaleStore(db *gorm.DB) outbound.SaleRepository {
	return &saleStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *saleStore) Get(ctx context.Context, id int64) (*domain.Sale, error) {
	row := s.db.WithContext(ctx).Raw(s.selectSQL()+" WHERE s.id = ?", id).Row()
	sale, err := s.scanSale(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get sale: %w", err)
	}
	if err := s.attachRelations(ctx, []*domain.Sale{sale}); err != nil {
		return nil, err
	}
	return sale, nil
}

func (s *saleStore) List(ctx context.Context, filter outbound.SaleFilter) ([]*domain.Sale, bool, error) {
	where, args := s.filterSQL(filter)
	query := s.selectSQL() + where + " ORDER BY s.id DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit+1, filter.Skip)

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("list sales: %w", err)
	}
	sales, err := s.scanSales(rows)
	if err != nil {
		return nil, false, fmt.Errorf("list sales: %w", err)
	}
	if err := s.attachRelations(ctx, sales); err != nil {
		return nil, false, err
	}

	hasNext := len(sales) > filter.Limit
	if hasNext {
		sales = sales[:filter.Limit]
	}
	return sales, hasNext, nil
}

func (s *saleStore) Stats(ctx context.Context) (*domain.SaleStats, error) {
	var (
		unpaid        int64
		undelivered   int64
		totalRaw      any
		amountPaidRaw any
	)
	err := s.db.WithContext(ctx).Raw(
		`SELECT
			COALESCE(SUM(CASE WHEN payment_status <> 'PAID' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN delivery_status <> 'DELIVERED' THEN 1 ELSE 0 END), 0),
			`+s.moneySQL("COALESCE(SUM(total_amount), 0)")+`,
			`+s.moneySQL("COALESCE(SUM(amount_paid), 0)")+`
		FROM sales`,
	).Row().Scan(&unpaid, &undelivered, &totalRaw, &amountPaidRaw)
	if err != nil {
		return nil, fmt.Errorf("sale stats: %w", err)
	}
	total, err := s.moneyFromDB(totalRaw)
	if err != nil {
		return nil, fmt.Errorf("sale stats total: %w", err)
	}
	paid, err := s.moneyFromDB(amountPaidRaw)
	if err != nil {
		return nil, fmt.Errorf("sale stats paid: %w", err)
	}
	return &domain.SaleStats{
		UnpaidCount:      unpaid,
		UndeliveredCount: undelivered,
		TotalAmountSum:   total,
		AmountPaidSum:    paid,
	}, nil
}

func (s *saleStore) Create(ctx context.Context, sale *domain.Sale, items []*domain.SaleItem) (*domain.Sale, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(
		`INSERT INTO sales
			(created_at, updated_at, total_amount, amount_paid, payment_status, delivery_status, payment_method, client_id, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		Timestamp(sale.CreatedAt),
		nil,
		s.moneyToDB(sale.TotalAmount),
		s.moneyToDB(sale.AmountPaid),
		dbPaymentStatus(sale.PaymentStatus),
		dbDeliveryStatus(sale.DeliveryStatus),
		dbPaymentMethod(sale.PaymentMethod),
		sale.ClientID,
		nullableID(sale.CreatedByID),
	).Scan(&id).Error
	if err != nil {
		return nil, storeError("create sale", err)
	}
	sale.ID = id
	for _, item := range items {
		item.SaleID = id
		if _, err := s.CreateItem(ctx, item); err != nil {
			return nil, err
		}
	}
	return sale, nil
}

func (s *saleStore) Update(ctx context.Context, id int64, sale *domain.Sale) error {
	result := s.db.WithContext(ctx).Exec(
		`UPDATE sales SET
			updated_at = ?, total_amount = ?, amount_paid = ?, payment_status = ?,
			delivery_status = ?, payment_method = ?, client_id = ?
		 WHERE id = ?`,
		nullableTimestampValue(sale.UpdatedAt),
		s.moneyToDB(sale.TotalAmount),
		s.moneyToDB(sale.AmountPaid),
		dbPaymentStatus(sale.PaymentStatus),
		dbDeliveryStatus(sale.DeliveryStatus),
		dbPaymentMethod(sale.PaymentMethod),
		sale.ClientID,
		id,
	)
	if result.Error != nil {
		return storeError("update sale", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *saleStore) Delete(ctx context.Context, id int64) error {
	result := s.db.WithContext(ctx).Exec("DELETE FROM sales WHERE id = ?", id)
	if result.Error != nil {
		return storeError("delete sale", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *saleStore) ListItems(ctx context.Context, saleID int64, skip, limit int) ([]*domain.SaleItem, bool, error) {
	query := s.itemsSQL() + " WHERE si.sale_id = ? ORDER BY si.id ASC LIMIT ? OFFSET ?"
	rows, err := s.db.WithContext(ctx).Raw(query, saleID, limit+1, skip).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("list sale items: %w", err)
	}
	items, err := s.scanItems(rows)
	if err != nil {
		return nil, false, fmt.Errorf("list sale items: %w", err)
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return items, hasNext, nil
}

func (s *saleStore) ListAllItems(ctx context.Context, saleID int64) ([]*domain.SaleItem, error) {
	rows, err := s.db.WithContext(ctx).Raw(s.itemsSQL()+" WHERE si.sale_id = ? ORDER BY si.id ASC", saleID).Rows()
	if err != nil {
		return nil, fmt.Errorf("load sale items: %w", err)
	}
	items, err := s.scanItems(rows)
	if err != nil {
		return nil, fmt.Errorf("load sale items: %w", err)
	}
	return items, nil
}

func (s *saleStore) GetItem(ctx context.Context, saleID, itemID int64) (*domain.SaleItem, error) {
	row := s.db.WithContext(ctx).Raw(
		s.itemsSQL()+" WHERE si.sale_id = ? AND si.id = ?", saleID, itemID,
	).Row()
	item, err := s.scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get sale item: %w", err)
	}
	return item, nil
}

func (s *saleStore) GetItemByProduct(ctx context.Context, saleID, productID int64) (*domain.SaleItem, error) {
	row := s.db.WithContext(ctx).Raw(
		s.itemsSQL()+" WHERE si.sale_id = ? AND si.product_id = ?", saleID, productID,
	).Row()
	item, err := s.scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get sale item by product: %w", err)
	}
	return item, nil
}

func (s *saleStore) CreateItem(ctx context.Context, item *domain.SaleItem) (*domain.SaleItem, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(
		`INSERT INTO sale_items
			(sale_id, product_id, quantity, delivered_quantity, unit_price, subtotal)
		 VALUES (?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		item.SaleID,
		item.ProductID,
		item.Quantity,
		item.DeliveredQuantity,
		s.moneyToDB(item.UnitPrice),
		s.moneyToDB(item.Subtotal),
	).Scan(&id).Error
	if err != nil {
		return nil, storeError("create sale item", err)
	}
	item.ID = id
	return item, nil
}

func (s *saleStore) UpdateItem(ctx context.Context, item *domain.SaleItem) error {
	result := s.db.WithContext(ctx).Exec(
		`UPDATE sale_items SET
			quantity = ?, delivered_quantity = ?, unit_price = ?, subtotal = ?
		 WHERE id = ? AND sale_id = ?`,
		item.Quantity,
		item.DeliveredQuantity,
		s.moneyToDB(item.UnitPrice),
		s.moneyToDB(item.Subtotal),
		item.ID,
		item.SaleID,
	)
	if result.Error != nil {
		return storeError("update sale item", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *saleStore) DeleteItem(ctx context.Context, saleID, itemID int64) error {
	result := s.db.WithContext(ctx).Exec("DELETE FROM sale_items WHERE id = ? AND sale_id = ?", itemID, saleID)
	if result.Error != nil {
		return storeError("delete sale item", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *saleStore) DeleteItems(ctx context.Context, saleID int64) error {
	if err := s.db.WithContext(ctx).Exec("DELETE FROM sale_items WHERE sale_id = ?", saleID).Error; err != nil {
		return storeError("delete sale items", err)
	}
	return nil
}

func (s *saleStore) selectSQL() string {
	return `SELECT
		s.id, s.created_at, s.updated_at, ` + s.moneySQL("s.total_amount") + `, ` + s.moneySQL("s.amount_paid") + `,
		s.payment_status, s.delivery_status, s.payment_method, s.client_id, s.created_by_id
	FROM sales s`
}

func (s *saleStore) itemsSQL() string {
	return `SELECT
		si.id, si.sale_id, si.product_id, si.quantity, si.delivered_quantity,
		` + s.moneySQL("si.unit_price") + `, ` + s.moneySQL("si.subtotal") + `,
		p.name, p.sku, b.name, m.name, c.name
	FROM sale_items si
	JOIN products p ON p.id = si.product_id
	LEFT JOIN brands b ON b.id = p.brand_id
	LEFT JOIN materials m ON m.id = p.material_id
	LEFT JOIN categories c ON c.id = p.category_id`
}

func (s *saleStore) moneySQL(column string) string {
	if s.sqlite {
		return column
	}
	return column + "::text"
}

func (s *saleStore) filterSQL(filter outbound.SaleFilter) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if filter.UserID != nil {
		conditions = append(conditions, "s.created_by_id = ?")
		args = append(args, *filter.UserID)
	}
	if filter.ClientID != nil {
		conditions = append(conditions, "s.client_id = ?")
		args = append(args, *filter.ClientID)
	}
	if filter.NoClient {
		conditions = append(conditions, "s.client_id IS NULL")
	}
	if filter.StartDate != nil {
		conditions = append(conditions, "s.created_at >= ?")
		args = append(args, Timestamp(*filter.StartDate))
	}
	if filter.EndDate != nil {
		conditions = append(conditions, "s.created_at <= ?")
		args = append(args, Timestamp(*filter.EndDate))
	}
	if filter.PaymentStatus != "" {
		conditions = append(conditions, "s.payment_status = ?")
		args = append(args, dbPaymentStatus(filter.PaymentStatus))
	}
	if filter.DeliveryStatus != "" {
		conditions = append(conditions, "s.delivery_status = ?")
		args = append(args, dbDeliveryStatus(filter.DeliveryStatus))
	}
	if filter.PaymentMethod != "" {
		conditions = append(conditions, "s.payment_method = ?")
		args = append(args, dbPaymentMethod(&filter.PaymentMethod))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func (s *saleStore) scanSales(rows *sql.Rows) ([]*domain.Sale, error) {
	defer rows.Close()
	sales := make([]*domain.Sale, 0)
	for rows.Next() {
		sale, err := s.scanSale(rows)
		if err != nil {
			return nil, err
		}
		sales = append(sales, sale)
	}
	return sales, rows.Err()
}

func (s *saleStore) scanSale(row rowScanner) (*domain.Sale, error) {
	var (
		id             int64
		createdAt      Timestamp
		updatedAt      nullableTimestamp
		totalRaw       any
		paidRaw        any
		paymentStatus  string
		deliveryStatus string
		paymentMethod  sql.NullString
		clientID       sql.NullInt64
		createdByID    sql.NullInt64
	)
	err := row.Scan(
		&id, &createdAt, &updatedAt, &totalRaw, &paidRaw,
		&paymentStatus, &deliveryStatus, &paymentMethod, &clientID, &createdByID,
	)
	if err != nil {
		return nil, err
	}
	total, err := s.moneyFromDB(totalRaw)
	if err != nil {
		return nil, fmt.Errorf("decode sale total: %w", err)
	}
	paid, err := s.moneyFromDB(paidRaw)
	if err != nil {
		return nil, fmt.Errorf("decode sale amount paid: %w", err)
	}
	debt := total.Sub(paid)
	if debt.IsNegative() {
		debt = decimal.Zero
	}
	sale := &domain.Sale{
		ID:             id,
		CreatedAt:      createdAt.Time(),
		UpdatedAt:      updatedAt.timePtr(),
		TotalAmount:    total,
		AmountPaid:     paid,
		DebtAmount:     debt,
		PaymentStatus:  apiPaymentStatus(paymentStatus),
		DeliveryStatus: apiDeliveryStatus(deliveryStatus),
		PaymentMethod:  apiPaymentMethod(paymentMethod),
		ClientID:       nullableInt64Ptr(clientID),
	}
	if createdByID.Valid {
		sale.CreatedByID = createdByID.Int64
	}
	return sale, nil
}

func (s *saleStore) scanItems(rows *sql.Rows) ([]*domain.SaleItem, error) {
	defer rows.Close()
	items := make([]*domain.SaleItem, 0)
	for rows.Next() {
		item, err := s.scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *saleStore) scanItem(row rowScanner) (*domain.SaleItem, error) {
	var (
		id                int64
		saleID            int64
		productID         int64
		quantity          int64
		deliveredQuantity int64
		unitPriceRaw      any
		subtotalRaw       any
		productName       string
		productSKU        string
		brandName         sql.NullString
		materialName      sql.NullString
		categoryName      sql.NullString
	)
	err := row.Scan(
		&id, &saleID, &productID, &quantity, &deliveredQuantity,
		&unitPriceRaw, &subtotalRaw,
		&productName, &productSKU, &brandName, &materialName, &categoryName,
	)
	if err != nil {
		return nil, err
	}
	unitPrice, err := s.moneyFromDB(unitPriceRaw)
	if err != nil {
		return nil, fmt.Errorf("decode sale item unit price: %w", err)
	}
	subtotal, err := s.moneyFromDB(subtotalRaw)
	if err != nil {
		return nil, fmt.Errorf("decode sale item subtotal: %w", err)
	}
	return &domain.SaleItem{
		ID:                id,
		SaleID:            saleID,
		ProductID:         productID,
		Quantity:          int(quantity),
		DeliveredQuantity: int(deliveredQuantity),
		UnitPrice:         unitPrice,
		Subtotal:          subtotal,
		ProductName:       productName,
		ProductSKU:        productSKU,
		BrandName:         nullString(brandName),
		MaterialName:      nullString(materialName),
		CategoryName:      nullString(categoryName),
	}, nil
}

func (s *saleStore) attachRelations(ctx context.Context, sales []*domain.Sale) error {
	if len(sales) == 0 {
		return nil
	}
	userIDs := make([]int64, 0, len(sales))
	clientIDs := make([]int64, 0, len(sales))
	seenUsers := map[int64]struct{}{}
	seenClients := map[int64]struct{}{}
	for _, sale := range sales {
		if sale.CreatedByID > 0 {
			if _, ok := seenUsers[sale.CreatedByID]; !ok {
				seenUsers[sale.CreatedByID] = struct{}{}
				userIDs = append(userIDs, sale.CreatedByID)
			}
		}
		if sale.ClientID != nil {
			if _, ok := seenClients[*sale.ClientID]; !ok {
				seenClients[*sale.ClientID] = struct{}{}
				clientIDs = append(clientIDs, *sale.ClientID)
			}
		}
	}

	users := make(map[int64]*domain.User, len(userIDs))
	if len(userIDs) > 0 {
		var records []userModel
		if err := s.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&records).Error; err != nil {
			return fmt.Errorf("load sale users: %w", err)
		}
		for i := range records {
			users[records[i].ID] = records[i].toDomain()
		}
	}

	clients := make(map[int64]*domain.Client, len(clientIDs))
	if len(clientIDs) > 0 {
		var records []clientModel
		if err := s.db.WithContext(ctx).Where("id IN ?", clientIDs).Find(&records).Error; err != nil {
			return fmt.Errorf("load sale clients: %w", err)
		}
		for i := range records {
			clients[records[i].ID] = records[i].toDomain()
		}
	}

	for _, sale := range sales {
		if sale.CreatedByID > 0 {
			sale.CreatedBy = users[sale.CreatedByID]
		}
		if sale.ClientID != nil {
			sale.Client = clients[*sale.ClientID]
		}
	}
	return nil
}

func (s *saleStore) moneyToDB(value decimal.Decimal) any {
	rounded := value.Round(2)
	if s.sqlite {
		return rounded.Shift(2).IntPart()
	}
	return rounded.StringFixed(2)
}

func (s *saleStore) moneyFromDB(value any) (decimal.Decimal, error) {
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

func dbPaymentStatus(value string) string {
	switch value {
	case domain.PaymentStatusPending:
		return "PENDING"
	case domain.PaymentStatusPartial:
		return "PARTIAL"
	case domain.PaymentStatusPaid:
		return "PAID"
	default:
		return value
	}
}

func apiPaymentStatus(value string) string {
	return strings.ToLower(value)
}

func dbDeliveryStatus(value string) string {
	switch value {
	case domain.DeliveryStatusPending:
		return "PENDING"
	case domain.DeliveryStatusPartial:
		return "PARTIAL"
	case domain.DeliveryStatusDelivered:
		return "DELIVERED"
	default:
		return value
	}
}

func apiDeliveryStatus(value string) string {
	return strings.ToLower(value)
}

func dbPaymentMethod(value *string) any {
	if value == nil {
		return nil
	}
	switch *value {
	case domain.PaymentMethodCash:
		return "CASH"
	case domain.PaymentMethodCredit:
		return "CREDIT"
	case domain.PaymentMethodDebit:
		return "DEBIT"
	default:
		return *value
	}
}

func apiPaymentMethod(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	method := strings.ToLower(value.String)
	return &method
}

func nullableID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	cloned := value.Int64
	return &cloned
}

type nullableTimestamp struct {
	value Timestamp
	valid bool
}

func (n *nullableTimestamp) Scan(value any) error {
	if value == nil {
		n.valid = false
		return nil
	}
	n.valid = true
	return n.value.Scan(value)
}

func (n nullableTimestamp) timePtr() *time.Time {
	if !n.valid {
		return nil
	}
	value := n.value.Time()
	return &value
}

func nullableTimestampValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return Timestamp(*value)
}
