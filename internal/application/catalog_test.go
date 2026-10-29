package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type fakeCatalogStore[T any] struct {
	getResult *T
	getErr    error

	listItems   []*T
	listHasNext bool
	listErr     error

	createErr error
	updateErr error

	deleteResult *T
	deleteErr    error

	getID      int64
	listSkip   int
	listLimit  int
	listSearch string
	created    *T
	updateID   int64
	updated    *T
	deleteID   int64
}

func (f *fakeCatalogStore[T]) Get(_ context.Context, id int64) (*T, error) {
	f.getID = id
	return f.getResult, f.getErr
}

func (f *fakeCatalogStore[T]) List(_ context.Context, skip, limit int, search string) ([]*T, bool, error) {
	f.listSkip, f.listLimit, f.listSearch = skip, limit, search
	return f.listItems, f.listHasNext, f.listErr
}

func (f *fakeCatalogStore[T]) Create(_ context.Context, entity *T) (*T, error) {
	f.created = entity
	if f.createErr != nil {
		return nil, f.createErr
	}
	return entity, nil
}

func (f *fakeCatalogStore[T]) Update(_ context.Context, id int64, entity *T) error {
	f.updateID, f.updated = id, entity
	return f.updateErr
}

func (f *fakeCatalogStore[T]) Delete(_ context.Context, id int64) (*T, error) {
	f.deleteID = id
	return f.deleteResult, f.deleteErr
}

func ptr(value string) *string {
	return &value
}

func repeat(value string, count int) string {
	return strings.Repeat(value, count)
}

func TestCategoryCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   inbound.CreateCategoryInput
		wantErr bool
	}{
		{name: "valid", input: inbound.CreateCategoryInput{Name: "Tools", Code: ptr("TOL"), Description: ptr("Hand tools")}},
		{name: "empty name", input: inbound.CreateCategoryInput{Name: ""}, wantErr: true},
		{name: "name at max", input: inbound.CreateCategoryInput{Name: repeat("a", 150)}},
		{name: "name too long", input: inbound.CreateCategoryInput{Name: repeat("a", 151)}, wantErr: true},
		{name: "code at max", input: inbound.CreateCategoryInput{Name: "Tools", Code: ptr(repeat("c", 10))}},
		{name: "code too long", input: inbound.CreateCategoryInput{Name: "Tools", Code: ptr(repeat("c", 11))}, wantErr: true},
		{name: "description at max", input: inbound.CreateCategoryInput{Name: "Tools", Description: ptr(repeat("d", 500))}},
		{name: "description too long", input: inbound.CreateCategoryInput{Name: "Tools", Description: ptr(repeat("d", 501))}, wantErr: true},
		{name: "empty optional clears", input: inbound.CreateCategoryInput{Name: "Tools", Code: ptr(""), Description: ptr("")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCatalogStore[domain.Category]{}
			service := NewCategoryService(store)
			entity, err := service.Create(context.Background(), tt.input)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				if store.created != nil {
					t.Fatal("repository was called for invalid input")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if entity == nil || store.created == nil {
				t.Fatal("expected the repository to receive the new entity")
			}
			if tt.name == "empty optional clears" && (entity.Code != nil || entity.Description != nil) {
				t.Fatalf("empty optional values = %v/%v, want nil", entity.Code, entity.Description)
			}
		})
	}
}

func TestBrandCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   inbound.CreateBrandInput
		wantErr bool
	}{
		{name: "valid", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("https://example.com/logo.png")}},
		{name: "empty name", input: inbound.CreateBrandInput{Name: ""}, wantErr: true},
		{name: "name at max", input: inbound.CreateBrandInput{Name: repeat("a", 150)}},
		{name: "name too long", input: inbound.CreateBrandInput{Name: repeat("a", 151)}, wantErr: true},
		{name: "logo at max", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("https://example.com/" + repeat("l", 235))}},
		{name: "logo too long", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("https://example.com/" + repeat("l", 236))}, wantErr: true},
		{name: "empty logo clears", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("")}},
		{name: "relative logo", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("/logo.png")}, wantErr: true},
		{name: "javascript logo", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("javascript:alert(1)")}, wantErr: true},
		{name: "data logo", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("data:image/png;base64,AAAA")}, wantErr: true},
		{name: "ftp logo", input: inbound.CreateBrandInput{Name: "Acme", LogoURL: ptr("ftp://example.com/logo.png")}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCatalogStore[domain.Brand]{}
			service := NewBrandService(store)
			_, err := service.Create(context.Background(), tt.input)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaterialCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   inbound.CreateMaterialInput
		wantErr bool
	}{
		{name: "valid", input: inbound.CreateMaterialInput{Name: "Steel"}},
		{name: "empty name", input: inbound.CreateMaterialInput{Name: ""}, wantErr: true},
		{name: "name at max", input: inbound.CreateMaterialInput{Name: repeat("a", 150)}},
		{name: "name too long", input: inbound.CreateMaterialInput{Name: repeat("a", 151)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCatalogStore[domain.Material]{}
			service := NewMaterialService(store)
			_, err := service.Create(context.Background(), tt.input)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMeasurementUnitCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   inbound.CreateMeasurementUnitInput
		wantErr bool
	}{
		{name: "valid", input: inbound.CreateMeasurementUnitInput{Name: "Centimeter", Abbreviation: "cm"}},
		{name: "empty name", input: inbound.CreateMeasurementUnitInput{Name: "", Abbreviation: "cm"}, wantErr: true},
		{name: "empty abbreviation", input: inbound.CreateMeasurementUnitInput{Name: "Centimeter", Abbreviation: ""}, wantErr: true},
		{name: "name at max", input: inbound.CreateMeasurementUnitInput{Name: repeat("a", 150), Abbreviation: "cm"}},
		{name: "name too long", input: inbound.CreateMeasurementUnitInput{Name: repeat("a", 151), Abbreviation: "cm"}, wantErr: true},
		{name: "abbreviation at max", input: inbound.CreateMeasurementUnitInput{Name: "Centimeter", Abbreviation: repeat("x", 5)}},
		{name: "abbreviation too long", input: inbound.CreateMeasurementUnitInput{Name: "Centimeter", Abbreviation: repeat("x", 6)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeCatalogStore[domain.MeasurementUnit]{}
			service := NewMeasurementUnitService(store)
			_, err := service.Create(context.Background(), tt.input)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCategoryPartialUpdate(t *testing.T) {
	store := &fakeCatalogStore[domain.Category]{getResult: &domain.Category{
		ID: 7, Name: "Old", Code: ptr("OLD"), Description: ptr("old description"),
	}}
	service := NewCategoryService(store)

	entity, err := service.Update(context.Background(), 7, inbound.UpdateCategoryInput{Description: ptr("new description")})
	if err != nil {
		t.Fatal(err)
	}
	if store.updateID != 7 || store.updated != entity {
		t.Fatalf("repository update = id %d entity %p, want id 7 entity %p", store.updateID, store.updated, entity)
	}
	if entity.Name != "Old" || entity.Code == nil || *entity.Code != "OLD" {
		t.Fatalf("omitted fields changed: %+v", entity)
	}
	if entity.Description == nil || *entity.Description != "new description" {
		t.Fatalf("description = %v, want new description", entity.Description)
	}

	entity, err = service.Update(context.Background(), 7, inbound.UpdateCategoryInput{Code: ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if entity.Code != nil {
		t.Fatalf("cleared code = %v, want nil", entity.Code)
	}

	if _, err := service.Update(context.Background(), 7, inbound.UpdateCategoryInput{Name: ptr("")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty name error = %v, want ErrValidation", err)
	}
}

func TestBrandMaterialAndUnitPartialUpdate(t *testing.T) {
	t.Run("brand", func(t *testing.T) {
		store := &fakeCatalogStore[domain.Brand]{getResult: &domain.Brand{ID: 1, Name: "Acme", LogoURL: ptr("https://example.com/logo.png")}}
		service := NewBrandService(store)
		entity, err := service.Update(context.Background(), 1, inbound.UpdateBrandInput{LogoURL: ptr("")})
		if err != nil {
			t.Fatal(err)
		}
		if entity.Name != "Acme" || entity.LogoURL != nil {
			t.Fatalf("updated brand = %+v, want name kept and logo cleared", entity)
		}
		if _, err := service.Update(context.Background(), 1, inbound.UpdateBrandInput{LogoURL: ptr("javascript:alert(1)")}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("javascript logo error = %v, want ErrValidation", err)
		}
	})

	t.Run("material", func(t *testing.T) {
		store := &fakeCatalogStore[domain.Material]{getResult: &domain.Material{ID: 2, Name: "Old"}}
		service := NewMaterialService(store)
		entity, err := service.Update(context.Background(), 2, inbound.UpdateMaterialInput{Name: ptr("New")})
		if err != nil {
			t.Fatal(err)
		}
		if entity.Name != "New" {
			t.Fatalf("material name = %q, want New", entity.Name)
		}
	})

	t.Run("unit", func(t *testing.T) {
		store := &fakeCatalogStore[domain.MeasurementUnit]{getResult: &domain.MeasurementUnit{ID: 3, Name: "Centimeter", Abbreviation: "cm"}}
		service := NewMeasurementUnitService(store)
		entity, err := service.Update(context.Background(), 3, inbound.UpdateMeasurementUnitInput{Abbreviation: ptr("mm")})
		if err != nil {
			t.Fatal(err)
		}
		if entity.Name != "Centimeter" || entity.Abbreviation != "mm" {
			t.Fatalf("updated unit = %+v", entity)
		}
		if _, err := service.Update(context.Background(), 3, inbound.UpdateMeasurementUnitInput{Abbreviation: ptr("")}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("empty abbreviation error = %v, want ErrValidation", err)
		}
	})
}

func TestCatalogErrorPropagation(t *testing.T) {
	ctx := context.Background()
	store := &fakeCatalogStore[domain.Category]{createErr: domain.ErrConflict}
	service := NewCategoryService(store)

	if _, err := service.Create(ctx, inbound.CreateCategoryInput{Name: "Tools"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("create conflict error = %v, want ErrConflict", err)
	}

	store.getResult = &domain.Category{ID: 1, Name: "Tools"}
	store.updateErr = domain.ErrConflict
	if _, err := service.Update(ctx, 1, inbound.UpdateCategoryInput{Name: ptr("Other")}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("update conflict error = %v, want ErrConflict", err)
	}

	store.getErr = domain.ErrNotFound
	if _, err := service.Get(ctx, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get missing error = %v, want ErrNotFound", err)
	}
	if _, err := service.Update(ctx, 99, inbound.UpdateCategoryInput{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update missing error = %v, want ErrNotFound", err)
	}

	store.deleteErr = domain.ErrNotFound
	if _, err := service.Delete(ctx, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing error = %v, want ErrNotFound", err)
	}

	store.listErr = errors.New("list failed")
	if _, err := service.List(ctx, 0, 10, ""); err == nil {
		t.Fatal("list error was swallowed")
	}
}

func TestCatalogListPaginationAndSearch(t *testing.T) {
	first := &domain.Category{ID: 2, Name: "Tools"}
	second := &domain.Category{ID: 1, Name: "Paints"}
	store := &fakeCatalogStore[domain.Category]{listItems: []*domain.Category{first, second}, listHasNext: true}
	service := NewCategoryService(store)

	result, err := service.List(context.Background(), 5, 10, "  tools  ")
	if err != nil {
		t.Fatal(err)
	}
	if store.listSkip != 5 || store.listLimit != 10 || store.listSearch != "tools" {
		t.Fatalf("repository list args = skip %d limit %d search %q, want 5/10/tools", store.listSkip, store.listLimit, store.listSearch)
	}
	if result.Skip != 5 || result.Limit != 10 || !result.HasNext {
		t.Fatalf("result = %+v, want skip 5 limit 10 has_next true", result)
	}
	if len(result.Items) != 2 || result.Items[0] != first || result.Items[1] != second {
		t.Fatalf("result items = %+v", result.Items)
	}
}
