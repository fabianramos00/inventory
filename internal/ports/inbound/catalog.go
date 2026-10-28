package inbound

import "context"

type CatalogList[T any] struct {
	Items   []*T
	HasNext bool
	Skip    int
	Limit   int
}

type CreateCategoryInput struct {
	Name        string
	Code        *string
	Description *string
}

type UpdateCategoryInput struct {
	Name        *string
	Code        *string
	Description *string
}

type CreateBrandInput struct {
	Name    string
	LogoURL *string
}

type UpdateBrandInput struct {
	Name    *string
	LogoURL *string
}

type CreateMaterialInput struct {
	Name string
}

type UpdateMaterialInput struct {
	Name *string
}

type CreateMeasurementUnitInput struct {
	Name         string
	Abbreviation string
}

type UpdateMeasurementUnitInput struct {
	Name         *string
	Abbreviation *string
}

// CatalogUseCase is the inbound port shared by the catalog entities.
type CatalogUseCase[T any, C any, U any] interface {
	List(ctx context.Context, skip, limit int, search string) (*CatalogList[T], error)
	Get(ctx context.Context, id int64) (*T, error)
	Create(ctx context.Context, input C) (*T, error)
	Update(ctx context.Context, id int64, input U) (*T, error)
	Delete(ctx context.Context, id int64) (*T, error)
}
