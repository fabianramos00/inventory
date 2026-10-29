package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type fakeClientStore struct {
	getResult *domain.Client
	getErr    error

	listItems   []*domain.Client
	listHasNext bool
	listErr     error
	listSkip    int
	listLimit   int
	listSearch  string

	createResult *domain.Client
	createErr    error
	created      *domain.Client

	updateErr   error
	updateID    int64
	updateCalls int
	updated     *domain.Client

	deleteResult *domain.Client
	deleteErr    error
	deleteID     int64
}

func (f *fakeClientStore) Get(context.Context, int64) (*domain.Client, error) {
	return f.getResult, f.getErr
}

func (f *fakeClientStore) List(_ context.Context, skip, limit int, search string) ([]*domain.Client, bool, error) {
	f.listSkip, f.listLimit, f.listSearch = skip, limit, search
	return f.listItems, f.listHasNext, f.listErr
}

func (f *fakeClientStore) Create(_ context.Context, client *domain.Client) (*domain.Client, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	client.ID = 7
	f.created = client
	if f.createResult != nil {
		return f.createResult, nil
	}
	return client, nil
}

func (f *fakeClientStore) Update(_ context.Context, id int64, client *domain.Client) error {
	f.updateCalls++
	f.updateID, f.updated = id, client
	return f.updateErr
}

func (f *fakeClientStore) Delete(_ context.Context, id int64) (*domain.Client, error) {
	f.deleteID = id
	return f.deleteResult, f.deleteErr
}

func clientStrPtr(value string) *string {
	return &value
}

func newClientFixture() *domain.Client {
	return &domain.Client{
		ID:           3,
		Name:         "Ana",
		IdentityCard: clientStrPtr("123"),
		Email:        clientStrPtr("ana@example.com"),
		CreatedAt:    time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestClientCreateTrimsAndClearsOptionalFields(t *testing.T) {
	store := &fakeClientStore{}
	service := NewClientService(store)

	before := time.Now().UTC().Add(-time.Microsecond)
	client, err := service.Create(context.Background(), inbound.CreateClientInput{
		Name:         "  Ana Pérez  ",
		IdentityCard: clientStrPtr(" 123 "),
		Email:        clientStrPtr("   "),
		Phone:        clientStrPtr(" 555 "),
	})
	after := time.Now().UTC().Add(time.Microsecond)
	if err != nil {
		t.Fatal(err)
	}
	if client.ID != 7 || client.Name != "Ana Pérez" {
		t.Fatalf("client = %+v", client)
	}
	if client.IdentityCard == nil || *client.IdentityCard != "123" {
		t.Fatalf("identity = %v, want 123", client.IdentityCard)
	}
	if client.Email != nil {
		t.Fatalf("email = %v, want nil", client.Email)
	}
	if client.Phone == nil || *client.Phone != "555" {
		t.Fatalf("phone = %v, want 555", client.Phone)
	}
	if client.UpdatedAt != nil {
		t.Fatalf("updated_at = %v, want nil", client.UpdatedAt)
	}
	if client.CreatedAt.Before(before) || client.CreatedAt.After(after) {
		t.Fatalf("created_at = %v, want between %v and %v", client.CreatedAt, before, after)
	}
	if client.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at location = %v, want UTC", client.CreatedAt.Location())
	}
	if client.CreatedAt.Nanosecond()%int(time.Microsecond) != 0 {
		t.Fatalf("created_at = %v, want microsecond truncation", client.CreatedAt)
	}
	if store.created != client {
		t.Fatal("created client was not persisted through the repository")
	}
}

func TestClientCreateRequiresName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		t.Run(name, func(t *testing.T) {
			store := &fakeClientStore{}
			service := NewClientService(store)
			if _, err := service.Create(context.Background(), inbound.CreateClientInput{Name: name}); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
			if store.created != nil {
				t.Fatal("invalid client reached the repository")
			}
		})
	}
}

func TestClientListPassesPaginationAndTrimsSearch(t *testing.T) {
	store := &fakeClientStore{
		listItems:   []*domain.Client{newClientFixture()},
		listHasNext: true,
	}
	service := NewClientService(store)

	result, err := service.List(context.Background(), inbound.ClientFilter{Skip: 2, Limit: 5, Search: "  ana  "})
	if err != nil {
		t.Fatal(err)
	}
	if store.listSkip != 2 || store.listLimit != 5 || store.listSearch != "ana" {
		t.Fatalf("repository args = %d/%d/%q", store.listSkip, store.listLimit, store.listSearch)
	}
	if !result.HasNext || result.Skip != 2 || result.Limit != 5 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestClientUpdatePartialAndTimestamps(t *testing.T) {
	t.Run("empty update changes nothing", func(t *testing.T) {
		store := &fakeClientStore{getResult: newClientFixture()}
		service := NewClientService(store)
		client, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{})
		if err != nil {
			t.Fatal(err)
		}
		if store.updateCalls != 0 || client.UpdatedAt != nil {
			t.Fatalf("update calls = %d updated_at = %v, want 0 and nil", store.updateCalls, client.UpdatedAt)
		}
	})

	t.Run("whitespace-only name is unchanged", func(t *testing.T) {
		store := &fakeClientStore{getResult: newClientFixture()}
		service := NewClientService(store)
		client, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
			Name: inbound.Optional[string]{Set: true, Value: clientStrPtr("  Ana  ")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if store.updateCalls != 0 || client.UpdatedAt != nil {
			t.Fatalf("update calls = %d updated_at = %v, want 0 and nil", store.updateCalls, client.UpdatedAt)
		}
	})

	t.Run("changed name sets updated timestamp", func(t *testing.T) {
		fixture := newClientFixture()
		store := &fakeClientStore{getResult: fixture}
		service := NewClientService(store)
		before := time.Now().UTC().Add(-time.Microsecond)
		client, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
			Name: inbound.Optional[string]{Set: true, Value: clientStrPtr("Ana Pérez")},
		})
		after := time.Now().UTC().Add(time.Microsecond)
		if err != nil {
			t.Fatal(err)
		}
		if store.updateCalls != 1 || store.updateID != 3 {
			t.Fatalf("update calls = %d id = %d, want 1 and 3", store.updateCalls, store.updateID)
		}
		if client.Name != "Ana Pérez" || client.UpdatedAt == nil {
			t.Fatalf("client = %+v", client)
		}
		if client.UpdatedAt.Before(before) || client.UpdatedAt.After(after) {
			t.Fatalf("updated_at = %v, want between %v and %v", client.UpdatedAt, before, after)
		}
		if client.UpdatedAt.Nanosecond()%int(time.Microsecond) != 0 {
			t.Fatalf("updated_at = %v, want microsecond truncation", client.UpdatedAt)
		}
		if !client.CreatedAt.Equal(fixture.CreatedAt) {
			t.Fatalf("created_at changed to %v", client.CreatedAt)
		}
	})

	t.Run("null clears optional fields", func(t *testing.T) {
		fixture := newClientFixture()
		fixture.Phone = clientStrPtr("555")
		store := &fakeClientStore{getResult: fixture}
		service := NewClientService(store)
		client, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
			IdentityCard: inbound.Optional[string]{Set: true, Value: nil},
			Email:        inbound.Optional[string]{Set: true, Value: clientStrPtr("   ")},
			Phone:        inbound.Optional[string]{Set: true, Value: clientStrPtr(" 777 ")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if client.IdentityCard != nil || client.Email != nil {
			t.Fatalf("identity/email = %v/%v, want nil/nil", client.IdentityCard, client.Email)
		}
		if client.Phone == nil || *client.Phone != "777" {
			t.Fatalf("phone = %v, want 777", client.Phone)
		}
		if client.UpdatedAt == nil {
			t.Fatal("updated_at is nil after a change")
		}
	})

	t.Run("omitted fields stay", func(t *testing.T) {
		fixture := newClientFixture()
		store := &fakeClientStore{getResult: fixture}
		service := NewClientService(store)
		client, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
			Phone: inbound.Optional[string]{Set: true, Value: clientStrPtr("777")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if client.Name != "Ana" || client.IdentityCard == nil || *client.IdentityCard != "123" ||
			client.Email == nil || *client.Email != "ana@example.com" {
			t.Fatalf("client = %+v", client)
		}
	})

	t.Run("same optional value is unchanged", func(t *testing.T) {
		store := &fakeClientStore{getResult: newClientFixture()}
		service := NewClientService(store)
		client, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
			Email: inbound.Optional[string]{Set: true, Value: clientStrPtr(" ana@example.com ")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if store.updateCalls != 0 || client.UpdatedAt != nil {
			t.Fatalf("update calls = %d updated_at = %v, want 0 and nil", store.updateCalls, client.UpdatedAt)
		}
	})

	for _, name := range []*string{nil, clientStrPtr(""), clientStrPtr("   ")} {
		t.Run("invalid name", func(t *testing.T) {
			store := &fakeClientStore{getResult: newClientFixture()}
			service := NewClientService(store)
			if _, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
				Name: inbound.Optional[string]{Set: true, Value: name},
			}); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
			if store.updateCalls != 0 {
				t.Fatal("invalid update reached the repository")
			}
		})
	}

	t.Run("not found and repository errors propagate", func(t *testing.T) {
		service := NewClientService(&fakeClientStore{getErr: domain.ErrNotFound})
		if _, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("get error = %v, want ErrNotFound", err)
		}

		store := &fakeClientStore{getResult: newClientFixture(), updateErr: domain.ErrConflict}
		service = NewClientService(store)
		if _, err := service.Update(context.Background(), 3, inbound.UpdateClientInput{
			Name: inbound.Optional[string]{Set: true, Value: clientStrPtr("Other")},
		}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("update error = %v, want ErrConflict", err)
		}
	})
}

func TestClientDeletePassesThrough(t *testing.T) {
	deleted := newClientFixture()
	store := &fakeClientStore{deleteResult: deleted}
	service := NewClientService(store)

	got, err := service.Delete(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if got != deleted || store.deleteID != 3 {
		t.Fatalf("deleted = %+v id = %d", got, store.deleteID)
	}
}
