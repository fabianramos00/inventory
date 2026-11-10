package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

const maxAmountLiteralLength = 32

var (
	errInvalidClientPaymentAmount = errors.New("invalid client payment amount")
	maxClientPaymentAmount        = decimal.RequireFromString(domain.MaxMoneyAmount)
)

// clientPaymentAmount accepts only a plain JSON number: no strings, exponent,
// NaN/Inf, more than two decimals, non-positive values, or amounts above the
// numeric(12,2) ceiling.
type clientPaymentAmount struct {
	decimal.Decimal
}

func (a *clientPaymentAmount) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if len(text) == 0 || len(text) > maxAmountLiteralLength {
		return errInvalidClientPaymentAmount
	}
	for _, r := range text {
		if (r < '0' || r > '9') && r != '-' && r != '.' {
			return errInvalidClientPaymentAmount
		}
	}
	value, err := decimal.NewFromString(text)
	if err != nil {
		return errInvalidClientPaymentAmount
	}
	if value.Exponent() < -2 || !value.IsPositive() || value.GreaterThan(maxClientPaymentAmount) {
		return errInvalidClientPaymentAmount
	}
	a.Decimal = value
	return nil
}

type createClientPaymentRequest struct {
	Amount        clientPaymentAmount `json:"amount"`
	PaymentMethod string              `json:"payment_method"`
}

type clientPaymentAllocationResponse struct {
	SaleID int64     `json:"sale_id"`
	Amount moneyJSON `json:"amount"`
}

type clientPaymentResponse struct {
	ClientID      int64                             `json:"client_id"`
	Amount        moneyJSON                         `json:"amount"`
	RemainingDebt moneyJSON                         `json:"remaining_debt"`
	Allocations   []clientPaymentAllocationResponse `json:"allocations"`
}

type ClientPaymentHandler struct {
	service inbound.ClientPaymentUseCase
	logger  *slog.Logger
}

func NewClientPaymentHandler(service inbound.ClientPaymentUseCase, logger *slog.Logger) *ClientPaymentHandler {
	return &ClientPaymentHandler{service: service, logger: logger}
}

func (h *ClientPaymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid client id", "VALIDATION_ERROR")
		return
	}
	var body createClientPaymentRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	user := UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated", "NOT_AUTHENTICATED")
		return
	}
	if h.service == nil {
		h.logger.Error("client payment service is not configured")
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	result, err := h.service.CreateClientPayment(r.Context(), inbound.CreateClientPaymentInput{
		ClientID:      id,
		Amount:        body.Amount.Decimal,
		PaymentMethod: body.PaymentMethod,
		CreatedByID:   user.ID,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	allocations := make([]clientPaymentAllocationResponse, len(result.Allocations))
	for i, allocation := range result.Allocations {
		allocations[i] = clientPaymentAllocationResponse{
			SaleID: allocation.SaleID,
			Amount: moneyJSON(allocation.Amount),
		}
	}
	writeJSON(w, http.StatusOK, clientPaymentResponse{
		ClientID:      result.ClientID,
		Amount:        moneyJSON(result.Amount),
		RemainingDebt: moneyJSON(result.RemainingDebt),
		Allocations:   allocations,
	})
}

func (h *ClientPaymentHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var notFound *domain.NotFoundError
	switch {
	case errors.As(err, &notFound):
		writeError(w, http.StatusNotFound, notFound.Error(), "")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Client not found", "")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("client payment request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}
