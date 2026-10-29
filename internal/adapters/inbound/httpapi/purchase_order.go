package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type createOrderItemRequest struct {
	ProductID   int64                     `json:"product_id"`
	Quantity    int                       `json:"quantity"`
	UnitCost    optional[decimal.Decimal] `json:"unit_cost"`
	SupplierSKU *string                   `json:"supplier_sku"`
}

type createPurchaseOrderRequest struct {
	ProviderID    optional[int64]          `json:"provider_id"`
	Status        string                   `json:"status"`
	PaymentStatus string                   `json:"payment_status"`
	Items         []createOrderItemRequest `json:"items"`
}

type updatePurchaseOrderRequest struct {
	ProviderID    optional[int64]  `json:"provider_id"`
	Status        optional[string] `json:"status"`
	PaymentStatus optional[string] `json:"payment_status"`
}

type addOrderItemRequest struct {
	ProductID   int64                     `json:"product_id"`
	Quantity    int                       `json:"quantity"`
	UnitCost    optional[decimal.Decimal] `json:"unit_cost"`
	SupplierSKU *string                   `json:"supplier_sku"`
}

type updateOrderItemRequest struct {
	Quantity    optional[int]             `json:"quantity"`
	UnitCost    optional[decimal.Decimal] `json:"unit_cost"`
	SupplierSKU optional[string]          `json:"supplier_sku"`
}

type purchaseOrderResponse struct {
	ID            int64             `json:"id"`
	ProviderID    int64             `json:"provider_id"`
	Status        string            `json:"status"`
	PaymentStatus string            `json:"payment_status"`
	OrderDate     string            `json:"order_date"`
	TotalAmount   moneyJSON         `json:"total_amount"`
	Provider      *providerResponse `json:"provider"`
}

type orderItemResponse struct {
	ID           int64     `json:"id"`
	OrderID      int64     `json:"order_id"`
	ProductID    int64     `json:"product_id"`
	Quantity     int       `json:"quantity"`
	UnitCost     moneyJSON `json:"unit_cost"`
	Subtotal     moneyJSON `json:"subtotal"`
	SupplierSKU  *string   `json:"supplier_sku"`
	ProductName  string    `json:"product_name"`
	ProductSKU   string    `json:"product_sku"`
	BrandName    *string   `json:"brand_name"`
	MaterialName *string   `json:"material_name"`
	CategoryName *string   `json:"category_name"`
}

type PurchaseOrderHandler struct {
	service inbound.PurchaseOrderUseCase
	logger  *slog.Logger
}

func NewPurchaseOrderHandler(service inbound.PurchaseOrderUseCase, logger *slog.Logger) *PurchaseOrderHandler {
	return &PurchaseOrderHandler{service: service, logger: logger}
}

func (h *PurchaseOrderHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, _, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	query := r.URL.Query()
	providerID, err := parseFilterID(query.Get("provider_id"), "provider_id")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid provider_id", "VALIDATION_ERROR")
		return
	}
	status := query.Get("status")
	if !validOrderStatusFilter(status) {
		writeError(w, http.StatusUnprocessableEntity, "Invalid status", "VALIDATION_ERROR")
		return
	}
	paymentStatus := query.Get("payment_status")
	if !validPurchasePaymentStatusFilter(paymentStatus) {
		writeError(w, http.StatusUnprocessableEntity, "Invalid payment_status", "VALIDATION_ERROR")
		return
	}
	minDate, err := parseFilterDate(query.Get("min_date"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid min_date", "VALIDATION_ERROR")
		return
	}
	maxDate, err := parseFilterDate(query.Get("max_date"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid max_date", "VALIDATION_ERROR")
		return
	}

	result, err := h.service.List(r.Context(), inbound.PurchaseOrderFilter{
		Skip:          skip,
		Limit:         limit,
		ProviderID:    providerID,
		Status:        status,
		PaymentStatus: paymentStatus,
		MinDate:       minDate,
		MaxDate:       maxDate,
	})
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	items := make([]purchaseOrderResponse, len(result.Items))
	for i, order := range result.Items {
		items[i] = newPurchaseOrderResponse(order)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *PurchaseOrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createPurchaseOrderRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	if !body.ProviderID.Present || body.ProviderID.Value == nil {
		writeError(w, http.StatusUnprocessableEntity, "provider_id is required", "VALIDATION_ERROR")
		return
	}
	user := UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated", "NOT_AUTHENTICATED")
		return
	}
	items := make([]inbound.CreateOrderItemInput, 0, len(body.Items))
	for _, item := range body.Items {
		if !item.UnitCost.Present || item.UnitCost.Value == nil {
			writeError(w, http.StatusUnprocessableEntity, "unit_cost is required", "VALIDATION_ERROR")
			return
		}
		items = append(items, inbound.CreateOrderItemInput{
			ProductID:   item.ProductID,
			Quantity:    item.Quantity,
			UnitCost:    *item.UnitCost.Value,
			SupplierSKU: item.SupplierSKU,
		})
	}

	order, err := h.service.Create(r.Context(), inbound.CreatePurchaseOrderInput{
		ProviderID:    *body.ProviderID.Value,
		Status:        body.Status,
		PaymentStatus: body.PaymentStatus,
		Items:         items,
		CreatedByID:   user.ID,
	})
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	order, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	var body updatePurchaseOrderRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	order, err := h.service.Update(r.Context(), id, inbound.UpdatePurchaseOrderInput{
		ProviderID:    toOptional(body.ProviderID),
		Status:        toOptional(body.Status),
		PaymentStatus: toOptional(body.PaymentStatus),
	})
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	order, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) ListItems(w http.ResponseWriter, r *http.Request) {
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
		h.writePurchaseOrderError(w, r, err)
		return
	}
	items := make([]orderItemResponse, len(result.Items))
	for i, item := range result.Items {
		items[i] = newOrderItemResponse(item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *PurchaseOrderHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	var body addOrderItemRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	if !body.UnitCost.Present || body.UnitCost.Value == nil {
		writeError(w, http.StatusUnprocessableEntity, "unit_cost is required", "VALIDATION_ERROR")
		return
	}
	order, err := h.service.AddItem(r.Context(), id, inbound.AddOrderItemInput{
		ProductID:   body.ProductID,
		Quantity:    body.Quantity,
		UnitCost:    *body.UnitCost.Value,
		SupplierSKU: body.SupplierSKU,
	})
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
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
	var body updateOrderItemRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	order, err := h.service.UpdateItem(r.Context(), id, itemID, inbound.UpdateOrderItemInput{
		Quantity:    toOptional(body.Quantity),
		UnitCost:    toOptional(body.UnitCost),
		SupplierSKU: toOptional(body.SupplierSKU),
	})
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
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
	order, err := h.service.DeleteItem(r.Context(), id, itemID)
	if err != nil {
		h.writePurchaseOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPurchaseOrderResponse(order))
}

func (h *PurchaseOrderHandler) writePurchaseOrderError(w http.ResponseWriter, r *http.Request, err error) {
	var notFound *domain.NotFoundError
	var missingProvider *domain.ProviderNotFoundError
	switch {
	case errors.As(err, &missingProvider):
		writeError(w, http.StatusBadRequest, missingProvider.Error(), "")
	case errors.As(err, &notFound):
		writeError(w, http.StatusNotFound, notFound.Error(), "")
	case errors.Is(err, domain.ErrConflict):
		detail := strings.TrimPrefix(err.Error(), domain.ErrConflict.Error()+": ")
		writeError(w, http.StatusBadRequest, detail, "UNIQUE_VIOLATION")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, "A referenced record does not exist.", "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("purchase order request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func newPurchaseOrderResponse(order *domain.PurchaseOrder) purchaseOrderResponse {
	response := purchaseOrderResponse{
		ID:            order.ID,
		ProviderID:    order.ProviderID,
		Status:        order.Status,
		PaymentStatus: order.PaymentStatus,
		OrderDate:     order.OrderDate.UTC().Format(time.RFC3339),
		TotalAmount:   moneyJSON(order.TotalAmount),
	}
	if order.Provider != nil {
		provider := newProviderResponse(order.Provider)
		response.Provider = &provider
	}
	return response
}

func newOrderItemResponse(item *domain.OrderItem) orderItemResponse {
	return orderItemResponse{
		ID:           item.ID,
		OrderID:      item.OrderID,
		ProductID:    item.ProductID,
		Quantity:     item.Quantity,
		UnitCost:     moneyJSON(item.UnitCost),
		Subtotal:     moneyJSON(item.Subtotal),
		SupplierSKU:  item.SupplierSKU,
		ProductName:  item.ProductName,
		ProductSKU:   item.ProductSKU,
		BrandName:    item.BrandName,
		MaterialName: item.MaterialName,
		CategoryName: item.CategoryName,
	}
}

func validOrderStatusFilter(value string) bool {
	switch value {
	case "", domain.OrderStatusPending, domain.OrderStatusSent, domain.OrderStatusReceived, domain.OrderStatusCancelled:
		return true
	default:
		return false
	}
}

func validPurchasePaymentStatusFilter(value string) bool {
	switch value {
	case "", domain.PurchasePaymentStatusPending, domain.PurchasePaymentStatusPaid:
		return true
	default:
		return false
	}
}
