package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type supplierCatalogResponse struct {
	ID             int64             `json:"id"`
	ProductID      int64             `json:"product_id"`
	ProviderID     int64             `json:"provider_id"`
	SupplierSKU    *string           `json:"supplier_sku"`
	LastQuotedCost moneyJSON         `json:"last_quoted_cost"`
	LeadTime       *int64            `json:"lead_time"`
	Provider       *providerResponse `json:"provider"`
	Products       *productResponse  `json:"products"`
}

type SupplierCatalogHandler struct {
	service inbound.SupplierCatalogUseCase
	logger  *slog.Logger
}

func NewSupplierCatalogHandler(service inbound.SupplierCatalogUseCase, logger *slog.Logger) *SupplierCatalogHandler {
	return &SupplierCatalogHandler{service: service, logger: logger}
}

func (h *SupplierCatalogHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, _, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	query := r.URL.Query()
	providerID, err := parseSupplierCatalogFilterID(query.Get("provider_id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	productID, err := parseSupplierCatalogFilterID(query.Get("product_id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	result, err := h.service.List(r.Context(), inbound.SupplierCatalogFilter{
		Skip:       skip,
		Limit:      limit,
		ProviderID: providerID,
		ProductID:  productID,
	})
	if err != nil {
		h.writeSupplierCatalogError(w, r, err)
		return
	}
	items := make([]supplierCatalogResponse, len(result.Items))
	for i, catalog := range result.Items {
		items[i] = newSupplierCatalogResponse(catalog)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *SupplierCatalogHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid supplier catalog id", "VALIDATION_ERROR")
		return
	}
	catalog, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeSupplierCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSupplierCatalogResponse(catalog))
}

func (h *SupplierCatalogHandler) writeSupplierCatalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Supplier catalog not found", "")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("supplier catalog request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func newSupplierCatalogResponse(catalog *domain.SupplierCatalog) supplierCatalogResponse {
	response := supplierCatalogResponse{
		ID:             catalog.ID,
		ProductID:      catalog.ProductID,
		ProviderID:     catalog.ProviderID,
		SupplierSKU:    catalog.SupplierSKU,
		LastQuotedCost: moneyJSON(catalog.LastQuotedCost),
		LeadTime:       catalog.LeadTime,
	}
	if catalog.Provider != nil {
		provider := newProviderResponse(catalog.Provider)
		response.Provider = &provider
	}
	if catalog.Product != nil {
		product := newProductResponse(catalog.Product)
		response.Products = &product
	}
	return response
}

func parseSupplierCatalogFilterID(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, nil
	}
	return &id, nil
}
