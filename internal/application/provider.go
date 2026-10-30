package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

const (
	maxProviderName        = 255
	maxProviderContactInfo = 500
	maxProviderEmail       = 255
	maxProviderPhone       = 50
)

type ProviderService struct {
	repository outbound.ProviderRepository
}

func NewProviderService(repository outbound.ProviderRepository) *ProviderService {
	return &ProviderService{repository: repository}
}

var _ inbound.ProviderUseCase = (*ProviderService)(nil)

func (s *ProviderService) List(ctx context.Context, filter inbound.ProviderFilter) (*inbound.ProviderList, error) {
	items, hasNext, err := s.repository.List(ctx, filter.Skip, filter.Limit, strings.TrimSpace(filter.Search))
	if err != nil {
		return nil, err
	}
	return &inbound.ProviderList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *ProviderService) Get(ctx context.Context, id int64) (*domain.Provider, error) {
	return s.repository.Get(ctx, id)
}

func (s *ProviderService) Create(ctx context.Context, input inbound.CreateProviderInput) (*domain.Provider, error) {
	name, err := providerName(input.Name)
	if err != nil {
		return nil, err
	}
	contactInfo, err := providerOptional(input.ContactInfo, "contact_info", maxProviderContactInfo)
	if err != nil {
		return nil, err
	}
	email, err := providerOptional(input.Email, "email", maxProviderEmail)
	if err != nil {
		return nil, err
	}
	phone, err := providerOptional(input.Phone, "phone", maxProviderPhone)
	if err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, &domain.Provider{
		Name:        name,
		ContactInfo: contactInfo,
		Email:       email,
		Phone:       phone,
		CreatedByID: clonePtr(input.CreatedByID),
	})
}

func (s *ProviderService) Update(ctx context.Context, id int64, input inbound.UpdateProviderInput) (*domain.Provider, error) {
	provider, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	changed := false
	if input.Name.Set {
		if input.Name.Value == nil {
			return nil, requiredField("name")
		}
		name, err := providerName(*input.Name.Value)
		if err != nil {
			return nil, err
		}
		if provider.Name != name {
			provider.Name = name
			changed = true
		}
	}
	for _, field := range []struct {
		input  inbound.Optional[string]
		target **string
		label  string
		max    int
	}{
		{input.ContactInfo, &provider.ContactInfo, "contact_info", maxProviderContactInfo},
		{input.Email, &provider.Email, "email", maxProviderEmail},
		{input.Phone, &provider.Phone, "phone", maxProviderPhone},
	} {
		if !field.input.Set {
			continue
		}
		cleaned, err := providerOptional(field.input.Value, field.label, field.max)
		if err != nil {
			return nil, err
		}
		if !sameText(*field.target, cleaned) {
			*field.target = cleaned
			changed = true
		}
	}
	if !changed {
		return provider, nil
	}
	if err := s.repository.Update(ctx, id, provider); err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *ProviderService) Delete(ctx context.Context, id int64) (*domain.Provider, error) {
	return s.repository.Delete(ctx, id)
}

func providerName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", requiredField("name")
	}
	if utf8.RuneCountInString(name) > maxProviderName {
		return "", tooLong("name", maxProviderName)
	}
	return name, nil
}

func providerOptional(value *string, field string, max int) (*string, error) {
	if value == nil {
		return nil, nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(cleaned) > max {
		return nil, tooLong(field, max)
	}
	return &cleaned, nil
}
