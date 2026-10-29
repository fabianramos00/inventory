package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type createSaleItemRequest struct {
	ProductID         int64                     `json:"product_id"`
	Quantity          int                       `json:"quantity"`
	DeliveredQuantity optional[int]             `json:"delivered_quantity"`
	UnitPrice         optional[decimal.Decimal] `json:"unit_price"`
}

type createSaleRequest struct {
	PaymentMethod *string                   `json:"payment_method"`
	AmountPaid    optional[decimal.Decimal] `json:"amount_paid"`
	ClientID      *int64                    `json:"client_id"`
	CreatedByID   *int64                    `json:"created_by_id"`
	Items         []createSaleItemRequest   `json:"items"`
}

type updateSaleRequest struct {
	PaymentMethod optional[string]          `json:"payment_method"`
	AmountPaid    optional[decimal.Decimal] `json:"amount_paid"`
	ClientID      optional[int64]           `json:"client_id"`
}

type addSaleItemRequest struct {
	ProductID         int64                     `json:"product_id"`
	Quantity          int                       `json:"quantity"`
	DeliveredQuantity optional[int]             `json:"delivered_quantity"`
	UnitPrice         optional[decimal.Decimal] `json:"unit_price"`
}

type updateSaleItemRequest struct {
	Quantity          optional[int]             `json:"quantity"`
	DeliveredQuantity optional[int]             `json:"delivered_quantity"`
	UnitPrice         optional[decimal.Decimal] `json:"unit_price"`
}

type saleResponse struct {
	ID             int64           `json:"id"`
	TotalAmount    moneyJSON       `json:"total_amount"`
	DebtAmount     moneyJSON       `json:"debt_amount"`
	PaymentStatus  string          `json:"payment_status"`
	DeliveryStatus string          `json:"delivery_status"`
	PaymentMethod  *string         `json:"payment_method"`
	AmountPaid     moneyJSON       `json:"amount_paid"`
	ClientID       *int64          `json:"client_id"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      *string         `json:"updated_at"`
	CreatedByID    int64           `json:"created_by_id"`
	CreatedBy      *userResponse   `json:"created_by"`
	Client         *clientResponse `json:"client"`
}

type saleItemResponse struct {
	ID                int64     `json:"id"`
	SaleID            int64     `json:"sale_id"`
	ProductID         int64     `json:"product_id"`
	Quantity          int       `json:"quantity"`
	DeliveredQuantity int       `json:"delivered_quantity"`
	UnitPrice         moneyJSON `json:"unit_price"`
	Subtotal          moneyJSON `json:"subtotal"`
	ProductName       string    `json:"product_name"`
	ProductSKU        string    `json:"product_sku"`
	BrandName         *string   `json:"brand_name"`
	MaterialName      *string   `json:"material_name"`
	CategoryName      *string   `json:"category_name"`
}

type saleStatsResponse struct {
	UnpaidCount      int64     `json:"unpaid_count"`
	UndeliveredCount int64     `json:"undelivered_count"`
	TotalAmountSum   moneyJSON `json:"total_amount_sum"`
	AmountPaidSum    moneyJSON `json:"amount_paid_sum"`
}

type SaleHandler struct {
	service inbound.SaleUseCase
	logger  *slog.Logger
}

func NewSaleHandler(service inbound.SaleUseCase, logger *slog.Logger) *SaleHandler {
	return &SaleHandler{service: service, logger: logger}
}

func (h *SaleHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, _, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	query := r.URL.Query()
	userID, err := parseFilterID(query.Get("user_id"), "user_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid user_id", "VALIDATION_ERROR")
		return
	}
	clientID, err := parseFilterID(query.Get("client_id"), "client_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid client_id", "VALIDATION_ERROR")
		return
	}
	startDate, err := parseFilterDate(query.Get("start_date"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid start_date", "VALIDATION_ERROR")
		return
	}
	endDate, err := parseFilterDate(query.Get("end_date"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid end_date", "VALIDATION_ERROR")
		return
	}
	noClient := false
	if raw := query.Get("no_client"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "Invalid no_client", "VALIDATION_ERROR")
			return
		}
		noClient = parsed
	}
	paymentStatus := query.Get("payment_status")
	if !validPaymentStatus(paymentStatus) {
		writeError(w, http.StatusUnprocessableEntity, "Invalid payment_status", "VALIDATION_ERROR")
		return
	}
	deliveryStatus := query.Get("delivery_status")
	if !validDeliveryStatus(deliveryStatus) {
		writeError(w, http.StatusUnprocessableEntity, "Invalid delivery_status", "VALIDATION_ERROR")
		return
	}
	paymentMethod := query.Get("payment_method")
	if !validPaymentMethod(paymentMethod) {
		writeError(w, http.StatusUnprocessableEntity, "Invalid payment_method", "VALIDATION_ERROR")
		return
	}

	result, err := h.service.List(r.Context(), inbound.SaleFilter{
		Skip:           skip,
		Limit:          limit,
		UserID:         userID,
		ClientID:       clientID,
		NoClient:       noClient,
		StartDate:      startDate,
		EndDate:        endDate,
		PaymentStatus:  paymentStatus,
		DeliveryStatus: deliveryStatus,
		PaymentMethod:  paymentMethod,
	})
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	items := make([]saleResponse, len(result.Items))
	for i, sale := range result.Items {
		items[i] = newSaleResponse(sale)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *SaleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createSaleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	user := UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated", "NOT_AUTHENTICATED")
		return
	}
	amountPaid := decimal.Zero
	if body.AmountPaid.Present {
		if body.AmountPaid.Value == nil {
			writeError(w, http.StatusUnprocessableEntity, "amount_paid is required", "VALIDATION_ERROR")
			return
		}
		amountPaid = *body.AmountPaid.Value
	}
	items := make([]inbound.CreateSaleItemInput, 0, len(body.Items))
	for _, item := range body.Items {
		delivered := 0
		if item.DeliveredQuantity.Present {
			if item.DeliveredQuantity.Value == nil {
				writeError(w, http.StatusUnprocessableEntity, "delivered_quantity is required", "VALIDATION_ERROR")
				return
			}
			delivered = *item.DeliveredQuantity.Value
		}
		items = append(items, inbound.CreateSaleItemInput{
			ProductID:         item.ProductID,
			Quantity:          item.Quantity,
			DeliveredQuantity: delivered,
			UnitPrice:         item.UnitPrice.Value,
		})
	}

	sale, err := h.service.Create(r.Context(), inbound.CreateSaleInput{
		PaymentMethod: body.PaymentMethod,
		AmountPaid:    amountPaid,
		ClientID:      body.ClientID,
		Items:         items,
		CreatedByID:   user.ID,
	})
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context())
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saleStatsResponse{
		UnpaidCount:      stats.UnpaidCount,
		UndeliveredCount: stats.UndeliveredCount,
		TotalAmountSum:   moneyJSON(stats.TotalAmountSum),
		AmountPaidSum:    moneyJSON(stats.AmountPaidSum),
	})
}

func (h *SaleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	sale, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	var body updateSaleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	sale, err := h.service.Update(r.Context(), id, inbound.UpdateSaleInput{
		PaymentMethod: toOptional(body.PaymentMethod),
		AmountPaid:    toOptional(body.AmountPaid),
		ClientID:      toOptional(body.ClientID),
	})
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	sale, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	skip, limit, _, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	result, err := h.service.ListItems(r.Context(), id, skip, limit)
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	items := make([]saleItemResponse, len(result.Items))
	for i, item := range result.Items {
		items[i] = newSaleItemResponse(item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *SaleHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	var body addSaleItemRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	delivered := 0
	if body.DeliveredQuantity.Present {
		if body.DeliveredQuantity.Value == nil {
			writeError(w, http.StatusUnprocessableEntity, "delivered_quantity is required", "VALIDATION_ERROR")
			return
		}
		delivered = *body.DeliveredQuantity.Value
	}
	sale, err := h.service.AddItem(r.Context(), id, inbound.AddSaleItemInput{
		ProductID:         body.ProductID,
		Quantity:          body.Quantity,
		DeliveredQuantity: delivered,
		UnitPrice:         body.UnitPrice.Value,
	})
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	itemID, ok := pathValueID(r, "item_id")
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid item id", "VALIDATION_ERROR")
		return
	}
	var body updateSaleItemRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	sale, err := h.service.UpdateItem(r.Context(), id, itemID, inbound.UpdateSaleItemInput{
		Quantity:          toOptional(body.Quantity),
		DeliveredQuantity: toOptional(body.DeliveredQuantity),
		UnitPrice:         toOptional(body.UnitPrice),
	})
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	itemID, ok := pathValueID(r, "item_id")
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid item id", "VALIDATION_ERROR")
		return
	}
	sale, err := h.service.DeleteItem(r.Context(), id, itemID)
	if err != nil {
		h.writeSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSaleResponse(sale))
}

func (h *SaleHandler) writeSaleError(w http.ResponseWriter, r *http.Request, err error) {
	var notFound *domain.NotFoundError
	var stockErr *domain.InsufficientStockError
	switch {
	case errors.As(err, &notFound):
		writeError(w, http.StatusNotFound, notFound.Error(), "")
	case errors.As(err, &stockErr):
		writeError(w, http.StatusBadRequest, stockErr.Error(), "INSUFFICIENT_STOCK")
	case errors.Is(err, domain.ErrInsufficientStock):
		writeError(w, http.StatusBadRequest, "Insufficient stock", "INSUFFICIENT_STOCK")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Sale not found", "")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusBadRequest, "Product already exists in this sale.", "UNIQUE_VIOLATION")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, "A referenced record does not exist.", "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("sale request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func newSaleResponse(sale *domain.Sale) saleResponse {
	response := saleResponse{
		ID:             sale.ID,
		TotalAmount:    moneyJSON(sale.TotalAmount),
		DebtAmount:     moneyJSON(sale.DebtAmount),
		PaymentStatus:  sale.PaymentStatus,
		DeliveryStatus: sale.DeliveryStatus,
		PaymentMethod:  sale.PaymentMethod,
		AmountPaid:     moneyJSON(sale.AmountPaid),
		ClientID:       sale.ClientID,
		CreatedAt:      sale.CreatedAt.UTC().Format(time.RFC3339),
		CreatedByID:    sale.CreatedByID,
	}
	if sale.UpdatedAt != nil {
		updated := sale.UpdatedAt.UTC().Format(time.RFC3339)
		response.UpdatedAt = &updated
	}
	if sale.CreatedBy != nil {
		response.CreatedBy = newUserResponse(sale.CreatedBy)
	}
	if sale.Client != nil {
		client := newClientResponse(sale.Client)
		response.Client = &client
	}
	return response
}

func newSaleItemResponse(item *domain.SaleItem) saleItemResponse {
	return saleItemResponse{
		ID:                item.ID,
		SaleID:            item.SaleID,
		ProductID:         item.ProductID,
		Quantity:          item.Quantity,
		DeliveredQuantity: item.DeliveredQuantity,
		UnitPrice:         moneyJSON(item.UnitPrice),
		Subtotal:          moneyJSON(item.Subtotal),
		ProductName:       item.ProductName,
		ProductSKU:        item.ProductSKU,
		BrandName:         item.BrandName,
		MaterialName:      item.MaterialName,
		CategoryName:      item.CategoryName,
	}
}

func parseFilterID(raw, field string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New("invalid " + field)
	}
	return &id, nil
}

func parseFilterDate(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func pathValueID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func validPaymentStatus(value string) bool {
	switch value {
	case "", domain.PaymentStatusPending, domain.PaymentStatusPartial, domain.PaymentStatusPaid:
		return true
	default:
		return false
	}
}

func validDeliveryStatus(value string) bool {
	switch value {
	case "", domain.DeliveryStatusPending, domain.DeliveryStatusPartial, domain.DeliveryStatusDelivered:
		return true
	default:
		return false
	}
}

func validPaymentMethod(value string) bool {
	switch value {
	case "", domain.PaymentMethodCash, domain.PaymentMethodCredit, domain.PaymentMethodDebit:
		return true
	default:
		return false
	}
}
