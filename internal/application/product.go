package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

const (
	maxProductName      = 150
	maxProductSKU       = 100
	maxProductSizeValue = 20
	productSKUAttempts  = 5
	defaultLowStock     = 10
)

var maxProductMoney = decimal.RequireFromString("9999999999.99")

type ProductService struct {
	repository outbound.ProductRepository
}

func NewProductService(repository outbound.ProductRepository) *ProductService {
	return &ProductService{repository: repository}
}

var _ inbound.ProductUseCase = (*ProductService)(nil)

func (s *ProductService) List(ctx context.Context, filter inbound.ProductFilter) (*inbound.ProductList, error) {
	if !domain.ValidStockStatus(filter.StockStatus) {
		return nil, fmt.Errorf("%w: invalid stock_status", domain.ErrValidation)
	}
	filter.Search = strings.TrimSpace(filter.Search)
	items, hasNext, err := s.repository.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &inbound.ProductList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *ProductService) Get(ctx context.Context, id int64) (*domain.ProductDetail, error) {
	return s.repository.Get(ctx, id)
}

func (s *ProductService) Create(ctx context.Context, input inbound.CreateProductInput) (*domain.Product, error) {
	product, generated, err := newProduct(input)
	if err != nil {
		return nil, err
	}

	attempts := 1
	if generated {
		attempts = productSKUAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			product.SKU, err = generateProductSKU()
			if err != nil {
				return nil, err
			}
		}
		created, err := s.repository.Create(ctx, product)
		if err == nil {
			return created, nil
		}
		if !generated || !errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
	}
	return nil, domain.ErrConflict
}

func (s *ProductService) Update(ctx context.Context, id int64, input inbound.UpdateProductInput) (*domain.Product, error) {
	detail, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	product := detail.Product
	if err := applyProductUpdate(&product, input); err != nil {
		return nil, err
	}
	if err := s.repository.Update(ctx, id, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

func (s *ProductService) Delete(ctx context.Context, id int64) (*domain.Product, error) {
	return s.repository.Delete(ctx, id)
}

func (s *ProductService) Stats(ctx context.Context) (*domain.ProductStats, error) {
	return s.repository.Stats(ctx)
}

func newProduct(input inbound.CreateProductInput) (*domain.Product, bool, error) {
	name, err := productName(input.Name)
	if err != nil {
		return nil, false, err
	}
	sku, generated, err := createProductSKU(input.SKU)
	if err != nil {
		return nil, false, err
	}
	sizeValue, err := cleanProductSize(input.SizeValue)
	if err != nil {
		return nil, false, err
	}
	measurementUnitID, err := optionalID(input.MeasurementUnitID, "measurement_unit_id")
	if err != nil {
		return nil, false, err
	}
	price, err := productMoney(input.Price, "price")
	if err != nil {
		return nil, false, err
	}
	cost, err := productMoney(input.Cost, "cost")
	if err != nil {
		return nil, false, err
	}
	stock, err := createNonNegative(input.StockQuantity, "stock_quantity", 0)
	if err != nil {
		return nil, false, err
	}
	threshold, err := createNonNegative(input.LowStockThreshold, "low_stock_threshold", defaultLowStock)
	if err != nil {
		return nil, false, err
	}
	for field, id := range map[string]int64{"brand_id": input.BrandID, "category_id": input.CategoryID, "material_id": input.MaterialID} {
		if id <= 0 {
			return nil, false, fmt.Errorf("%w: %s must be greater than zero", domain.ErrValidation, field)
		}
	}
	return &domain.Product{
		SKU:               sku,
		Name:              name,
		SizeValue:         sizeValue,
		MeasurementUnitID: measurementUnitID,
		Price:             price,
		Cost:              cost,
		StockQuantity:     stock,
		LowStockThreshold: threshold,
		BrandID:           input.BrandID,
		CategoryID:        input.CategoryID,
		MaterialID:        input.MaterialID,
	}, generated, nil
}

func applyProductUpdate(product *domain.Product, input inbound.UpdateProductInput) error {
	if input.Name.Set {
		if input.Name.Value == nil {
			return requiredField("name")
		}
		name, err := productName(*input.Name.Value)
		if err != nil {
			return err
		}
		product.Name = name
	}
	if input.SKU.Set {
		if input.SKU.Value == nil {
			return requiredField("sku")
		}
		sku := strings.TrimSpace(*input.SKU.Value)
		if sku == "" {
			return requiredField("sku")
		}
		if utf8.RuneCountInString(sku) > maxProductSKU {
			return tooLong("sku", maxProductSKU)
		}
		product.SKU = sku
	}
	if input.SizeValue.Set {
		if input.SizeValue.Value == nil {
			product.SizeValue = nil
		} else {
			sizeValue, err := cleanProductSize(input.SizeValue.Value)
			if err != nil {
				return err
			}
			product.SizeValue = sizeValue
		}
	}
	if input.MeasurementUnitID.Set {
		if input.MeasurementUnitID.Value == nil {
			product.MeasurementUnitID = nil
		} else {
			id, err := optionalID(input.MeasurementUnitID.Value, "measurement_unit_id")
			if err != nil {
				return err
			}
			product.MeasurementUnitID = id
		}
	}
	if input.Price.Set {
		if input.Price.Value == nil {
			return requiredField("price")
		}
		price, err := productMoney(*input.Price.Value, "price")
		if err != nil {
			return err
		}
		product.Price = price
	}
	if input.Cost.Set {
		if input.Cost.Value == nil {
			return requiredField("cost")
		}
		cost, err := productMoney(*input.Cost.Value, "cost")
		if err != nil {
			return err
		}
		product.Cost = cost
	}
	if input.StockQuantity.Set {
		if input.StockQuantity.Value == nil {
			return requiredField("stock_quantity")
		}
		if *input.StockQuantity.Value < 0 {
			return nonNegative("stock_quantity")
		}
		product.StockQuantity = *input.StockQuantity.Value
	}
	if input.LowStockThreshold.Set {
		if input.LowStockThreshold.Value == nil {
			return requiredField("low_stock_threshold")
		}
		if *input.LowStockThreshold.Value < 0 {
			return nonNegative("low_stock_threshold")
		}
		product.LowStockThreshold = *input.LowStockThreshold.Value
	}
	for field, optional := range map[string]inbound.Optional[int64]{
		"brand_id":    input.BrandID,
		"category_id": input.CategoryID,
		"material_id": input.MaterialID,
	} {
		if !optional.Set {
			continue
		}
		if optional.Value == nil {
			return requiredField(field)
		}
		if *optional.Value <= 0 {
			return fmt.Errorf("%w: %s must be greater than zero", domain.ErrValidation, field)
		}
		switch field {
		case "brand_id":
			product.BrandID = *optional.Value
		case "category_id":
			product.CategoryID = *optional.Value
		case "material_id":
			product.MaterialID = *optional.Value
		}
	}
	return nil
}

func productName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", requiredField("name")
	}
	if utf8.RuneCountInString(name) > maxProductName {
		return "", tooLong("name", maxProductName)
	}
	return name, nil
}

func createProductSKU(value *string) (string, bool, error) {
	if value != nil {
		sku := strings.TrimSpace(*value)
		if sku != "" {
			if utf8.RuneCountInString(sku) > maxProductSKU {
				return "", false, tooLong("sku", maxProductSKU)
			}
			return sku, false, nil
		}
	}
	sku, err := generateProductSKU()
	return sku, true, err
}

func generateProductSKU() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate product sku: %w", err)
	}
	return "SKU-" + strings.ToUpper(hex.EncodeToString(buf[:])), nil
}

func cleanProductSize(value *string) (*string, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(*value) > maxProductSizeValue {
		return nil, tooLong("size_value", maxProductSizeValue)
	}
	size := *value
	return &size, nil
}

func productMoney(value decimal.Decimal, field string) (decimal.Decimal, error) {
	rounded := value.Round(2)
	if rounded.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("%w: %s must be greater than zero", domain.ErrValidation, field)
	}
	if rounded.GreaterThan(maxProductMoney) {
		return decimal.Zero, fmt.Errorf("%w: %s must be at most %s", domain.ErrValidation, field, maxProductMoney)
	}
	return rounded, nil
}

func createNonNegative(value inbound.Optional[int], field string, fallback int) (int, error) {
	if !value.Set {
		return fallback, nil
	}
	if value.Value == nil {
		return 0, requiredField(field)
	}
	if *value.Value < 0 {
		return 0, nonNegative(field)
	}
	return *value.Value, nil
}

func optionalID(value *int64, field string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if *value <= 0 {
		return nil, fmt.Errorf("%w: %s must be greater than zero", domain.ErrValidation, field)
	}
	id := *value
	return &id, nil
}

func requiredField(field string) error {
	return fmt.Errorf("%w: %s is required", domain.ErrValidation, field)
}

func nonNegative(field string) error {
	return fmt.Errorf("%w: %s must be zero or greater", domain.ErrValidation, field)
}

func tooLong(field string, max int) error {
	return fmt.Errorf("%w: %s must be at most %d characters", domain.ErrValidation, field, max)
}
