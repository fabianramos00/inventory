package application

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

func ptrDecimal(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}

func ptrInt64(value int64) *int64 {
	return &value
}

func ptrString(value string) *string {
	return &value
}

func TestSaleCreateTotalsStatusesAndStock(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10.50", 5)
	env.addProduct(2, "SKU-2", "2.25", 5)
	env.addUser(7, "seller@example.com")
	env.addClient(9, "Ana")
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		PaymentMethod: ptrString(domain.PaymentMethodCash),
		AmountPaid:    decimal.RequireFromString("10.50"),
		ClientID:      ptrInt64(9),
		CreatedByID:   7,
		Items: []inbound.CreateSaleItemInput{
			{ProductID: 1, Quantity: 2, DeliveredQuantity: 2},
			{ProductID: 2, Quantity: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sale.TotalAmount.Equal(decimal.RequireFromString("23.25")) {
		t.Fatalf("total = %s, want 23.25", sale.TotalAmount)
	}
	if !sale.AmountPaid.Equal(decimal.RequireFromString("10.50")) {
		t.Fatalf("paid = %s, want 10.50", sale.AmountPaid)
	}
	if !sale.DebtAmount.Equal(decimal.RequireFromString("12.75")) {
		t.Fatalf("debt = %s, want 12.75", sale.DebtAmount)
	}
	if sale.PaymentStatus != domain.PaymentStatusPartial {
		t.Fatalf("payment status = %q, want partial", sale.PaymentStatus)
	}
	if sale.DeliveryStatus != domain.DeliveryStatusPartial {
		t.Fatalf("delivery status = %q, want partial", sale.DeliveryStatus)
	}
	if sale.CreatedByID != 7 || sale.CreatedBy == nil || sale.CreatedBy.Email != "seller@example.com" {
		t.Fatalf("created_by = %+v / id %d", sale.CreatedBy, sale.CreatedByID)
	}
	if sale.Client == nil || sale.Client.Name != "Ana" {
		t.Fatalf("client = %+v", sale.Client)
	}
	requireSaleTime(t, sale.CreatedAt)
	if sale.UpdatedAt != nil {
		t.Fatalf("updated_at = %v, want nil", sale.UpdatedAt)
	}
	if env.stock[1] != 3 || env.stock[2] != 4 {
		t.Fatalf("stock = %d/%d, want 3/4", env.stock[1], env.stock[2])
	}

	payments, err := service.ListPayments(ctx, sale.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(payments.Items) != 1 {
		t.Fatalf("initial payments = %d, want 1", len(payments.Items))
	}
	initial := payments.Items[0]
	if !initial.Amount.Equal(decimal.RequireFromString("10.50")) {
		t.Fatalf("initial payment amount = %s, want 10.50", initial.Amount)
	}
	if initial.PaymentMethod == nil || *initial.PaymentMethod != domain.PaymentMethodCash {
		t.Fatalf("initial payment method = %v", initial.PaymentMethod)
	}
	if initial.CreatedByID == nil || *initial.CreatedByID != 7 || initial.CreatedBy == nil {
		t.Fatalf("initial payment creator = %v / %+v", initial.CreatedByID, initial.CreatedBy)
	}

	_, err = service.Create(ctx, inbound.CreateSaleInput{
		PaymentMethod: ptrString(domain.PaymentMethodCash),
		AmountPaid:    decimal.RequireFromString("100"),
		CreatedByID:   7,
		Items:         []inbound.CreateSaleItemInput{{ProductID: 2, Quantity: 1}},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("overpaid create error = %v, want ErrValidation", err)
	}
	if env.stock[2] != 4 {
		t.Fatalf("overpaid create changed stock to %d", env.stock[2])
	}
}

func TestSaleCreateValidation(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	service := env.service()

	cases := []struct {
		name  string
		input inbound.CreateSaleInput
	}{
		{"no items", inbound.CreateSaleInput{}},
		{"zero quantity", inbound.CreateSaleInput{Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 0}}}},
		{"delivered over quantity", inbound.CreateSaleInput{Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1, DeliveredQuantity: 2}}}},
		{"negative delivered", inbound.CreateSaleInput{Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1, DeliveredQuantity: -1}}}},
		{"negative amount", inbound.CreateSaleInput{AmountPaid: decimal.RequireFromString("-1"), Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}}}},
		{"invalid method", inbound.CreateSaleInput{PaymentMethod: ptrString("cheque"), Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}}}},
		{"empty method", inbound.CreateSaleInput{PaymentMethod: ptrString(""), Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}}}},
		{"method with zero amount", inbound.CreateSaleInput{PaymentMethod: ptrString(domain.PaymentMethodCash), Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}}}},
		{"positive amount without method", inbound.CreateSaleInput{AmountPaid: decimal.RequireFromString("1"), Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}}}},
		{"duplicate product", inbound.CreateSaleInput{Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}, {ProductID: 1, Quantity: 1}}}},
		{"negative unit price", inbound.CreateSaleInput{Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1, UnitPrice: ptrDecimal("-0.01")}}}},
		{"invalid client", inbound.CreateSaleInput{ClientID: ptrInt64(0), Items: []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Create(ctx, tc.input); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestSaleCreateInsufficientStockRollsBack(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 1)
	env.addProduct(2, "SKU-2", "10", 5)
	service := env.service()

	_, err := service.Create(ctx, inbound.CreateSaleInput{
		CreatedByID: 1,
		Items: []inbound.CreateSaleItemInput{
			{ProductID: 1, Quantity: 2},
			{ProductID: 2, Quantity: 1},
		},
	})
	var stockErr *domain.InsufficientStockError
	if !errors.As(err, &stockErr) {
		t.Fatalf("error = %v, want InsufficientStockError", err)
	}
	if stockErr.SKU != "SKU-1" || stockErr.Available != 1 || stockErr.Requested != 2 {
		t.Fatalf("stock error = %+v", stockErr)
	}
	if env.stock[1] != 1 || env.stock[2] != 5 {
		t.Fatalf("stock after failure = %d/%d, want unchanged 1/5", env.stock[1], env.stock[2])
	}
	if env.createdSales != 0 {
		t.Fatalf("sales created = %d, want 0", env.createdSales)
	}
}

func TestSaleCreateUnknownReferences(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	service := env.service()

	_, err := service.Create(ctx, inbound.CreateSaleInput{
		Items: []inbound.CreateSaleItemInput{{ProductID: 99, Quantity: 1}},
	})
	var notFound *domain.NotFoundError
	if !errors.As(err, &notFound) || notFound.Error() != "Product with id 99 not found" {
		t.Fatalf("product error = %v", err)
	}

	_, err = service.Create(ctx, inbound.CreateSaleInput{
		ClientID: ptrInt64(99),
		Items:    []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}},
	})
	if !errors.As(err, &notFound) || notFound.Error() != "Client with id 99 not found" {
		t.Fatalf("client error = %v", err)
	}
}

func TestSaleUpdateClientAndTimestamp(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	env.addClient(2, "Ana")
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		ClientID:    ptrInt64(2),
		CreatedByID: 1,
		Items:       []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.Update(ctx, sale.ID, inbound.UpdateSaleInput{
		ClientID: inbound.Optional[int64]{Set: true, Value: ptrInt64(2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Client == nil || updated.Client.ID != 2 {
		t.Fatalf("client = %+v", updated.Client)
	}
	if updated.UpdatedAt == nil {
		t.Fatal("updated_at is nil")
	}
	requireSaleTime(t, *updated.UpdatedAt)

	cleared, err := service.Update(ctx, sale.ID, inbound.UpdateSaleInput{
		ClientID: inbound.Optional[int64]{Set: true, Value: nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ClientID != nil || cleared.Client != nil {
		t.Fatalf("cleared sale = %+v", cleared)
	}
}

func TestSaleEmptyUpdateLeavesTimestamp(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		CreatedByID: 1,
		Items:       []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sale.UpdatedAt != nil {
		t.Fatalf("created updated_at = %v, want nil", sale.UpdatedAt)
	}

	unchanged, err := service.Update(ctx, sale.ID, inbound.UpdateSaleInput{})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.UpdatedAt != nil {
		t.Fatalf("empty update set updated_at = %v", unchanged.UpdatedAt)
	}
	if unchanged.PaymentStatus != sale.PaymentStatus || !unchanged.TotalAmount.Equal(sale.TotalAmount) {
		t.Fatalf("empty update changed sale: %+v", unchanged)
	}
}

func TestSaleItemAddUpdateDeleteStock(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 10)
	env.addProduct(2, "SKU-2", "5", 10)
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		CreatedByID: 1,
		Items:       []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.stock[1] != 9 {
		t.Fatalf("stock after create = %d, want 9", env.stock[1])
	}

	added, err := service.AddItem(ctx, sale.ID, inbound.AddSaleItemInput{ProductID: 2, Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if env.stock[2] != 8 {
		t.Fatalf("stock after add = %d, want 8", env.stock[2])
	}
	if !added.TotalAmount.Equal(decimal.RequireFromString("20")) {
		t.Fatalf("total after add = %s, want 20", added.TotalAmount)
	}

	if _, err := service.AddItem(ctx, sale.ID, inbound.AddSaleItemInput{ProductID: 2, Quantity: 1}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate item error = %v, want ErrConflict", err)
	}
	if env.stock[2] != 8 {
		t.Fatalf("stock after duplicate = %d, want 8", env.stock[2])
	}

	items, err := service.ListItems(ctx, sale.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var productTwo *domain.SaleItem
	for _, item := range items.Items {
		if item.ProductID == 2 {
			productTwo = item
		}
	}
	if productTwo == nil {
		t.Fatal("added item not found")
	}

	updated, err := service.UpdateItem(ctx, sale.ID, productTwo.ID, inbound.UpdateSaleItemInput{
		Quantity:          inbound.Optional[int]{Set: true, Value: func() *int { v := 1; return &v }()},
		DeliveredQuantity: inbound.Optional[int]{Set: true, Value: func() *int { v := 1; return &v }()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.stock[2] != 9 {
		t.Fatalf("stock after quantity decrease = %d, want 9", env.stock[2])
	}
	if updated.DeliveryStatus != domain.DeliveryStatusPartial {
		t.Fatalf("delivery status = %q, want partial", updated.DeliveryStatus)
	}
	if !updated.TotalAmount.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("total after update = %s, want 15", updated.TotalAmount)
	}

	afterDelete, err := service.DeleteItem(ctx, sale.ID, productTwo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if env.stock[2] != 10 {
		t.Fatalf("stock after delete item = %d, want 10", env.stock[2])
	}
	if !afterDelete.TotalAmount.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("total after item delete = %s, want 10", afterDelete.TotalAmount)
	}
}

func TestSaleDeleteRestoresStock(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	env.addProduct(2, "SKU-2", "5", 5)
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		CreatedByID: 1,
		Items: []inbound.CreateSaleItemInput{
			{ProductID: 1, Quantity: 2},
			{ProductID: 2, Quantity: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := service.Delete(ctx, sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != sale.ID {
		t.Fatalf("deleted id = %d, want %d", deleted.ID, sale.ID)
	}
	if env.stock[1] != 5 || env.stock[2] != 5 {
		t.Fatalf("restored stock = %d/%d, want 5/5", env.stock[1], env.stock[2])
	}
	if _, err := service.Get(ctx, sale.ID); err == nil {
		t.Fatal("deleted sale is still readable")
	}
}

func TestSaleEmptyRecalculationIsPending(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		PaymentMethod: ptrString(domain.PaymentMethodCash),
		AmountPaid:    decimal.RequireFromString("10"),
		CreatedByID:   1,
		Items:         []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1, DeliveredQuantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.ListItems(ctx, sale.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	emptied, err := service.DeleteItem(ctx, sale.ID, items.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !emptied.TotalAmount.IsZero() {
		t.Fatalf("empty total = %s, want 0", emptied.TotalAmount)
	}
	if emptied.PaymentStatus != domain.PaymentStatusPending || emptied.DeliveryStatus != domain.DeliveryStatusPending {
		t.Fatalf("empty statuses = %q/%q, want pending/pending", emptied.PaymentStatus, emptied.DeliveryStatus)
	}
}

func TestSalePaymentsLifecycle(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	env.addUser(7, "seller@example.com")
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		PaymentMethod: ptrString(domain.PaymentMethodCash),
		AmountPaid:    decimal.RequireFromString("5"),
		CreatedByID:   7,
		Items:         []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.CreatePayment(ctx, sale.ID, inbound.CreateSalePaymentInput{
		Amount:        decimal.RequireFromString("10"),
		PaymentMethod: domain.PaymentMethodDebit,
		CreatedByID:   7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.AmountPaid.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("amount paid = %s, want 15", updated.AmountPaid)
	}
	if updated.PaymentStatus != domain.PaymentStatusPartial || !updated.DebtAmount.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("status = %q debt = %s", updated.PaymentStatus, updated.DebtAmount)
	}

	if _, err := service.CreatePayment(ctx, sale.ID, inbound.CreateSalePaymentInput{
		Amount:        decimal.RequireFromString("5.01"),
		PaymentMethod: domain.PaymentMethodCash,
		CreatedByID:   7,
	}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("overpayment error = %v, want ErrValidation", err)
	}

	final, err := service.CreatePayment(ctx, sale.ID, inbound.CreateSalePaymentInput{
		Amount:        decimal.RequireFromString("5"),
		PaymentMethod: domain.PaymentMethodCredit,
		CreatedByID:   7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.PaymentStatus != domain.PaymentStatusPaid || !final.DebtAmount.IsZero() {
		t.Fatalf("paid status = %q debt = %s", final.PaymentStatus, final.DebtAmount)
	}

	if _, err := service.CreatePayment(ctx, sale.ID, inbound.CreateSalePaymentInput{
		Amount:        decimal.RequireFromString("1"),
		PaymentMethod: domain.PaymentMethodCash,
		CreatedByID:   7,
	}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("already paid error = %v, want ErrValidation", err)
	}

	payments, err := service.ListPayments(ctx, sale.ID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(payments.Items) != 2 || !payments.HasNext {
		t.Fatalf("first payment page = %+v", payments)
	}
	if payments.Items[0].ID < payments.Items[1].ID {
		t.Fatalf("payment order = %d/%d, want id DESC", payments.Items[0].ID, payments.Items[1].ID)
	}
	if payments.Items[0].CreatedBy == nil || payments.Items[0].CreatedBy.Email != "seller@example.com" {
		t.Fatalf("payment creator = %+v", payments.Items[0].CreatedBy)
	}

	remaining, err := service.ListPayments(ctx, sale.ID, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Items) != 1 || remaining.HasNext {
		t.Fatalf("second payment page = %+v", remaining)
	}

	deleted, err := service.DeletePayment(ctx, sale.ID, finalPaymentID(env, sale.ID, domain.PaymentMethodCredit))
	if err != nil {
		t.Fatal(err)
	}
	if deleted.PaymentStatus != domain.PaymentStatusPartial || !deleted.AmountPaid.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("after delete = %q / %s", deleted.PaymentStatus, deleted.AmountPaid)
	}
	var notFound *domain.NotFoundError
	if _, err := service.DeletePayment(ctx, sale.ID, 9999); !errors.As(err, &notFound) {
		t.Fatalf("second delete error = %v, want NotFoundError", err)
	}

	for _, tc := range []struct {
		name  string
		input inbound.CreateSalePaymentInput
	}{
		{"zero amount", inbound.CreateSalePaymentInput{Amount: decimal.Zero, PaymentMethod: domain.PaymentMethodCash, CreatedByID: 7}},
		{"negative amount", inbound.CreateSalePaymentInput{Amount: decimal.RequireFromString("-1"), PaymentMethod: domain.PaymentMethodCash, CreatedByID: 7}},
		{"missing method", inbound.CreateSalePaymentInput{Amount: decimal.RequireFromString("1"), CreatedByID: 7}},
		{"invalid method", inbound.CreateSalePaymentInput{Amount: decimal.RequireFromString("1"), PaymentMethod: "cheque", CreatedByID: 7}},
	} {
		if _, err := service.CreatePayment(ctx, sale.ID, tc.input); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("%s error = %v, want ErrValidation", tc.name, err)
		}
	}
	if _, err := service.CreatePayment(ctx, 9999, inbound.CreateSalePaymentInput{
		Amount:        decimal.RequireFromString("1"),
		PaymentMethod: domain.PaymentMethodCash,
		CreatedByID:   7,
	}); !errors.As(err, &notFound) {
		t.Fatalf("missing sale error = %v, want NotFoundError", err)
	}
}

func finalPaymentID(env *fakeSaleEnv, saleID int64, method string) int64 {
	var id int64
	for paymentID, payment := range env.payments {
		if payment.SaleID == saleID && payment.PaymentMethod != nil && *payment.PaymentMethod == method {
			if paymentID > id {
				id = paymentID
			}
		}
	}
	return id
}

func TestSaleDeleteCascadesPayments(t *testing.T) {
	ctx := context.Background()
	env := newFakeSaleEnv()
	env.addProduct(1, "SKU-1", "10", 5)
	service := env.service()

	sale, err := service.Create(ctx, inbound.CreateSaleInput{
		PaymentMethod: ptrString(domain.PaymentMethodCash),
		AmountPaid:    decimal.RequireFromString("5"),
		CreatedByID:   1,
		Items:         []inbound.CreateSaleItemInput{{ProductID: 1, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delete(ctx, sale.ID); err != nil {
		t.Fatal(err)
	}
	for _, payment := range env.payments {
		if payment.SaleID == sale.ID {
			t.Fatalf("payment %d survived sale delete", payment.ID)
		}
	}
}
