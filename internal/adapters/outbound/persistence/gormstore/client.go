package gormstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type clientModel struct {
	ID           int64      `gorm:"column:id;primaryKey;autoIncrement"`
	CreatedAt    Timestamp  `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt    *Timestamp `gorm:"column:updated_at;autoUpdateTime:false"`
	Name         string     `gorm:"column:name"`
	IdentityCard *string    `gorm:"column:identity_card"`
	Email        *string    `gorm:"column:email"`
	Phone        *string    `gorm:"column:phone"`
}

func (clientModel) TableName() string {
	return "clients"
}

func (m clientModel) toDomain() *domain.Client {
	client := &domain.Client{
		ID:           m.ID,
		Name:         m.Name,
		IdentityCard: cloneString(m.IdentityCard),
		Email:        cloneString(m.Email),
		Phone:        cloneString(m.Phone),
		CreatedAt:    m.CreatedAt.Time(),
	}
	if m.UpdatedAt != nil {
		updated := m.UpdatedAt.Time()
		client.UpdatedAt = &updated
	}
	return client
}

func clientModelFromDomain(client *domain.Client) clientModel {
	return clientModel{
		CreatedAt:    Timestamp(client.CreatedAt),
		UpdatedAt:    clientTimestamp(client.UpdatedAt),
		Name:         client.Name,
		IdentityCard: cloneString(client.IdentityCard),
		Email:        cloneString(client.Email),
		Phone:        cloneString(client.Phone),
	}
}

func clientTimestamp(value *time.Time) *Timestamp {
	if value == nil {
		return nil
	}
	converted := Timestamp(*value)
	return &converted
}

type clientStore struct {
	db *gorm.DB
}

var _ outbound.ClientRepository = (*clientStore)(nil)

func NewClientStore(db *gorm.DB) outbound.ClientRepository {
	return &clientStore{db: db}
}

func (s *clientStore) Get(ctx context.Context, id int64) (*domain.Client, error) {
	var record clientModel
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return record.toDomain(), nil
}

func (s *clientStore) List(ctx context.Context, skip, limit int, search string) ([]*domain.Client, bool, error) {
	search = strings.TrimSpace(search)
	var records []clientModel

	if search != "" && s.db.Dialector.Name() == "sqlite" {
		// ponytail: SQLite search scans the whole table so Go can case-fold Unicode; add FTS5 when the client table makes it measurable.
		if err := s.db.WithContext(ctx).Order("id DESC").Find(&records).Error; err != nil {
			return nil, false, storeError(s.db, "list clients", err)
		}
		needle := strings.ToLower(search)
		items, hasNext := filterPage(records, func(record clientModel) bool {
			return clientMatches(record, needle)
		}, skip, limit, clientModel.toDomain)
		return items, hasNext, nil
	}

	query := s.db.WithContext(ctx).Model(new(clientModel))
	if search != "" {
		query = query.Where(
			`strpos(lower(name), lower(?)) > 0
			OR strpos(lower(COALESCE(identity_card, '')), lower(?)) > 0
			OR strpos(lower(COALESCE(email, '')), lower(?)) > 0
			OR strpos(lower(COALESCE(phone, '')), lower(?)) > 0`,
			search, search, search, search,
		)
	}
	if err := query.Order("id DESC").Offset(skip).Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, false, storeError(s.db, "list clients", err)
	}
	items, hasNext := filterPage(records, nil, 0, limit, clientModel.toDomain)
	return items, hasNext, nil
}

func (s *clientStore) Create(ctx context.Context, client *domain.Client) (*domain.Client, error) {
	record := clientModelFromDomain(client)
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, storeError(s.db, "create client", err)
	}
	return record.toDomain(), nil
}

func (s *clientStore) Update(ctx context.Context, id int64, client *domain.Client) error {
	result := s.db.WithContext(ctx).Model(new(clientModel)).Where("id = ?", id).Updates(map[string]any{
		"updated_at":    clientTimestamp(client.UpdatedAt),
		"name":          client.Name,
		"identity_card": client.IdentityCard,
		"email":         client.Email,
		"phone":         client.Phone,
	})
	if result.Error != nil {
		return storeError(s.db, "update client", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *clientStore) Delete(ctx context.Context, id int64) (*domain.Client, error) {
	var record clientModel
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load client for delete: %w", err)
	}
	if err := s.db.WithContext(ctx).Delete(&record).Error; err != nil {
		return nil, storeError(s.db, "delete client", err)
	}
	return record.toDomain(), nil
}

func clientMatches(record clientModel, needle string) bool {
	fields := []string{record.Name}
	for _, value := range []*string{record.IdentityCard, record.Email, record.Phone} {
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
