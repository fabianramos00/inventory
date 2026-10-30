package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type optional[T any] struct {
	Present bool
	Value   *T
}

func (o *optional[T]) UnmarshalJSON(data []byte) error {
	o.Present = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

func toOptional[T any](value optional[T]) inbound.Optional[T] {
	return inbound.Optional[T]{Set: value.Present, Value: value.Value}
}

type createProductRequest struct {
	Name              string          `json:"name"`
	SKU               *string         `json:"sku"`
	SizeValue         *string         `json:"size_value"`
	MeasurementUnitID *int64          `json:"measurement_unit_id"`
	Price             decimal.Decimal `json:"price"`
	Cost              decimal.Decimal `json:"cost"`
	StockQuantity     optional[int]   `json:"stock_quantity"`
	LowStockThreshold optional[int]   `json:"low_stock_threshold"`
	BrandID           int64           `json:"brand_id"`
	CategoryID        int64           `json:"category_id"`
	MaterialID        int64           `json:"material_id"`
}

type updateProductRequest struct {
	Name              optional[string]          `json:"name"`
	SKU               optional[string]          `json:"sku"`
	SizeValue         optional[string]          `json:"size_value"`
	MeasurementUnitID optional[int64]           `json:"measurement_unit_id"`
	Price             optional[decimal.Decimal] `json:"price"`
	Cost              optional[decimal.Decimal] `json:"cost"`
	StockQuantity     optional[int]             `json:"stock_quantity"`
	LowStockThreshold optional[int]             `json:"low_stock_threshold"`
	BrandID           optional[int64]           `json:"brand_id"`
	CategoryID        optional[int64]           `json:"category_id"`
	MaterialID        optional[int64]           `json:"material_id"`
}

type moneyJSON decimal.Decimal

func (m moneyJSON) MarshalJSON() ([]byte, error) {
	return []byte(decimal.Decimal(m).String()), nil
}

type productResponse struct {
	ID                int64     `json:"id"`
	SKU               string    `json:"sku"`
	Name              string    `json:"name"`
	SizeValue         *string   `json:"size_value"`
	MeasurementUnitID *int64    `json:"measurement_unit_id"`
	Price             moneyJSON `json:"price"`
	Cost              moneyJSON `json:"cost"`
	StockQuantity     int       `json:"stock_quantity"`
	LowStockThreshold int       `json:"low_stock_threshold"`
	BrandID           int64     `json:"brand_id"`
	CategoryID        int64     `json:"category_id"`
	MaterialID        int64     `json:"material_id"`
}

type productDetailResponse struct {
	productResponse
	Category        *categoryResponse        `json:"category"`
	Material        *materialResponse        `json:"material"`
	Brand           *brandResponse           `json:"brand"`
	MeasurementUnit *measurementUnitResponse `json:"measurement_unit"`
}

type productStatsResponse struct {
	TotalProducts       int64     `json:"total_products"`
	TotalStock          int64     `json:"total_stock"`
	LowStockCount       int64     `json:"low_stock_count"`
	TotalInventoryValue moneyJSON `json:"total_inventory_value"`
}

type ProductHandler struct {
	service inbound.ProductUseCase
	logger  *slog.Logger
}

func NewProductHandler(service inbound.ProductUseCase, logger *slog.Logger) *ProductHandler {
	return &ProductHandler{service: service, logger: logger}
}

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, search, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	query := r.URL.Query()
	categoryID, err := parseFilterID(query.Get("category_id"), "category_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	brandID, err := parseFilterID(query.Get("brand_id"), "brand_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	materialID, err := parseFilterID(query.Get("material_id"), "material_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	stockStatus := query.Get("stock_status")
	if !domain.ValidStockStatus(stockStatus) {
		writeError(w, http.StatusUnprocessableEntity, "Invalid stock_status", "VALIDATION_ERROR")
		return
	}

	result, err := h.service.List(r.Context(), inbound.ProductFilter{
		Skip:        skip,
		Limit:       limit,
		Search:      search,
		CategoryID:  categoryID,
		BrandID:     brandID,
		MaterialID:  materialID,
		StockStatus: stockStatus,
	})
	if err != nil {
		h.writeProductError(w, r, err)
		return
	}
	items := make([]productDetailResponse, len(result.Items))
	for i, item := range result.Items {
		items[i] = newProductDetailResponse(item)
	}
	writePage(w, items, result.HasNext, result.Skip, result.Limit)
}

func (h *ProductHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	detail, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeProductError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProductDetailResponse(detail))
}

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createProductRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	product, err := h.service.Create(r.Context(), inbound.CreateProductInput{
		Name:              body.Name,
		SKU:               body.SKU,
		SizeValue:         body.SizeValue,
		MeasurementUnitID: body.MeasurementUnitID,
		Price:             body.Price,
		Cost:              body.Cost,
		StockQuantity:     toOptional(body.StockQuantity),
		LowStockThreshold: toOptional(body.LowStockThreshold),
		BrandID:           body.BrandID,
		CategoryID:        body.CategoryID,
		MaterialID:        body.MaterialID,
	})
	if err != nil {
		h.writeProductError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProductResponse(product))
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	var body updateProductRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	product, err := h.service.Update(r.Context(), id, inbound.UpdateProductInput{
		Name:              toOptional(body.Name),
		SKU:               toOptional(body.SKU),
		SizeValue:         toOptional(body.SizeValue),
		MeasurementUnitID: toOptional(body.MeasurementUnitID),
		Price:             toOptional(body.Price),
		Cost:              toOptional(body.Cost),
		StockQuantity:     toOptional(body.StockQuantity),
		LowStockThreshold: toOptional(body.LowStockThreshold),
		BrandID:           toOptional(body.BrandID),
		CategoryID:        toOptional(body.CategoryID),
		MaterialID:        toOptional(body.MaterialID),
	})
	if err != nil {
		h.writeProductError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProductResponse(product))
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	product, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writeProductError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProductResponse(product))
}

func (h *ProductHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context())
	if err != nil {
		h.writeProductError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, productStatsResponse{
		TotalProducts:       stats.TotalProducts,
		TotalStock:          stats.TotalStock,
		LowStockCount:       stats.LowStockCount,
		TotalInventoryValue: moneyJSON(stats.TotalInventoryValue),
	})
}

func (h *ProductHandler) writeProductError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusBadRequest, "A product with this SKU already exists.", "UNIQUE_VIOLATION")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Product not found", "")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, "A referenced record does not exist.", "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("product request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func newProductResponse(product *domain.Product) productResponse {
	return productResponse{
		ID:                product.ID,
		SKU:               product.SKU,
		Name:              product.Name,
		SizeValue:         product.SizeValue,
		MeasurementUnitID: product.MeasurementUnitID,
		Price:             moneyJSON(product.Price),
		Cost:              moneyJSON(product.Cost),
		StockQuantity:     product.StockQuantity,
		LowStockThreshold: product.LowStockThreshold,
		BrandID:           product.BrandID,
		CategoryID:        product.CategoryID,
		MaterialID:        product.MaterialID,
	}
}

func newProductDetailResponse(detail *domain.ProductDetail) productDetailResponse {
	response := productDetailResponse{productResponse: newProductResponse(&detail.Product)}
	if detail.Category != nil {
		response.Category = &categoryResponse{ID: detail.Category.ID, Name: detail.Category.Name, Code: detail.Category.Code, Description: detail.Category.Description}
	}
	if detail.Material != nil {
		response.Material = &materialResponse{ID: detail.Material.ID, Name: detail.Material.Name}
	}
	if detail.Brand != nil {
		response.Brand = &brandResponse{ID: detail.Brand.ID, Name: detail.Brand.Name, LogoURL: detail.Brand.LogoURL}
	}
	if detail.MeasurementUnit != nil {
		response.MeasurementUnit = &measurementUnitResponse{
			ID:           detail.MeasurementUnit.ID,
			Name:         detail.MeasurementUnit.Name,
			Abbreviation: detail.MeasurementUnit.Abbreviation,
		}
	}
	return response
}
