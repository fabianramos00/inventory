package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type fakeProviderStore struct {
	listItems   []*domain.Provider
	listHasNext bool
	listErr     error
	listSkip    int
	listLimit   int
	listSearch  string

	getResult *domain.Provider
	getErr    error

	createResult *domain.Provider
	createErr    error
	created      *domain.Provider

	updateErr   error
	updateID    int64
	updateCalls int
	updated     *domain.Provider

	deleteResult *domain.Provider
	deleteErr    error
	deleteID     int64
}

func (f *fakeProviderStore) List(_ context.Context, skip, limit int, search string) ([]*domain.Provider, bool, error) {
	f.listSkip, f.listLimit, f.listSearch = skip, limit, search
	return f.listItems, f.listHasNext, f.listErr
}

func (f *fakeProviderStore) Get(context.Context, int64) (*domain.Provider, error) {
	return f.getResult, f.getErr
}

func (f *fakeProviderStore) Create(_ context.Context, provider *domain.Provider) (*domain.Provider, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	provider.ID = 7
	f.created = provider
	if f.createResult != nil {
		return f.createResult, nil
	}
	return provider, nil
}

func (f *fakeProviderStore) Update(_ context.Context, id int64, provider *domain.Provider) error {
	f.updateCalls++
	f.updateID, f.updated = id, provider
	return f.updateErr
}

func (f *fakeProviderStore) Delete(_ context.Context, id int64) (*domain.Provider, error) {
	f.deleteID = id
	return f.deleteResult, f.deleteErr
}

func providerString(value string) *string { return &value }

func TestProviderCreateTrimsClearsAndSetsCreator(t *testing.T) {
	store := &fakeProviderStore{}
	service := NewProviderService(store)
	creator := int64(42)

	provider, err := service.Create(context.Background(), inbound.CreateProviderInput{
		Name:        "  Distribuidora Metálica  ",
		ContactInfo: providerString("  Juan Pérez "),
		Email:       providerString("   "),
		Phone:       providerString(" 3001234567 "),
		CreatedByID: &creator,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.ID != 7 || provider.Name != "Distribuidora Metálica" {
		t.Fatalf("provider = %+v", provider)
	}
	if provider.ContactInfo == nil || *provider.ContactInfo != "Juan Pérez" {
		t.Fatalf("contact_info = %v", provider.ContactInfo)
	}
	if provider.Email != nil {
		t.Fatalf("email = %v, want nil", provider.Email)
	}
	if provider.Phone == nil || *provider.Phone != "3001234567" {
		t.Fatalf("phone = %v", provider.Phone)
	}
	if provider.CreatedByID == nil || *provider.CreatedByID != 42 {
		t.Fatalf("created_by_id = %v, want 42", provider.CreatedByID)
	}
	if store.created != provider {
		t.Fatal("provider was not persisted through the repository")
	}
}

func TestProviderCreateValidation(t *testing.T) {
	tests := []struct {
		name  string
		input inbound.CreateProviderInput
	}{
		{name: "blank name", input: inbound.CreateProviderInput{Name: "  "}},
		{name: "long name", input: inbound.CreateProviderInput{Name: strings.Repeat("a", 256)}},
		{name: "long contact", input: inbound.CreateProviderInput{Name: "P", ContactInfo: providerString(strings.Repeat("a", 501))}},
		{name: "long email", input: inbound.CreateProviderInput{Name: "P", Email: providerString(strings.Repeat("a", 256))}},
		{name: "long phone", input: inbound.CreateProviderInput{Name: "P", Phone: providerString(strings.Repeat("a", 51))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeProviderStore{}
			service := NewProviderService(store)
			if _, err := service.Create(context.Background(), tt.input); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
			if store.created != nil {
				t.Fatal("invalid provider reached the repository")
			}
		})
	}
}

func TestProviderUpdatePartialClearAndNoOp(t *testing.T) {
	empty := ""
	provider := &domain.Provider{
		ID:          3,
		Name:        "Original",
		ContactInfo: providerString("Contact"),
		Email:       providerString("mail@example.com"),
		Phone:       providerString("300"),
	}
	store := &fakeProviderStore{getResult: provider}
	service := NewProviderService(store)

	updated, err := service.Update(context.Background(), 3, inbound.UpdateProviderInput{
		Name:  inbound.Optional[string]{Set: true, Value: providerString("  Renamed  ")},
		Email: inbound.Optional[string]{Set: true, Value: nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.updateCalls != 1 || store.updateID != 3 {
		t.Fatalf("update calls/id = %d/%d", store.updateCalls, store.updateID)
	}
	if updated.Name != "Renamed" || updated.Email != nil {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.ContactInfo == nil || *updated.ContactInfo != "Contact" || updated.Phone == nil || *updated.Phone != "300" {
		t.Fatalf("omitted fields changed: %+v", updated)
	}

	store.getResult = &domain.Provider{ID: 3, Name: "Same", ContactInfo: providerString("Contact")}
	before := store.updateCalls
	noop, err := service.Update(context.Background(), 3, inbound.UpdateProviderInput{
		Name:        inbound.Optional[string]{Set: true, Value: providerString(" Same ")},
		ContactInfo: inbound.Optional[string]{Set: true, Value: providerString(" Contact ")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.updateCalls != before {
		t.Fatal("no-op update reached the repository")
	}
	if noop.Name != "Same" || noop.ContactInfo == nil || *noop.ContactInfo != "Contact" {
		t.Fatalf("no-op provider = %+v", noop)
	}

	store.getResult = &domain.Provider{ID: 3, Name: "Same", Phone: providerString("300")}
	if _, err := service.Update(context.Background(), 3, inbound.UpdateProviderInput{
		Phone: inbound.Optional[string]{Set: true, Value: &empty},
	}); err != nil {
		t.Fatal(err)
	}
	if store.updateCalls != before+1 || store.updated.Phone != nil {
		t.Fatalf("empty string did not clear phone: %+v", store.updated)
	}
}

func TestProviderUpdateRejectsBlankNameAndPropagatesErrors(t *testing.T) {
	store := &fakeProviderStore{getResult: &domain.Provider{ID: 3, Name: "Original"}}
	service := NewProviderService(store)

	if _, err := service.Update(context.Background(), 3, inbound.UpdateProviderInput{
		Name: inbound.Optional[string]{Set: true, Value: nil},
	}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("null name error = %v, want ErrValidation", err)
	}
	if _, err := service.Update(context.Background(), 3, inbound.UpdateProviderInput{
		Name: inbound.Optional[string]{Set: true, Value: providerString("   ")},
	}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("blank name error = %v, want ErrValidation", err)
	}
	if store.updateCalls != 0 {
		t.Fatal("invalid update reached the repository")
	}

	want := errors.New("database down")
	store.getErr = want
	if _, err := service.Update(context.Background(), 4, inbound.UpdateProviderInput{}); !errors.Is(err, want) {
		t.Fatalf("get error = %v, want %v", err, want)
	}
	store.getErr = nil
	store.getResult = &domain.Provider{ID: 3, Name: "Original"}
	store.updateErr = want
	if _, err := service.Update(context.Background(), 3, inbound.UpdateProviderInput{
		Name: inbound.Optional[string]{Set: true, Value: providerString("Changed")},
	}); !errors.Is(err, want) {
		t.Fatalf("update error = %v, want %v", err, want)
	}
}

func TestProviderListPassesPaginationAndTrimsSearch(t *testing.T) {
	store := &fakeProviderStore{
		listItems:   []*domain.Provider{{ID: 1, Name: "Distribuidora"}},
		listHasNext: true,
	}
	service := NewProviderService(store)

	result, err := service.List(context.Background(), inbound.ProviderFilter{Skip: 2, Limit: 5, Search: "  metálica  "})
	if err != nil {
		t.Fatal(err)
	}
	if store.listSkip != 2 || store.listLimit != 5 || store.listSearch != "metálica" {
		t.Fatalf("repository args = %d/%d/%q", store.listSkip, store.listLimit, store.listSearch)
	}
	if !result.HasNext || result.Skip != 2 || result.Limit != 5 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}

	want := errors.New("list failed")
	store.listErr = want
	if _, err := service.List(context.Background(), inbound.ProviderFilter{}); !errors.Is(err, want) {
		t.Fatalf("list error = %v, want %v", err, want)
	}
}

func TestProviderGetDeleteAndErrorPropagation(t *testing.T) {
	store := &fakeProviderStore{getResult: &domain.Provider{ID: 5, Name: "P"}, deleteResult: &domain.Provider{ID: 5, Name: "P"}}
	service := NewProviderService(store)

	if got, err := service.Get(context.Background(), 5); err != nil || got.ID != 5 {
		t.Fatalf("get = %+v / %v", got, err)
	}
	if got, err := service.Delete(context.Background(), 5); err != nil || store.deleteID != 5 || got.ID != 5 {
		t.Fatalf("delete = %+v / %v id %d", got, err, store.deleteID)
	}

	want := domain.ErrNotFound
	store.getErr = want
	if _, err := service.Get(context.Background(), 6); !errors.Is(err, want) {
		t.Fatalf("get error = %v, want %v", err, want)
	}
	store.deleteErr = want
	if _, err := service.Delete(context.Background(), 6); !errors.Is(err, want) {
		t.Fatalf("delete error = %v, want %v", err, want)
	}
}
