package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

type SaleService struct {
	repository outbound.SaleRepository
	unitOfWork outbound.UnitOfWork
}

func NewSaleService(repository outbound.SaleRepository, unitOfWork outbound.UnitOfWork) *SaleService {
	return &SaleService{repository: repository, unitOfWork: unitOfWork}
}

var _ inbound.SaleUseCase = (*SaleService)(nil)
var _ inbound.ClientPaymentUseCase = (*SaleService)(nil)

func (s *SaleService) List(ctx context.Context, filter inbound.SaleFilter) (*inbound.SaleList, error) {
	if !domain.ValidPaymentStatus(filter.PaymentStatus) {
		return nil, fmt.Errorf("%w: invalid payment_status", domain.ErrValidation)
	}
	if !domain.ValidDeliveryStatus(filter.DeliveryStatus) {
		return nil, fmt.Errorf("%w: invalid delivery_status", domain.ErrValidation)
	}
	if filter.PaymentMethod != "" && !domain.ValidPaymentMethod(filter.PaymentMethod) {
		return nil, fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	items, hasNext, err := s.repository.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &inbound.SaleList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *SaleService) Get(ctx context.Context, id int64) (*domain.Sale, error) {
	sale, err := s.repository.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, &domain.NotFoundError{Entity: "Sale", ID: id}
	}
	return sale, err
}

func (s *SaleService) Stats(ctx context.Context) (*domain.SaleStats, error) {
	return s.repository.Stats(ctx)
}

func (s *SaleService) Create(ctx context.Context, input inbound.CreateSaleInput) (*domain.Sale, error) {
	if err := validateCreateSale(input); err != nil {
		return nil, err
	}

	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if input.ClientID != nil {
			if err := lockClient(ctx, repos, *input.ClientID); err != nil {
				return referenceError(err, "Client", *input.ClientID)
			}
		}

		ids := saleProductIDs(input.Items)
		stock, err := repos.Stock.LockForUpdate(ctx, ids)
		if err != nil {
			return err
		}
		products := make(map[int64]*domain.ProductDetail, len(ids))
		for _, id := range ids {
			if _, ok := stock[id]; !ok {
				return &domain.NotFoundError{Entity: "Product", ID: id}
			}
			product, err := repos.Products.Get(ctx, id)
			if err != nil {
				return referenceError(err, "Product", id)
			}
			products[id] = product
		}

		items := make([]*domain.SaleItem, 0, len(input.Items))
		total := decimal.Zero
		for _, item := range input.Items {
			available := int(stock[item.ProductID])
			if available < item.Quantity {
				return &domain.InsufficientStockError{
					SKU:       products[item.ProductID].SKU,
					Available: available,
					Requested: item.Quantity,
				}
			}
			unitPrice := products[item.ProductID].Price
			if item.UnitPrice != nil {
				unitPrice = *item.UnitPrice
			}
			unitPrice = unitPrice.Round(2)
			subtotal := unitPrice.Mul(decimal.NewFromInt(int64(item.Quantity))).Round(2)
			total = total.Add(subtotal)
			items = append(items, &domain.SaleItem{
				ProductID:         item.ProductID,
				Quantity:          item.Quantity,
				DeliveredQuantity: item.DeliveredQuantity,
				UnitPrice:         unitPrice,
				Subtotal:          subtotal,
			})
		}

		now := nowUTC()
		paid := input.AmountPaid.Round(2)
		if paid.GreaterThan(total) {
			return fmt.Errorf("%w: amount_paid cannot exceed total_amount", domain.ErrValidation)
		}
		if input.ClientID == nil && total.GreaterThan(paid) {
			return requiredClientForDebt()
		}

		for _, item := range items {
			if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(-item.Quantity)); err != nil {
				return stockError(err, products[item.ProductID].SKU, stock[item.ProductID], item.Quantity)
			}
		}

		sale := &domain.Sale{
			CreatedAt:      now,
			TotalAmount:    total,
			AmountPaid:     paid,
			DebtAmount:     debtAmount(total, paid),
			PaymentStatus:  derivePaymentStatus(total, paid),
			DeliveryStatus: deriveDeliveryStatus(items),
			ClientID:       input.ClientID,
			CreatedByID:    input.CreatedByID,
		}
		created, err := repos.Sales.Create(ctx, sale, items)
		if err != nil {
			return err
		}
		if paid.IsPositive() {
			if _, err := repos.Sales.CreatePayment(ctx, &domain.SalePayment{
				SaleID:        created.ID,
				Amount:        paid,
				PaymentMethod: input.PaymentMethod,
				CreatedAt:     now,
				CreatedByID:   &input.CreatedByID,
			}); err != nil {
				return err
			}
		}
		if err := recomputeClientDebt(ctx, repos, input.ClientID); err != nil {
			return err
		}
		result, err = repos.Sales.Get(ctx, created.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) Update(ctx context.Context, id int64, input inbound.UpdateSaleInput) (*domain.Sale, error) {
	if input.ClientID.Set && input.ClientID.Value != nil && *input.ClientID.Value <= 0 {
		return nil, fmt.Errorf("%w: client_id must be greater than zero", domain.ErrValidation)
	}
	if !input.ClientID.Set {
		return s.Get(ctx, id)
	}

	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		current, err := repos.Sales.Get(ctx, id)
		if err != nil {
			return referenceError(err, "Sale", id)
		}
		oldClientID := current.ClientID
		newClientID := input.ClientID.Value

		ids := make([]int64, 0, 2)
		if oldClientID != nil {
			ids = append(ids, *oldClientID)
		}
		if newClientID != nil {
			ids = append(ids, *newClientID)
		}
		for _, clientID := range sortedIDs(ids) {
			if err := lockClient(ctx, repos, clientID); err != nil {
				return referenceError(err, "Client", clientID)
			}
		}

		sale, err := repos.Sales.Lock(ctx, id)
		if err != nil {
			return referenceError(err, "Sale", id)
		}
		if !sameClientID(oldClientID, sale.ClientID) {
			return errClientChanged
		}
		if newClientID == nil && sale.DebtAmount.IsPositive() {
			return requiredClientForDebt()
		}
		sale.ClientID = newClientID
		sale.PaymentStatus = derivePaymentStatus(sale.TotalAmount, sale.AmountPaid)
		sale.DebtAmount = debtAmount(sale.TotalAmount, sale.AmountPaid)
		now := nowUTC()
		sale.UpdatedAt = &now
		if err := repos.Sales.Update(ctx, id, sale); err != nil {
			return err
		}
		if !sameClientID(oldClientID, newClientID) {
			if err := recomputeClientDebt(ctx, repos, oldClientID); err != nil {
				return err
			}
		}
		if err := recomputeClientDebt(ctx, repos, newClientID); err != nil {
			return err
		}
		result, err = repos.Sales.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) Delete(ctx context.Context, id int64) (*domain.Sale, error) {
	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		current, err := repos.Sales.Get(ctx, id)
		if err != nil {
			return referenceError(err, "Sale", id)
		}
		clientID := current.ClientID
		if clientID != nil {
			if err := lockClient(ctx, repos, *clientID); err != nil {
				return referenceError(err, "Client", *clientID)
			}
		}
		sale, err := repos.Sales.Lock(ctx, id)
		if err != nil {
			return referenceError(err, "Sale", id)
		}
		if !sameClientID(clientID, sale.ClientID) {
			return errClientChanged
		}
		items, err := repos.Sales.ListAllItems(ctx, id)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ProductID)
		}
		if _, err := repos.Stock.LockForUpdate(ctx, ids); err != nil {
			return err
		}
		for _, item := range items {
			if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(item.Quantity)); err != nil {
				return err
			}
		}
		if err := repos.Sales.DeleteItems(ctx, id); err != nil {
			return err
		}
		if err := repos.Sales.Delete(ctx, id); err != nil {
			return err
		}
		if err := recomputeClientDebt(ctx, repos, clientID); err != nil {
			return err
		}
		result = sale
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) ListItems(ctx context.Context, saleID int64, skip, limit int) (*inbound.SaleItemList, error) {
	if _, err := s.Get(ctx, saleID); err != nil {
		return nil, err
	}
	items, hasNext, err := s.repository.ListItems(ctx, saleID, skip, limit)
	if err != nil {
		return nil, err
	}
	return &inbound.SaleItemList{Items: items, HasNext: hasNext, Skip: skip, Limit: limit}, nil
}

func (s *SaleService) AddItem(ctx context.Context, saleID int64, input inbound.AddSaleItemInput) (*domain.Sale, error) {
	if err := validateSaleItem(input.ProductID, input.Quantity, input.DeliveredQuantity, input.UnitPrice); err != nil {
		return nil, err
	}

	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := lockSaleAndClient(ctx, repos, saleID); err != nil {
			return err
		}
		product, err := repos.Products.Get(ctx, input.ProductID)
		if err != nil {
			return referenceError(err, "Product", input.ProductID)
		}
		if _, err := repos.Sales.GetItemByProduct(ctx, saleID, input.ProductID); err == nil {
			return fmt.Errorf("%w: product with id %d already exists in this sale", domain.ErrConflict, input.ProductID)
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		stock, err := repos.Stock.LockForUpdate(ctx, []int64{input.ProductID})
		if err != nil {
			return err
		}
		available := int(stock[input.ProductID])
		if available < input.Quantity {
			return &domain.InsufficientStockError{
				SKU:       product.SKU,
				Available: available,
				Requested: input.Quantity,
			}
		}
		if err := repos.Stock.ApplyDelta(ctx, input.ProductID, int64(-input.Quantity)); err != nil {
			return stockError(err, product.SKU, stock[input.ProductID], input.Quantity)
		}

		unitPrice := product.Price
		if input.UnitPrice != nil {
			unitPrice = *input.UnitPrice
		}
		unitPrice = unitPrice.Round(2)
		item := &domain.SaleItem{
			SaleID:            saleID,
			ProductID:         input.ProductID,
			Quantity:          input.Quantity,
			DeliveredQuantity: input.DeliveredQuantity,
			UnitPrice:         unitPrice,
			Subtotal:          unitPrice.Mul(decimal.NewFromInt(int64(input.Quantity))).Round(2),
		}
		if _, err := repos.Sales.CreateItem(ctx, item); err != nil {
			return err
		}
		result, err = s.recalculate(ctx, repos, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) UpdateItem(ctx context.Context, saleID, itemID int64, input inbound.UpdateSaleItemInput) (*domain.Sale, error) {
	if input.Quantity.Set && (input.Quantity.Value == nil || *input.Quantity.Value <= 0) {
		return nil, fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation)
	}
	if input.DeliveredQuantity.Set && input.DeliveredQuantity.Value == nil {
		return nil, requiredField("delivered_quantity")
	}
	if input.UnitPrice.Set {
		if input.UnitPrice.Value == nil {
			return nil, requiredField("unit_price")
		}
		if input.UnitPrice.Value.IsNegative() {
			return nil, nonNegative("unit_price")
		}
	}

	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := lockSaleAndClient(ctx, repos, saleID); err != nil {
			return err
		}
		item, err := repos.Sales.GetItem(ctx, saleID, itemID)
		if err != nil {
			return referenceError(err, "Sale item", itemID)
		}

		newQuantity := item.Quantity
		if input.Quantity.Set {
			newQuantity = *input.Quantity.Value
		}
		newDelivered := item.DeliveredQuantity
		if input.DeliveredQuantity.Set {
			newDelivered = *input.DeliveredQuantity.Value
		}
		if newDelivered < 0 || newDelivered > newQuantity {
			return fmt.Errorf("%w: delivered_quantity cannot exceed quantity", domain.ErrValidation)
		}

		delta := newQuantity - item.Quantity
		if delta != 0 {
			stock, err := repos.Stock.LockForUpdate(ctx, []int64{item.ProductID})
			if err != nil {
				return err
			}
			if delta > 0 {
				available := int(stock[item.ProductID])
				if available < delta {
					return &domain.InsufficientStockError{
						SKU:       item.ProductSKU,
						Available: available,
						Requested: delta,
					}
				}
			}
			if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(-delta)); err != nil {
				return stockError(err, item.ProductSKU, stock[item.ProductID], delta)
			}
		}

		if input.UnitPrice.Set {
			item.UnitPrice = input.UnitPrice.Value.Round(2)
		}
		item.Quantity = newQuantity
		item.DeliveredQuantity = newDelivered
		item.Subtotal = item.UnitPrice.Mul(decimal.NewFromInt(int64(newQuantity))).Round(2)
		if err := repos.Sales.UpdateItem(ctx, item); err != nil {
			return err
		}
		result, err = s.recalculate(ctx, repos, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) DeleteItem(ctx context.Context, saleID, itemID int64) (*domain.Sale, error) {
	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := lockSaleAndClient(ctx, repos, saleID); err != nil {
			return err
		}
		item, err := repos.Sales.GetItem(ctx, saleID, itemID)
		if err != nil {
			return referenceError(err, "Sale item", itemID)
		}
		if _, err := repos.Stock.LockForUpdate(ctx, []int64{item.ProductID}); err != nil {
			return err
		}
		if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(item.Quantity)); err != nil {
			return err
		}
		if err := repos.Sales.DeleteItem(ctx, saleID, itemID); err != nil {
			return err
		}
		result, err = s.recalculate(ctx, repos, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) ListPayments(ctx context.Context, saleID int64, skip, limit int) (*inbound.SalePaymentList, error) {
	if _, err := s.Get(ctx, saleID); err != nil {
		return nil, err
	}
	items, hasNext, err := s.repository.ListPayments(ctx, saleID, skip, limit)
	if err != nil {
		return nil, err
	}
	return &inbound.SalePaymentList{Items: items, HasNext: hasNext, Skip: skip, Limit: limit}, nil
}

func (s *SaleService) CreatePayment(ctx context.Context, saleID int64, input inbound.CreateSalePaymentInput) (*domain.Sale, error) {
	if !domain.ValidPaymentMethod(input.PaymentMethod) {
		return nil, fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	amount := input.Amount.Round(2)
	if !amount.IsPositive() {
		return nil, fmt.Errorf("%w: amount must be greater than zero", domain.ErrValidation)
	}

	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		sale, err := lockSaleAndClient(ctx, repos, saleID)
		if err != nil {
			return err
		}
		remaining := debtAmount(sale.TotalAmount, sale.AmountPaid)
		if remaining.IsZero() {
			return fmt.Errorf("%w: sale is already paid", domain.ErrValidation)
		}
		if amount.GreaterThan(remaining) {
			return fmt.Errorf("%w: amount cannot exceed remaining debt", domain.ErrValidation)
		}
		now := nowUTC()
		if _, err := repos.Sales.CreatePayment(ctx, &domain.SalePayment{
			SaleID:        saleID,
			Amount:        amount,
			PaymentMethod: &input.PaymentMethod,
			CreatedAt:     now,
			CreatedByID:   &input.CreatedByID,
		}); err != nil {
			return err
		}
		if err := applyPaymentTotals(ctx, repos, sale, now); err != nil {
			return err
		}
		if err := recomputeClientDebt(ctx, repos, sale.ClientID); err != nil {
			return err
		}
		result, err = repos.Sales.Get(ctx, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) DeletePayment(ctx context.Context, saleID, paymentID int64) (*domain.Sale, error) {
	var result *domain.Sale
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		sale, err := lockSaleAndClient(ctx, repos, saleID)
		if err != nil {
			return err
		}
		if _, err := repos.Sales.GetPayment(ctx, saleID, paymentID); err != nil {
			return referenceError(err, "Sale payment", paymentID)
		}
		if err := repos.Sales.DeletePayment(ctx, saleID, paymentID); err != nil {
			return referenceError(err, "Sale payment", paymentID)
		}
		if err := applyPaymentTotals(ctx, repos, sale, nowUTC()); err != nil {
			return err
		}
		if err := recomputeClientDebt(ctx, repos, sale.ClientID); err != nil {
			return err
		}
		result, err = repos.Sales.Get(ctx, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) CreateClientPayment(ctx context.Context, input inbound.CreateClientPaymentInput) (*inbound.ClientPaymentResult, error) {
	if !domain.ValidPaymentMethod(input.PaymentMethod) {
		return nil, fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	amount := input.Amount
	if err := validateClientPaymentAmount(amount); err != nil {
		return nil, err
	}

	var result *inbound.ClientPaymentResult
	err := withinTxRetry(ctx, s.unitOfWork, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := lockClientEntity(ctx, repos, input.ClientID); err != nil {
			return referenceError(err, "Client", input.ClientID)
		}

		derived, err := repos.ClientDebt.DebtSum(ctx, input.ClientID)
		if err != nil {
			return err
		}
		if !derived.IsPositive() {
			return fmt.Errorf("%w: client has no outstanding debt", domain.ErrValidation)
		}
		if amount.GreaterThan(derived) {
			return fmt.Errorf("%w: amount cannot exceed client debt", domain.ErrValidation)
		}

		// Read at most one row past the cap; the plan rejects before any write.
		sales, err := repos.ClientDebt.DebtorSales(ctx, input.ClientID, maxClientPaymentSales+1)
		if err != nil {
			return err
		}
		remaining := amount
		plan := make([]inbound.ClientPaymentAllocation, 0, len(sales))
		for _, debtor := range sales {
			if !remaining.IsPositive() {
				break
			}
			saleDebt := debtAmount(debtor.TotalAmount, debtor.AmountPaid)
			if !saleDebt.IsPositive() {
				continue
			}
			allocated := decimal.Min(remaining, saleDebt)
			plan = append(plan, inbound.ClientPaymentAllocation{SaleID: debtor.ID, Amount: allocated})
			remaining = remaining.Sub(allocated)
		}
		if len(plan) > maxClientPaymentSales {
			return fmt.Errorf("%w: payment affects more than %d open sales", domain.ErrValidation, maxClientPaymentSales)
		}
		if remaining.IsPositive() {
			return fmt.Errorf("%w: amount cannot exceed client debt", domain.ErrValidation)
		}

		now := nowUTC()
		for _, planned := range plan {
			sale, err := repos.Sales.Lock(ctx, planned.SaleID)
			if err != nil {
				return referenceError(err, "Sale", planned.SaleID)
			}
			saleDebt := debtAmount(sale.TotalAmount, sale.AmountPaid)
			if !saleDebt.IsPositive() || saleDebt.LessThan(planned.Amount) {
				return errClientChanged
			}
			if _, err := repos.Sales.CreatePayment(ctx, &domain.SalePayment{
				SaleID:        sale.ID,
				Amount:        planned.Amount,
				PaymentMethod: &input.PaymentMethod,
				CreatedAt:     now,
				CreatedByID:   &input.CreatedByID,
			}); err != nil {
				return err
			}
			if err := applyPaymentTotals(ctx, repos, sale, now); err != nil {
				return err
			}
		}

		if err := recomputeClientDebt(ctx, repos, &input.ClientID); err != nil {
			return err
		}
		updated, err := repos.Clients.Get(ctx, input.ClientID)
		if err != nil {
			return err
		}
		result = &inbound.ClientPaymentResult{
			ClientID:      input.ClientID,
			Amount:        amount,
			RemainingDebt: updated.DebtAmount,
			Allocations:   plan,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateClientPaymentAmount(amount decimal.Decimal) error {
	if amount.Exponent() < -2 {
		return fmt.Errorf("%w: amount must have at most two decimals", domain.ErrValidation)
	}
	if !amount.IsPositive() {
		return fmt.Errorf("%w: amount must be greater than zero", domain.ErrValidation)
	}
	if amount.GreaterThan(maxClientPaymentAmount) {
		return fmt.Errorf("%w: amount exceeds the maximum allowed value", domain.ErrValidation)
	}
	return nil
}

func applyPaymentTotals(ctx context.Context, repos outbound.TxRepos, sale *domain.Sale, now time.Time) error {
	paid, err := repos.Sales.SumPayments(ctx, sale.ID)
	if err != nil {
		return err
	}
	sale.AmountPaid = paid.Round(2)
	sale.PaymentStatus = derivePaymentStatus(sale.TotalAmount, sale.AmountPaid)
	sale.DebtAmount = debtAmount(sale.TotalAmount, sale.AmountPaid)
	sale.UpdatedAt = &now
	return repos.Sales.Update(ctx, sale.ID, sale)
}

func (s *SaleService) recalculate(ctx context.Context, repos outbound.TxRepos, saleID int64) (*domain.Sale, error) {
	sale, err := repos.Sales.Lock(ctx, saleID)
	if err != nil {
		return nil, err
	}
	items, err := repos.Sales.ListAllItems(ctx, saleID)
	if err != nil {
		return nil, err
	}
	sale.TotalAmount = saleTotal(items)
	if len(items) == 0 {
		sale.PaymentStatus = domain.PaymentStatusPending
		sale.DeliveryStatus = domain.DeliveryStatusPending
	} else {
		sale.PaymentStatus = derivePaymentStatus(sale.TotalAmount, sale.AmountPaid)
		sale.DeliveryStatus = deriveDeliveryStatus(items)
	}
	sale.DebtAmount = debtAmount(sale.TotalAmount, sale.AmountPaid)
	if sale.ClientID == nil && sale.DebtAmount.IsPositive() {
		return nil, requiredClientForDebt()
	}
	now := nowUTC()
	sale.UpdatedAt = &now
	if err := repos.Sales.Update(ctx, saleID, sale); err != nil {
		return nil, err
	}
	if err := recomputeClientDebt(ctx, repos, sale.ClientID); err != nil {
		return nil, err
	}
	return repos.Sales.Get(ctx, saleID)
}

func validateCreateSale(input inbound.CreateSaleInput) error {
	if len(input.Items) == 0 {
		return fmt.Errorf("%w: sale must have at least one item", domain.ErrValidation)
	}
	if input.PaymentMethod != nil && !domain.ValidPaymentMethod(*input.PaymentMethod) {
		return fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	if input.AmountPaid.IsNegative() {
		return nonNegative("amount_paid")
	}
	roundedPaid := input.AmountPaid.Round(2)
	if roundedPaid.IsPositive() && input.PaymentMethod == nil {
		return requiredField("payment_method")
	}
	if !roundedPaid.IsPositive() && input.PaymentMethod != nil {
		return fmt.Errorf("%w: payment_method is not allowed when amount_paid is zero", domain.ErrValidation)
	}
	if input.ClientID != nil && *input.ClientID <= 0 {
		return fmt.Errorf("%w: client_id must be greater than zero", domain.ErrValidation)
	}
	seen := make(map[int64]struct{}, len(input.Items))
	for _, item := range input.Items {
		if err := validateSaleItem(item.ProductID, item.Quantity, item.DeliveredQuantity, item.UnitPrice); err != nil {
			return err
		}
		if _, ok := seen[item.ProductID]; ok {
			return fmt.Errorf("%w: one item per product is allowed", domain.ErrValidation)
		}
		seen[item.ProductID] = struct{}{}
	}
	return nil
}

func validateSaleItem(productID int64, quantity, delivered int, unitPrice *decimal.Decimal) error {
	if productID <= 0 {
		return fmt.Errorf("%w: product_id must be greater than zero", domain.ErrValidation)
	}
	if quantity <= 0 {
		return fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation)
	}
	if delivered < 0 || delivered > quantity {
		return fmt.Errorf("%w: delivered_quantity cannot exceed quantity", domain.ErrValidation)
	}
	if unitPrice != nil && unitPrice.IsNegative() {
		return nonNegative("unit_price")
	}
	return nil
}

func saleProductIDs(items []inbound.CreateSaleItemInput) []int64 {
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ProductID
	}
	return sortedIDs(ids)
}

func sortedIDs(ids []int64) []int64 {
	result := slices.Clone(ids)
	slices.Sort(result)
	return slices.Compact(result)
}

func referenceError(err error, entity string, id int64) error {
	if errors.Is(err, domain.ErrNotFound) {
		return &domain.NotFoundError{Entity: entity, ID: id}
	}
	return err
}

var (
	errClientChanged     = errors.New("sale client changed during transaction")
	errMissingClientDebt = errors.New("transaction repositories missing client debt store")
)

const (
	saleTxAttempts        = 3
	maxClientPaymentSales = 1000
)

var maxClientPaymentAmount = decimal.RequireFromString(domain.MaxMoneyAmount)

func withinTxRetry(ctx context.Context, unitOfWork outbound.UnitOfWork, fn func(context.Context, outbound.TxRepos) error) error {
	var err error
	for attempt := 0; attempt < saleTxAttempts; attempt++ {
		err = unitOfWork.WithinTx(ctx, fn)
		if !errors.Is(err, errClientChanged) {
			return err
		}
	}
	return err
}

func sameClientID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func lockClient(ctx context.Context, repos outbound.TxRepos, id int64) error {
	_, err := lockClientEntity(ctx, repos, id)
	return err
}

func lockClientEntity(ctx context.Context, repos outbound.TxRepos, id int64) (*domain.Client, error) {
	if repos.ClientDebt == nil {
		return nil, errMissingClientDebt
	}
	return repos.ClientDebt.Lock(ctx, id)
}

// lockSaleAndClient keeps the client -> sale lock order. The non-locking read
// finds the client before the sale row is locked; if a concurrent reassignment
// changed it, the transaction retries with the new client.
func lockSaleAndClient(ctx context.Context, repos outbound.TxRepos, saleID int64) (*domain.Sale, error) {
	current, err := repos.Sales.Get(ctx, saleID)
	if err != nil {
		return nil, referenceError(err, "Sale", saleID)
	}
	if current.ClientID != nil {
		if err := lockClient(ctx, repos, *current.ClientID); err != nil {
			return nil, referenceError(err, "Client", *current.ClientID)
		}
	}
	sale, err := repos.Sales.Lock(ctx, saleID)
	if err != nil {
		return nil, referenceError(err, "Sale", saleID)
	}
	if !sameClientID(current.ClientID, sale.ClientID) {
		return nil, errClientChanged
	}
	return sale, nil
}

func recomputeClientDebt(ctx context.Context, repos outbound.TxRepos, clientID *int64) error {
	if clientID == nil {
		return nil
	}
	if repos.ClientDebt == nil {
		return errMissingClientDebt
	}
	return repos.ClientDebt.SetClientDebt(ctx, *clientID)
}

func requiredClientForDebt() error {
	return fmt.Errorf("%w: client_id is required when the sale has an outstanding balance", domain.ErrValidation)
}

func stockError(err error, sku string, available int64, requested int) error {
	if errors.Is(err, domain.ErrInsufficientStock) {
		return &domain.InsufficientStockError{
			SKU:       sku,
			Available: int(available),
			Requested: requested,
		}
	}
	return err
}

func derivePaymentStatus(total, paid decimal.Decimal) string {
	if total.IsZero() && paid.IsZero() {
		return domain.PaymentStatusPending
	}
	switch {
	case paid.GreaterThanOrEqual(total):
		return domain.PaymentStatusPaid
	case paid.IsPositive():
		return domain.PaymentStatusPartial
	default:
		return domain.PaymentStatusPending
	}
}

func deriveDeliveryStatus(items []*domain.SaleItem) string {
	if len(items) == 0 {
		return domain.DeliveryStatusPending
	}
	allDelivered := true
	anyDelivered := false
	for _, item := range items {
		if item.DeliveredQuantity >= item.Quantity {
			anyDelivered = true
		} else {
			allDelivered = false
			if item.DeliveredQuantity > 0 {
				anyDelivered = true
			}
		}
	}
	if allDelivered {
		return domain.DeliveryStatusDelivered
	}
	if anyDelivered {
		return domain.DeliveryStatusPartial
	}
	return domain.DeliveryStatusPending
}

func saleTotal(items []*domain.SaleItem) decimal.Decimal {
	total := decimal.Zero
	for _, item := range items {
		total = total.Add(item.Subtotal)
	}
	return total
}

func debtAmount(total, paid decimal.Decimal) decimal.Decimal {
	debt := total.Sub(paid)
	if debt.IsNegative() {
		return decimal.Zero
	}
	return debt
}
