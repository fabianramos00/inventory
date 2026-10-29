package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"inventory-desktop/internal/ports/inbound"
)

type dashboardSummaryResponse struct {
	InventoryValue      moneyJSON `json:"inventory_value"`
	TotalSalesToday     moneyJSON `json:"total_sales_today"`
	PendingOrders       int64     `json:"pending_orders"`
	DeliveredOrders     int64     `json:"delivered_orders"`
	PendingPaymentSales moneyJSON `json:"pending_payment_sales"`
}

type dashboardTrendPointResponse struct {
	Date        string    `json:"date"`
	SalesAmount moneyJSON `json:"sales_amount"`
	OrdersCost  moneyJSON `json:"orders_cost"`
}

type dashboardSalesTrendResponse struct {
	Data []dashboardTrendPointResponse `json:"data"`
}

type dashboardTopProductResponse struct {
	ProductID         int64     `json:"product_id"`
	ProductName       string    `json:"product_name"`
	ProductSKU        string    `json:"product_sku"`
	BrandName         *string   `json:"brand_name"`
	CategoryName      *string   `json:"category_name"`
	MaterialName      *string   `json:"material_name"`
	TotalQuantitySold int64     `json:"total_quantity_sold"`
	TotalSalesAmount  moneyJSON `json:"total_sales_amount"`
}

type dashboardTopProductsResponse struct {
	Products []dashboardTopProductResponse `json:"products"`
}

type dashboardOrderStatusResponse struct {
	Status     string  `json:"status"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

type dashboardOrderStatusDistributionResponse struct {
	Data []dashboardOrderStatusResponse `json:"data"`
}

type DashboardHandler struct {
	service inbound.DashboardUseCase
	logger  *slog.Logger
}

func NewDashboardHandler(service inbound.DashboardUseCase, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{service: service, logger: logger}
}

func (h *DashboardHandler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.Summary(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dashboardSummaryResponse{
		InventoryValue:      moneyJSON(summary.InventoryValue),
		TotalSalesToday:     moneyJSON(summary.TotalSalesToday),
		PendingOrders:       summary.PendingOrders,
		DeliveredOrders:     summary.DeliveredOrders,
		PendingPaymentSales: moneyJSON(summary.PendingPaymentSales),
	})
}

func (h *DashboardHandler) SalesTrend(w http.ResponseWriter, r *http.Request) {
	points, err := h.service.SalesTrend(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	data := make([]dashboardTrendPointResponse, 0, len(points))
	for _, point := range points {
		data = append(data, dashboardTrendPointResponse{
			Date:        point.Date,
			SalesAmount: moneyJSON(point.SalesAmount),
			OrdersCost:  moneyJSON(point.OrdersCost),
		})
	}
	writeJSON(w, http.StatusOK, dashboardSalesTrendResponse{Data: data})
}

func (h *DashboardHandler) TopProducts(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if values, ok := r.URL.Query()["limit"]; ok {
		if len(values) == 0 {
			writeError(w, http.StatusUnprocessableEntity, "Invalid limit", "VALIDATION_ERROR")
			return
		}
		parsed, err := strconv.Atoi(values[0])
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusUnprocessableEntity, "Invalid limit", "VALIDATION_ERROR")
			return
		}
		limit = parsed
	}

	products, err := h.service.TopProducts(r.Context(), limit)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response := make([]dashboardTopProductResponse, 0, len(products))
	for _, product := range products {
		response = append(response, dashboardTopProductResponse{
			ProductID:         product.ProductID,
			ProductName:       product.ProductName,
			ProductSKU:        product.ProductSKU,
			BrandName:         product.BrandName,
			CategoryName:      product.CategoryName,
			MaterialName:      product.MaterialName,
			TotalQuantitySold: product.TotalQuantitySold,
			TotalSalesAmount:  moneyJSON(product.TotalSalesAmount),
		})
	}
	writeJSON(w, http.StatusOK, dashboardTopProductsResponse{Products: response})
}

func (h *DashboardHandler) OrderStatusDistribution(w http.ResponseWriter, r *http.Request) {
	rows, err := h.service.OrderStatusDistribution(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	data := make([]dashboardOrderStatusResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, dashboardOrderStatusResponse{
			Status:     row.Status,
			Count:      row.Count,
			Percentage: row.Percentage,
		})
	}
	writeJSON(w, http.StatusOK, dashboardOrderStatusDistributionResponse{Data: data})
}

func (h *DashboardHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("dashboard request failed", "request_id", requestIDFrom(r.Context()), "error", err)
	writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
}
