package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type categoryResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
}

type brandResponse struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url"`
}

type materialResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type measurementUnitResponse struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

type createCategoryRequest struct {
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
}

type updateCategoryRequest struct {
	Name        *string `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
}

type createBrandRequest struct {
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url"`
}

type updateBrandRequest struct {
	Name    *string `json:"name"`
	LogoURL *string `json:"logo_url"`
}

type createMaterialRequest struct {
	Name string `json:"name"`
}

type updateMaterialRequest struct {
	Name *string `json:"name"`
}

type createMeasurementUnitRequest struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

type updateMeasurementUnitRequest struct {
	Name         *string `json:"name"`
	Abbreviation *string `json:"abbreviation"`
}

type catalogHandler[T any, C any, U any] struct {
	service        inbound.CatalogUseCase[T, C, U]
	logger         *slog.Logger
	notFoundDetail string
	conflictDetail string
	fkDetail       string
	toResponse     func(*T) any
	decodeCreate   func(http.ResponseWriter, *http.Request) (C, error)
	decodeUpdate   func(http.ResponseWriter, *http.Request) (U, error)
}

func newCategoryHandler(service inbound.CatalogUseCase[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput], logger *slog.Logger) *catalogHandler[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput] {
	return &catalogHandler[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]{
		service:        service,
		logger:         logger,
		notFoundDetail: "Category not found",
		conflictDetail: "A category with this name or code already exists.",
		fkDetail:       "The category cannot be deleted because it is referenced by other records.",
		toResponse: func(entity *domain.Category) any {
			return categoryResponse{ID: entity.ID, Name: entity.Name, Code: entity.Code, Description: entity.Description}
		},
		decodeCreate: func(w http.ResponseWriter, r *http.Request) (inbound.CreateCategoryInput, error) {
			var body createCategoryRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.CreateCategoryInput{}, err
			}
			return inbound.CreateCategoryInput{Name: body.Name, Code: body.Code, Description: body.Description}, nil
		},
		decodeUpdate: func(w http.ResponseWriter, r *http.Request) (inbound.UpdateCategoryInput, error) {
			var body updateCategoryRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.UpdateCategoryInput{}, err
			}
			return inbound.UpdateCategoryInput{Name: body.Name, Code: body.Code, Description: body.Description}, nil
		},
	}
}

func newBrandHandler(service inbound.CatalogUseCase[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput], logger *slog.Logger) *catalogHandler[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput] {
	return &catalogHandler[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]{
		service:        service,
		logger:         logger,
		notFoundDetail: "Brand not found",
		conflictDetail: "A brand with this name already exists.",
		fkDetail:       "The brand cannot be deleted because it is referenced by other records.",
		toResponse: func(entity *domain.Brand) any {
			return brandResponse{ID: entity.ID, Name: entity.Name, LogoURL: entity.LogoURL}
		},
		decodeCreate: func(w http.ResponseWriter, r *http.Request) (inbound.CreateBrandInput, error) {
			var body createBrandRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.CreateBrandInput{}, err
			}
			return inbound.CreateBrandInput{Name: body.Name, LogoURL: body.LogoURL}, nil
		},
		decodeUpdate: func(w http.ResponseWriter, r *http.Request) (inbound.UpdateBrandInput, error) {
			var body updateBrandRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.UpdateBrandInput{}, err
			}
			return inbound.UpdateBrandInput{Name: body.Name, LogoURL: body.LogoURL}, nil
		},
	}
}

func newMaterialHandler(service inbound.CatalogUseCase[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput], logger *slog.Logger) *catalogHandler[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput] {
	return &catalogHandler[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]{
		service:        service,
		logger:         logger,
		notFoundDetail: "Material not found",
		conflictDetail: "A material with this name already exists.",
		fkDetail:       "The material cannot be deleted because it is referenced by other records.",
		toResponse: func(entity *domain.Material) any {
			return materialResponse{ID: entity.ID, Name: entity.Name}
		},
		decodeCreate: func(w http.ResponseWriter, r *http.Request) (inbound.CreateMaterialInput, error) {
			var body createMaterialRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.CreateMaterialInput{}, err
			}
			return inbound.CreateMaterialInput{Name: body.Name}, nil
		},
		decodeUpdate: func(w http.ResponseWriter, r *http.Request) (inbound.UpdateMaterialInput, error) {
			var body updateMaterialRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.UpdateMaterialInput{}, err
			}
			return inbound.UpdateMaterialInput{Name: body.Name}, nil
		},
	}
}

func newMeasurementUnitHandler(service inbound.CatalogUseCase[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput], logger *slog.Logger) *catalogHandler[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput] {
	return &catalogHandler[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]{
		service:        service,
		logger:         logger,
		notFoundDetail: "Unit of Measure not found",
		conflictDetail: "A measurement unit with this name or abbreviation already exists.",
		fkDetail:       "The unit of measure cannot be deleted because it is referenced by other records.",
		toResponse: func(entity *domain.MeasurementUnit) any {
			return measurementUnitResponse{ID: entity.ID, Name: entity.Name, Abbreviation: entity.Abbreviation}
		},
		decodeCreate: func(w http.ResponseWriter, r *http.Request) (inbound.CreateMeasurementUnitInput, error) {
			var body createMeasurementUnitRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.CreateMeasurementUnitInput{}, err
			}
			return inbound.CreateMeasurementUnitInput{Name: body.Name, Abbreviation: body.Abbreviation}, nil
		},
		decodeUpdate: func(w http.ResponseWriter, r *http.Request) (inbound.UpdateMeasurementUnitInput, error) {
			var body updateMeasurementUnitRequest
			if err := decodeJSON(w, r, &body); err != nil {
				return inbound.UpdateMeasurementUnitInput{}, err
			}
			return inbound.UpdateMeasurementUnitInput{Name: body.Name, Abbreviation: body.Abbreviation}, nil
		},
	}
}

func (h *catalogHandler[T, C, U]) List(w http.ResponseWriter, r *http.Request) {
	skip, limit, search, err := parseListQuery(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid query parameters", "VALIDATION_ERROR")
		return
	}
	result, err := h.service.List(r.Context(), skip, limit, search)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	items := make([]any, len(result.Items))
	for i, item := range result.Items {
		items[i] = h.toResponse(item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": result.HasNext,
		"skip":     result.Skip,
		"limit":    result.Limit,
	})
}

func (h *catalogHandler[T, C, U]) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	entity, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(entity))
}

func (h *catalogHandler[T, C, U]) Create(w http.ResponseWriter, r *http.Request) {
	input, err := h.decodeCreate(w, r)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	entity, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(entity))
}

func (h *catalogHandler[T, C, U]) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	input, err := h.decodeUpdate(w, r)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	entity, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(entity))
}

func (h *catalogHandler[T, C, U]) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "Invalid id", "VALIDATION_ERROR")
		return
	}
	entity, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(entity))
}

func (h *catalogHandler[T, C, U]) writeCatalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusBadRequest, h.conflictDetail, "UNIQUE_VIOLATION")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, h.notFoundDetail, "")
	case errors.Is(err, domain.ErrForeignKeyViolation):
		writeError(w, http.StatusBadRequest, h.fkDetail, "FOREIGN_KEY_VIOLATION")
	case errors.Is(err, domain.ErrValidation):
		detail := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusUnprocessableEntity, detail, "VALIDATION_ERROR")
	default:
		h.logger.Error("catalog request failed", "request_id", requestIDFrom(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}
