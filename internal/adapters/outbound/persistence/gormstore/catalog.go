package gormstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type categoryModel struct {
	ID          int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string  `gorm:"column:name"`
	Description *string `gorm:"column:description"`
	Code        *string `gorm:"column:code"`
}

func (categoryModel) TableName() string {
	return "categories"
}

func (m categoryModel) toDomain() *domain.Category {
	return &domain.Category{ID: m.ID, Name: m.Name, Code: cloneString(m.Code), Description: cloneString(m.Description)}
}

func categoryModelFromDomain(entity *domain.Category) categoryModel {
	return categoryModel{Name: entity.Name, Code: cloneString(entity.Code), Description: cloneString(entity.Description)}
}

func categoryUpdates(entity *domain.Category) map[string]any {
	return map[string]any{"name": entity.Name, "code": entity.Code, "description": entity.Description}
}

type brandModel struct {
	ID      int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Name    string  `gorm:"column:name"`
	LogoURL *string `gorm:"column:logo_url"`
}

func (brandModel) TableName() string {
	return "brands"
}

func (m brandModel) toDomain() *domain.Brand {
	return &domain.Brand{ID: m.ID, Name: m.Name, LogoURL: cloneString(m.LogoURL)}
}

func brandModelFromDomain(entity *domain.Brand) brandModel {
	return brandModel{Name: entity.Name, LogoURL: cloneString(entity.LogoURL)}
}

func brandUpdates(entity *domain.Brand) map[string]any {
	return map[string]any{"name": entity.Name, "logo_url": entity.LogoURL}
}

type materialModel struct {
	ID   int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name string `gorm:"column:name"`
}

func (materialModel) TableName() string {
	return "materials"
}

func (m materialModel) toDomain() *domain.Material {
	return &domain.Material{ID: m.ID, Name: m.Name}
}

func materialModelFromDomain(entity *domain.Material) materialModel {
	return materialModel{Name: entity.Name}
}

func materialUpdates(entity *domain.Material) map[string]any {
	return map[string]any{"name": entity.Name}
}

type measurementUnitModel struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name         string `gorm:"column:name"`
	Abbreviation string `gorm:"column:abbreviation"`
}

func (measurementUnitModel) TableName() string {
	return "measurement_units"
}

func (m measurementUnitModel) toDomain() *domain.MeasurementUnit {
	return &domain.MeasurementUnit{ID: m.ID, Name: m.Name, Abbreviation: m.Abbreviation}
}

func measurementUnitModelFromDomain(entity *domain.MeasurementUnit) measurementUnitModel {
	return measurementUnitModel{Name: entity.Name, Abbreviation: entity.Abbreviation}
}

func measurementUnitUpdates(entity *domain.MeasurementUnit) map[string]any {
	return map[string]any{"name": entity.Name, "abbreviation": entity.Abbreviation}
}

var (
	_ outbound.CatalogRepository[domain.Category]        = (*catalogStore[domain.Category, categoryModel])(nil)
	_ outbound.CatalogRepository[domain.Brand]           = (*catalogStore[domain.Brand, brandModel])(nil)
	_ outbound.CatalogRepository[domain.Material]        = (*catalogStore[domain.Material, materialModel])(nil)
	_ outbound.CatalogRepository[domain.MeasurementUnit] = (*catalogStore[domain.MeasurementUnit, measurementUnitModel])(nil)
)

type catalogStore[T any, M any] struct {
	db       *gorm.DB
	toDomain func(M) *T
	toModel  func(*T) M
	nameOf   func(M) string
	updates  func(*T) map[string]any
}

func newCatalogStore[T any, M any](
	db *gorm.DB,
	toDomain func(M) *T,
	toModel func(*T) M,
	nameOf func(M) string,
	updates func(*T) map[string]any,
) *catalogStore[T, M] {
	return &catalogStore[T, M]{db: db, toDomain: toDomain, toModel: toModel, nameOf: nameOf, updates: updates}
}

func NewCategoryStore(db *gorm.DB) outbound.CatalogRepository[domain.Category] {
	return newCatalogStore(db, categoryModel.toDomain, categoryModelFromDomain, func(m categoryModel) string { return m.Name }, categoryUpdates)
}

func NewBrandStore(db *gorm.DB) outbound.CatalogRepository[domain.Brand] {
	return newCatalogStore(db, brandModel.toDomain, brandModelFromDomain, func(m brandModel) string { return m.Name }, brandUpdates)
}

func NewMaterialStore(db *gorm.DB) outbound.CatalogRepository[domain.Material] {
	return newCatalogStore(db, materialModel.toDomain, materialModelFromDomain, func(m materialModel) string { return m.Name }, materialUpdates)
}

func NewMeasurementUnitStore(db *gorm.DB) outbound.CatalogRepository[domain.MeasurementUnit] {
	return newCatalogStore(db, measurementUnitModel.toDomain, measurementUnitModelFromDomain, func(m measurementUnitModel) string { return m.Name }, measurementUnitUpdates)
}

func (s *catalogStore[T, M]) Get(ctx context.Context, id int64) (*T, error) {
	var record M
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get catalog record: %w", err)
	}
	return s.toDomain(record), nil
}

func (s *catalogStore[T, M]) List(ctx context.Context, skip, limit int, search string) ([]*T, bool, error) {
	search = strings.TrimSpace(search)
	var records []M

	if search != "" && s.db.Dialector.Name() == "sqlite" {
		// ponytail: SQLite search scans the whole table so Go can case-fold Unicode; add FTS5 when catalog size makes it measurable.
		if err := s.db.WithContext(ctx).Order("id DESC").Find(&records).Error; err != nil {
			return nil, false, storeError("list catalog", err)
		}
		needle := strings.ToLower(search)
		matched := records[:0]
		for _, record := range records {
			if strings.Contains(strings.ToLower(s.nameOf(record)), needle) {
				matched = append(matched, record)
			}
		}
		records = matched
		if skip >= len(records) {
			records = nil
		} else {
			records = records[skip:]
		}
		hasNext := len(records) > limit
		if hasNext {
			records = records[:limit]
		}
		return s.toDomainList(records), hasNext, nil
	}

	query := s.db.WithContext(ctx).Model(new(M))
	if search != "" {
		query = query.Where("strpos(lower(name), lower(?)) > 0", search)
	}
	if err := query.Order("id DESC").Offset(skip).Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, false, storeError("list catalog", err)
	}
	hasNext := len(records) > limit
	if hasNext {
		records = records[:limit]
	}
	return s.toDomainList(records), hasNext, nil
}

func (s *catalogStore[T, M]) Create(ctx context.Context, entity *T) (*T, error) {
	record := s.toModel(entity)
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, storeError("create catalog record", err)
	}
	return s.toDomain(record), nil
}

func (s *catalogStore[T, M]) Update(ctx context.Context, id int64, entity *T) error {
	result := s.db.WithContext(ctx).Model(new(M)).Where("id = ?", id).Updates(s.updates(entity))
	if result.Error != nil {
		return storeError("update catalog record", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *catalogStore[T, M]) Delete(ctx context.Context, id int64) (*T, error) {
	var record M
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load catalog record for delete: %w", err)
	}
	result := s.db.WithContext(ctx).Delete(&record)
	if result.Error != nil {
		return nil, storeError("delete catalog record", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return s.toDomain(record), nil
}

func (s *catalogStore[T, M]) toDomainList(records []M) []*T {
	items := make([]*T, len(records))
	for i := range records {
		items[i] = s.toDomain(records[i])
	}
	return items
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
