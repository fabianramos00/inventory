package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type providerResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	ContactInfo *string `json:"contact_info"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
}

type createProviderRequest struct {
	Name        string  `json:"name"`
	ContactInfo *string `json:"contact_info"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
}

type updateProviderRequest struct {
	Name        optional[string] `json:"name"`
	ContactInfo optional[string] `json:"contact_info"`
	Email       optional[string] `json:"email"`
	Phone       optional[string] `json:"phone"`
}

type ProviderHandler struct {
	service inbound.ProviderUseCase
	logger  *slog.Logger
}

func NewProviderHandler(service inbound.ProviderUseCase, logger *slog.Logger) *ProviderHandler {
	return &ProviderHandler{service: service, logger: logger}
}

func (h *ProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, search, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	result, err := h.service.List(r.Context(), inbound.ProviderFilter{Skip: skip, Limit: limit, Search: search})
	if err != nil {
		h.writeProviderError(w, r, err)
		return
	}
	items := make([]providerResponse, len(result.Items))
	for i, provider := range result.Items {
		items[i] = newProviderResponse(provider)
	}
	writePage(w, items, result.HasNext, result.Skip, result.Limit)
}

func (h *ProviderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid provider id", "VALIDATION_ERROR")
		return
	}
	provider, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeProviderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProviderResponse(provider))
}

func (h *ProviderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createProviderRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	input := inbound.CreateProviderInput{
		Name:        body.Name,
		ContactInfo: body.ContactInfo,
		Email:       body.Email,
		Phone:       body.Phone,
	}
	if user := UserFromContext(r.Context()); user != nil {
		id := user.ID
		input.CreatedByID = &id
	}
	provider, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.writeProviderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProviderResponse(provider))
}

func (h *ProviderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid provider id", "VALIDATION_ERROR")
		return
	}
	var body updateProviderRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	provider, err := h.service.Update(r.Context(), id, inbound.UpdateProviderInput{
		Name:        toOptional(body.Name),
		ContactInfo: toOptional(body.ContactInfo),
		Email:       toOptional(body.Email),
		Phone:       toOptional(body.Phone),
	})
	if err != nil {
		h.writeProviderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProviderResponse(provider))
}

func (h *ProviderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid provider id", "VALIDATION_ERROR")
		return
	}
	provider, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writeProviderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newProviderResponse(provider))
}

func (h *ProviderHandler) writeProviderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Provider not found", "")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, "The provider cannot be deleted because it is referenced by other records.", "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("provider request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func newProviderResponse(provider *domain.Provider) providerResponse {
	return providerResponse{
		ID:          provider.ID,
		Name:        provider.Name,
		ContactInfo: provider.ContactInfo,
		Email:       provider.Email,
		Phone:       provider.Phone,
	}
}
