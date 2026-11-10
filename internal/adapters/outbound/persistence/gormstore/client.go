package gormstore

import (
	"context"
	"database/sql"
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

func clientSelectSQL(sqlite bool) string {
	return `SELECT id, created_at, updated_at, name, identity_card, email, phone, ` +
		moneySQL(sqlite, "debt_amount") + ` FROM clients`
}

func scanClient(row rowScanner, sqlite bool) (*domain.Client, error) {
	var (
		id           int64
		createdAt    Timestamp
		updatedAt    nullableTimestamp
		name         string
		identityCard *string
		email        *string
		phone        *string
		debtRaw      any
	)
	err := row.Scan(&id, &createdAt, &updatedAt, &name, &identityCard, &email, &phone, &debtRaw)
	if err != nil {
		return nil, err
	}
	debt, err := moneyFromDB(sqlite, debtRaw)
	if err != nil {
		return nil, fmt.Errorf("decode client debt: %w", err)
	}
	return &domain.Client{
		ID:           id,
		Name:         name,
		IdentityCard: identityCard,
		Email:        email,
		Phone:        phone,
		DebtAmount:   debt,
		CreatedAt:    createdAt.Time(),
		UpdatedAt:    updatedAt.timePtr(),
	}, nil
}

type clientStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.ClientRepository = (*clientStore)(nil)

func NewClientStore(db *gorm.DB) outbound.ClientRepository {
	return &clientStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *clientStore) Get(ctx context.Context, id int64) (*domain.Client, error) {
	row := s.db.WithContext(ctx).Raw(clientSelectSQL(s.sqlite)+" WHERE id = ?", id).Row()
	client, err := scanClient(row, s.sqlite)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return client, nil
}

func (s *clientStore) List(ctx context.Context, skip, limit int, search string) ([]*domain.Client, bool, error) {
	search = strings.TrimSpace(search)

	if search != "" && s.sqlite {
		// ponytail: SQLite search scans the whole table so Go can case-fold Unicode; add FTS5 when the client table makes it measurable.
		rows, err := s.db.WithContext(ctx).Raw(clientSelectSQL(true) + " ORDER BY id DESC").Rows()
		if err != nil {
			return nil, false, storeError(s.db, "list clients", err)
		}
		items, scanErr := scanClients(rows, true)
		if scanErr != nil {
			return nil, false, storeError(s.db, "list clients", scanErr)
		}
		needle := strings.ToLower(search)
		matched := items[:0]
		for _, item := range items {
			if clientMatches(item, needle) {
				matched = append(matched, item)
			}
		}
		if skip >= len(matched) {
			matched = nil
		} else {
			matched = matched[skip:]
		}
		hasNext := len(matched) > limit
		if hasNext {
			matched = matched[:limit]
		}
		return matched, hasNext, nil
	}

	query := clientSelectSQL(s.sqlite)
	args := make([]any, 0, 6)
	if search != "" {
		query += ` WHERE strpos(lower(name), lower(?)) > 0
			OR strpos(lower(COALESCE(identity_card, '')), lower(?)) > 0
			OR strpos(lower(COALESCE(email, '')), lower(?)) > 0
			OR strpos(lower(COALESCE(phone, '')), lower(?)) > 0`
		args = append(args, search, search, search, search)
	}
	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit+1, skip)

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, false, storeError(s.db, "list clients", err)
	}
	items, err := scanClients(rows, s.sqlite)
	if err != nil {
		return nil, false, storeError(s.db, "list clients", err)
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return items, hasNext, nil
}

func scanClients(rows *sql.Rows, sqlite bool) ([]*domain.Client, error) {
	defer rows.Close()
	items := make([]*domain.Client, 0)
	for rows.Next() {
		client, err := scanClient(rows, sqlite)
		if err != nil {
			return nil, err
		}
		items = append(items, client)
	}
	return items, rows.Err()
}

func (s *clientStore) Create(ctx context.Context, client *domain.Client) (*domain.Client, error) {
	record := clientModelFromDomain(client)
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, storeError(s.db, "create client", err)
	}
	return s.Get(ctx, record.ID)
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
	client, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	result := s.db.WithContext(ctx).Exec("DELETE FROM clients WHERE id = ?", id)
	if result.Error != nil {
		return nil, storeError(s.db, "delete client", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return client, nil
}

func clientMatches(record *domain.Client, needle string) bool {
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
