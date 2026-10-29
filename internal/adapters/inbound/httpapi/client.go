package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type clientResponse struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	IdentityCard *string `json:"identity_card"`
	Email        *string `json:"email"`
	Phone        *string `json:"phone"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    *string `json:"updated_at"`
}

type createClientRequest struct {
	Name         string  `json:"name"`
	IdentityCard *string `json:"identity_card"`
	Email        *string `json:"email"`
	Phone        *string `json:"phone"`
}

type updateClientRequest struct {
	Name         optional[string] `json:"name"`
	IdentityCard optional[string] `json:"identity_card"`
	Email        optional[string] `json:"email"`
	Phone        optional[string] `json:"phone"`
}

type ClientHandler struct {
	service inbound.ClientUseCase
	logger  *slog.Logger
}

func NewClientHandler(service inbound.ClientUseCase, logger *slog.Logger) *ClientHandler {
	return &ClientHandler{service: service, logger: logger}
}

func (h *ClientHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, search, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	result, err := h.service.List(r.Context(), inbound.ClientFilter{Skip: skip, Limit: limit, Search: search})
	if err != nil {
		h.writeClientError(w, r, err)
		return
	}
	items := make([]clientResponse, len(result.Items))
	for i, client := range result.Items {
		items[i] = newClientResponse(client)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *ClientHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid client id", "VALIDATION_ERROR")
		return
	}
	client, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeClientError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newClientResponse(client))
}

func (h *ClientHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createClientRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	client, err := h.service.Create(r.Context(), inbound.CreateClientInput{
		Name:         body.Name,
		IdentityCard: body.IdentityCard,
		Email:        body.Email,
		Phone:        body.Phone,
	})
	if err != nil {
		h.writeClientError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newClientResponse(client))
}

func (h *ClientHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid client id", "VALIDATION_ERROR")
		return
	}
	var body updateClientRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	client, err := h.service.Update(r.Context(), id, inbound.UpdateClientInput{
		Name:         toOptional(body.Name),
		IdentityCard: toOptional(body.IdentityCard),
		Email:        toOptional(body.Email),
		Phone:        toOptional(body.Phone),
	})
	if err != nil {
		h.writeClientError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newClientResponse(client))
}

func (h *ClientHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid client id", "VALIDATION_ERROR")
		return
	}
	client, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writeClientError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newClientResponse(client))
}

func (h *ClientHandler) writeClientError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusBadRequest, "A client with this identity card already exists.", "UNIQUE_VIOLATION")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Client not found", "")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, "The client cannot be deleted because it is referenced by other records.", "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("client request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func newClientResponse(client *domain.Client) clientResponse {
	response := clientResponse{
		ID:           client.ID,
		Name:         client.Name,
		IdentityCard: client.IdentityCard,
		Email:        client.Email,
		Phone:        client.Phone,
		CreatedAt:    client.CreatedAt.UTC().Format(time.RFC3339),
	}
	if client.UpdatedAt != nil {
		updated := client.UpdatedAt.UTC().Format(time.RFC3339)
		response.UpdatedAt = &updated
	}
	return response
}
