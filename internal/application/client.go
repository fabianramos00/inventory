package application

import (
	"context"
	"strings"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

type ClientService struct {
	repository outbound.ClientRepository
}

func NewClientService(repository outbound.ClientRepository) *ClientService {
	return &ClientService{repository: repository}
}

var _ inbound.ClientUseCase = (*ClientService)(nil)

func (s *ClientService) List(ctx context.Context, filter inbound.ClientFilter) (*inbound.ClientList, error) {
	items, hasNext, err := s.repository.List(ctx, filter.Skip, filter.Limit, strings.TrimSpace(filter.Search))
	if err != nil {
		return nil, err
	}
	return &inbound.ClientList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *ClientService) Get(ctx context.Context, id int64) (*domain.Client, error) {
	return s.repository.Get(ctx, id)
}

func (s *ClientService) Create(ctx context.Context, input inbound.CreateClientInput) (*domain.Client, error) {
	name, err := clientName(input.Name)
	if err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, &domain.Client{
		Name:         name,
		IdentityCard: cleanClientText(input.IdentityCard),
		Email:        cleanClientText(input.Email),
		Phone:        cleanClientText(input.Phone),
		CreatedAt:    nowUTC(),
	})
}

func (s *ClientService) Update(ctx context.Context, id int64, input inbound.UpdateClientInput) (*domain.Client, error) {
	client, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	changed := false
	if input.Name.Set {
		if input.Name.Value == nil {
			return nil, requiredField("name")
		}
		name, err := clientName(*input.Name.Value)
		if err != nil {
			return nil, err
		}
		if client.Name != name {
			client.Name = name
			changed = true
		}
	}
	for _, field := range []struct {
		input  inbound.Optional[string]
		target **string
	}{
		{input.IdentityCard, &client.IdentityCard},
		{input.Email, &client.Email},
		{input.Phone, &client.Phone},
	} {
		if !field.input.Set {
			continue
		}
		cleaned := cleanClientText(field.input.Value)
		if !sameText(*field.target, cleaned) {
			*field.target = cleaned
			changed = true
		}
	}
	if !changed {
		return client, nil
	}

	now := nowUTC()
	client.UpdatedAt = &now
	if err := s.repository.Update(ctx, id, client); err != nil {
		return nil, err
	}
	return client, nil
}

func (s *ClientService) Delete(ctx context.Context, id int64) (*domain.Client, error) {
	return s.repository.Delete(ctx, id)
}

func clientName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", requiredField("name")
	}
	return name, nil
}

func cleanClientText(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}
