package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

const (
	defaultListLimit = 100
	maxListLimit     = 200
	maxListSearch    = 256
)

type UsersHandler struct {
	service inbound.UserUseCase
	logger  *slog.Logger
}

type createUserRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	FullName    string `json:"full_name"`
	IsActive    *bool  `json:"is_active"`
	IsSuperuser *bool  `json:"is_superuser"`
}

type updateUserRequest struct {
	Email       *string `json:"email"`
	Password    *string `json:"password"`
	FullName    *string `json:"full_name"`
	IsActive    *bool   `json:"is_active"`
	IsSuperuser *bool   `json:"is_superuser"`
}

func NewUsersHandler(service inbound.UserUseCase, logger *slog.Logger) *UsersHandler {
	return &UsersHandler{service: service, logger: logger}
}

func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, search, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	result, err := h.service.List(r.Context(), skip, limit, search)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	items := make([]*userResponse, len(result.Items))
	for i, user := range result.Items {
		items[i] = newUserResponse(user)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createUserRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	user, err := h.service.Create(r.Context(), inbound.CreateUserInput{
		Email:       body.Email,
		Password:    body.Password,
		FullName:    body.FullName,
		IsActive:    body.IsActive,
		IsSuperuser: body.IsSuperuser,
	})
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(user))
}

func (h *UsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid user id", "VALIDATION_ERROR")
		return
	}
	user, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(user))
}

func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid user id", "VALIDATION_ERROR")
		return
	}
	var body updateUserRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeRequestError(w, err)
		return
	}
	user, err := h.service.Update(r.Context(), id, inbound.UpdateUserInput{
		Email:       body.Email,
		Password:    body.Password,
		FullName:    body.FullName,
		IsActive:    body.IsActive,
		IsSuperuser: body.IsSuperuser,
	})
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(user))
}

func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid user id", "VALIDATION_ERROR")
		return
	}
	user, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(user))
}

func (h *UsersHandler) writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusBadRequest, "The user with this username already exists in the system.", "")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "User not found", "")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, "The user cannot be deleted because it is referenced by other records.", "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("user request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

func requireSuperuser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || !user.IsSuperuser {
			writeError(w, http.StatusForbidden, "The user doesn't have enough privileges", "")
			return
		}
		next(w, r)
	}
}

func parseListQuery(r *http.Request) (int, int, string, error) {
	query := r.URL.Query()
	skip := 0
	if raw := query.Get("skip"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, 0, "", errors.New("invalid skip")
		}
		skip = parsed
	}
	limit := defaultListLimit
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxListLimit {
			return 0, 0, "", errors.New("invalid limit")
		}
		limit = parsed
	}
	search := strings.TrimSpace(query.Get("search"))
	if utf8.RuneCountInString(search) > maxListSearch {
		return 0, 0, "", errors.New("invalid search")
	}
	return skip, limit, search, nil
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}
