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

type providerModel struct {
	ID          int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string  `gorm:"column:name"`
	ContactInfo *string `gorm:"column:contact_info"`
	Email       *string `gorm:"column:email"`
	Phone       *string `gorm:"column:phone"`
	CreatedByID *int64  `gorm:"column:created_by_id"`
}

func (providerModel) TableName() string {
	return "providers"
}

func (m providerModel) toDomain() *domain.Provider {
	return &domain.Provider{
		ID:          m.ID,
		Name:        m.Name,
		ContactInfo: cloneString(m.ContactInfo),
		Email:       cloneString(m.Email),
		Phone:       cloneString(m.Phone),
		CreatedByID: cloneInt64(m.CreatedByID),
	}
}

func providerModelFromDomain(provider *domain.Provider) providerModel {
	return providerModel{
		Name:        provider.Name,
		ContactInfo: cloneString(provider.ContactInfo),
		Email:       cloneString(provider.Email),
		Phone:       cloneString(provider.Phone),
		CreatedByID: cloneInt64(provider.CreatedByID),
	}
}

type providerStore struct {
	db *gorm.DB
}

var _ outbound.ProviderRepository = (*providerStore)(nil)

func NewProviderStore(db *gorm.DB) outbound.ProviderRepository {
	return &providerStore{db: db}
}

func (s *providerStore) Get(ctx context.Context, id int64) (*domain.Provider, error) {
	var record providerModel
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}
	return record.toDomain(), nil
}

func (s *providerStore) List(ctx context.Context, skip, limit int, search string) ([]*domain.Provider, bool, error) {
	search = strings.TrimSpace(search)
	var records []providerModel

	if search != "" && s.db.Dialector.Name() == "sqlite" {
		// ponytail: SQLite search scans the whole table so Go can case-fold Unicode; add FTS5 when the provider table makes it measurable.
		if err := s.db.WithContext(ctx).Order("id DESC").Find(&records).Error; err != nil {
			return nil, false, s.storeError("list providers", err)
		}
		needle := strings.ToLower(search)
		matched := records[:0]
		for _, record := range records {
			if providerMatches(record, needle) {
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
		return providerDomainList(records), hasNext, nil
	}

	query := s.db.WithContext(ctx).Model(new(providerModel))
	if search != "" {
		query = query.Where(
			`strpos(lower(name), lower(?)) > 0
			OR strpos(lower(COALESCE(email, '')), lower(?)) > 0
			OR strpos(lower(COALESCE(phone, '')), lower(?)) > 0`,
			search, search, search,
		)
	}
	if err := query.Order("id DESC").Offset(skip).Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, false, s.storeError("list providers", err)
	}
	hasNext := len(records) > limit
	if hasNext {
		records = records[:limit]
	}
	return providerDomainList(records), hasNext, nil
}

func (s *providerStore) Create(ctx context.Context, provider *domain.Provider) (*domain.Provider, error) {
	record := providerModelFromDomain(provider)
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, s.storeError("create provider", err)
	}
	return record.toDomain(), nil
}

func (s *providerStore) Update(ctx context.Context, id int64, provider *domain.Provider) error {
	result := s.db.WithContext(ctx).Model(new(providerModel)).Where("id = ?", id).Updates(map[string]any{
		"name":         provider.Name,
		"contact_info": provider.ContactInfo,
		"email":        provider.Email,
		"phone":        provider.Phone,
	})
	if result.Error != nil {
		return s.storeError("update provider", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *providerStore) Delete(ctx context.Context, id int64) (*domain.Provider, error) {
	var record providerModel
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load provider for delete: %w", err)
	}
	if err := s.db.WithContext(ctx).Delete(&record).Error; err != nil {
		return nil, s.storeError("delete provider", err)
	}
	return record.toDomain(), nil
}

func (s *providerStore) storeError(op string, err error) error {
	if translator, ok := s.db.Dialector.(gorm.ErrorTranslator); ok {
		err = translator.Translate(err)
	}
	return storeError(op, err)
}

func providerMatches(record providerModel, needle string) bool {
	fields := []string{record.Name}
	for _, value := range []*string{record.Email, record.Phone} {
		if value != nil {
			fields = append(fields, *value)
		}
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

func providerDomainList(records []providerModel) []*domain.Provider {
	providers := make([]*domain.Provider, len(records))
	for i := range records {
		providers[i] = records[i].toDomain()
	}
	return providers
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
