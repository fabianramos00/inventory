package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

const (
	maxCatalogName         = 150
	maxCategoryCode        = 10
	maxCategoryDescription = 500
	maxBrandLogoURL        = 255
	maxUnitAbbreviation    = 5
)

type catalogService[T any, C any, U any] struct {
	repository  outbound.CatalogRepository[T]
	fromCreate  func(C) (*T, error)
	applyUpdate func(*T, U) error
}

func newCatalogService[T any, C any, U any](
	repository outbound.CatalogRepository[T],
	fromCreate func(C) (*T, error),
	applyUpdate func(*T, U) error,
) *catalogService[T, C, U] {
	return &catalogService[T, C, U]{
		repository:  repository,
		fromCreate:  fromCreate,
		applyUpdate: applyUpdate,
	}
}

func (s *catalogService[T, C, U]) List(ctx context.Context, skip, limit int, search string) (*inbound.CatalogList[T], error) {
	items, hasNext, err := s.repository.List(ctx, skip, limit, strings.TrimSpace(search))
	if err != nil {
		return nil, err
	}
	return &inbound.CatalogList[T]{Items: items, HasNext: hasNext, Skip: skip, Limit: limit}, nil
}

func (s *catalogService[T, C, U]) Get(ctx context.Context, id int64) (*T, error) {
	return s.repository.Get(ctx, id)
}

func (s *catalogService[T, C, U]) Create(ctx context.Context, input C) (*T, error) {
	entity, err := s.fromCreate(input)
	if err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, entity)
}

func (s *catalogService[T, C, U]) Update(ctx context.Context, id int64, input U) (*T, error) {
	entity, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.applyUpdate(entity, input); err != nil {
		return nil, err
	}
	if err := s.repository.Update(ctx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (s *catalogService[T, C, U]) Delete(ctx context.Context, id int64) (*T, error) {
	return s.repository.Delete(ctx, id)
}

var (
	_ inbound.CatalogUseCase[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]                      = (*catalogService[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput])(nil)
	_ inbound.CatalogUseCase[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]                               = (*catalogService[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput])(nil)
	_ inbound.CatalogUseCase[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]                      = (*catalogService[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput])(nil)
	_ inbound.CatalogUseCase[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput] = (*catalogService[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput])(nil)
)

func NewCategoryService(repository outbound.CatalogRepository[domain.Category]) inbound.CatalogUseCase[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput] {
	return newCatalogService(repository, newCategory, applyCategoryUpdate)
}

func NewBrandService(repository outbound.CatalogRepository[domain.Brand]) inbound.CatalogUseCase[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput] {
	return newCatalogService(repository, newBrand, applyBrandUpdate)
}

func NewMaterialService(repository outbound.CatalogRepository[domain.Material]) inbound.CatalogUseCase[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput] {
	return newCatalogService(repository, newMaterial, applyMaterialUpdate)
}

func NewMeasurementUnitService(repository outbound.CatalogRepository[domain.MeasurementUnit]) inbound.CatalogUseCase[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput] {
	return newCatalogService(repository, newMeasurementUnit, applyMeasurementUnitUpdate)
}

func newCategory(input inbound.CreateCategoryInput) (*domain.Category, error) {
	name, err := requiredName(input.Name)
	if err != nil {
		return nil, err
	}
	code, err := cleanOptional(input.Code, "code", maxCategoryCode)
	if err != nil {
		return nil, err
	}
	description, err := cleanOptional(input.Description, "description", maxCategoryDescription)
	if err != nil {
		return nil, err
	}
	return &domain.Category{Name: name, Code: code, Description: description}, nil
}

func applyCategoryUpdate(entity *domain.Category, input inbound.UpdateCategoryInput) error {
	if input.Name != nil {
		name, err := requiredName(*input.Name)
		if err != nil {
			return err
		}
		entity.Name = name
	}
	if input.Code != nil {
		code, err := cleanOptional(input.Code, "code", maxCategoryCode)
		if err != nil {
			return err
		}
		entity.Code = code
	}
	if input.Description != nil {
		description, err := cleanOptional(input.Description, "description", maxCategoryDescription)
		if err != nil {
			return err
		}
		entity.Description = description
	}
	return nil
}

func newBrand(input inbound.CreateBrandInput) (*domain.Brand, error) {
	name, err := requiredName(input.Name)
	if err != nil {
		return nil, err
	}
	logoURL, err := cleanLogoURL(input.LogoURL)
	if err != nil {
		return nil, err
	}
	return &domain.Brand{Name: name, LogoURL: logoURL}, nil
}

func applyBrandUpdate(entity *domain.Brand, input inbound.UpdateBrandInput) error {
	if input.Name != nil {
		name, err := requiredName(*input.Name)
		if err != nil {
			return err
		}
		entity.Name = name
	}
	if input.LogoURL != nil {
		logoURL, err := cleanLogoURL(input.LogoURL)
		if err != nil {
			return err
		}
		entity.LogoURL = logoURL
	}
	return nil
}

func newMaterial(input inbound.CreateMaterialInput) (*domain.Material, error) {
	name, err := requiredName(input.Name)
	if err != nil {
		return nil, err
	}
	return &domain.Material{Name: name}, nil
}

func applyMaterialUpdate(entity *domain.Material, input inbound.UpdateMaterialInput) error {
	if input.Name != nil {
		name, err := requiredName(*input.Name)
		if err != nil {
			return err
		}
		entity.Name = name
	}
	return nil
}

func newMeasurementUnit(input inbound.CreateMeasurementUnitInput) (*domain.MeasurementUnit, error) {
	name, err := requiredName(input.Name)
	if err != nil {
		return nil, err
	}
	abbreviation, err := requiredAbbreviation(input.Abbreviation)
	if err != nil {
		return nil, err
	}
	return &domain.MeasurementUnit{Name: name, Abbreviation: abbreviation}, nil
}

func applyMeasurementUnitUpdate(entity *domain.MeasurementUnit, input inbound.UpdateMeasurementUnitInput) error {
	if input.Name != nil {
		name, err := requiredName(*input.Name)
		if err != nil {
			return err
		}
		entity.Name = name
	}
	if input.Abbreviation != nil {
		abbreviation, err := requiredAbbreviation(*input.Abbreviation)
		if err != nil {
			return err
		}
		entity.Abbreviation = abbreviation
	}
	return nil
}

func requiredName(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%w: name is required", domain.ErrValidation)
	}
	if utf8.RuneCountInString(value) > maxCatalogName {
		return "", fmt.Errorf("%w: name must be at most %d characters", domain.ErrValidation, maxCatalogName)
	}
	return value, nil
}

func requiredAbbreviation(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%w: abbreviation is required", domain.ErrValidation)
	}
	if utf8.RuneCountInString(value) > maxUnitAbbreviation {
		return "", fmt.Errorf("%w: abbreviation must be at most %d characters", domain.ErrValidation, maxUnitAbbreviation)
	}
	return value, nil
}

func cleanLogoURL(value *string) (*string, error) {
	cleaned, err := cleanOptional(value, "logo_url", maxBrandLogoURL)
	if err != nil || cleaned == nil {
		return cleaned, err
	}
	parsed, err := url.Parse(*cleaned)
	if err != nil || parsed.Host == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return nil, fmt.Errorf("%w: logo_url must be an http or https URL", domain.ErrValidation)
	}
	return cleaned, nil
}

func cleanOptional(value *string, field string, max int) (*string, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(*value) > max {
		return nil, fmt.Errorf("%w: %s must be at most %d characters", domain.ErrValidation, field, max)
	}
	cleaned := *value
	return &cleaned, nil
}
