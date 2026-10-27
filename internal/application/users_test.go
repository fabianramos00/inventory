package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type fakeUserStore struct {
	users   map[int64]*domain.User
	nextID  int64
	listErr error

	listItems   []*domain.User
	listHasNext bool
	listSkip    int
	listLimit   int
	listSearch  string

	getErr    error
	createErr error
	updateErr error
	deleteErr error

	created []*domain.User
	updated []*domain.User
	deleted []int64
}

func (f *fakeUserStore) GetByID(_ context.Context, id int64) (*domain.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	user, ok := f.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return user, nil
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, user := range f.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUserStore) List(_ context.Context, skip, limit int, search string) ([]*domain.User, bool, error) {
	f.listSkip = skip
	f.listLimit = limit
	f.listSearch = search
	if f.listErr != nil {
		return nil, false, f.listErr
	}
	return f.listItems, f.listHasNext, nil
}

func (f *fakeUserStore) Create(_ context.Context, user *domain.User) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.nextID++
	user.ID = f.nextID
	f.users[user.ID] = user
	f.created = append(f.created, user)
	return nil
}

func (f *fakeUserStore) Update(_ context.Context, user *domain.User) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.users[user.ID] = user
	f.updated = append(f.updated, user)
	return nil
}

func (f *fakeUserStore) Delete(_ context.Context, id int64) (*domain.User, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	user, ok := f.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	delete(f.users, id)
	f.deleted = append(f.deleted, id)
	return user, nil
}

type fakeUserHasher struct {
	hashed  []string
	hashErr error
}

func (f *fakeUserHasher) Hash(password string) (string, error) {
	if f.hashErr != nil {
		return "", f.hashErr
	}
	f.hashed = append(f.hashed, password)
	return "hashed:" + password, nil
}

func (f *fakeUserHasher) Verify(password, hash string) bool {
	return hash == "hashed:"+password
}

type userHarness struct {
	service *UserService
	store   *fakeUserStore
	hasher  *fakeUserHasher
}

func newUserHarness(users ...*domain.User) *userHarness {
	store := &fakeUserStore{users: map[int64]*domain.User{}}
	for _, user := range users {
		store.users[user.ID] = user
		if user.ID > store.nextID {
			store.nextID = user.ID
		}
	}
	hasher := &fakeUserHasher{}
	return &userHarness{
		service: NewUserService(store, hasher),
		store:   store,
		hasher:  hasher,
	}
}

func userRecord(id int64, email string) *domain.User {
	return &domain.User{
		ID:             id,
		FullName:       "Ada Lovelace",
		Email:          email,
		HashedPassword: "hash-" + email,
		IsActive:       true,
	}
}

func TestUserListPassesPaginationAndSearch(t *testing.T) {
	first := userRecord(1, "ada@example.com")
	second := userRecord(2, "grace@example.com")
	h := newUserHarness(first, second)
	h.store.listItems = []*domain.User{first}
	h.store.listHasNext = true

	result, err := h.service.List(context.Background(), 5, 10, "  ada  ")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if h.store.listSkip != 5 || h.store.listLimit != 10 {
		t.Errorf("repository received skip=%d limit=%d, want 5 and 10", h.store.listSkip, h.store.listLimit)
	}
	if h.store.listSearch != "ada" {
		t.Errorf("repository search = %q, want trimmed %q", h.store.listSearch, "ada")
	}
	if result.Skip != 5 || result.Limit != 10 {
		t.Errorf("result skip/limit = %d/%d, want 5/10", result.Skip, result.Limit)
	}
	if !result.HasNext {
		t.Error("result has_next = false, want true")
	}
	if len(result.Items) != 1 || result.Items[0] != first {
		t.Errorf("result items = %v, want the repository items", result.Items)
	}
}

func TestUserListPropagatesRepositoryError(t *testing.T) {
	h := newUserHarness()
	h.store.listErr = errors.New("boom")

	if _, err := h.service.List(context.Background(), 0, 10, ""); err == nil {
		t.Fatal("List() error = nil, want the repository error")
	}
}

func TestUserCreateHashesPasswordAndAppliesDefaults(t *testing.T) {
	h := newUserHarness()

	created, err := h.service.Create(context.Background(), inbound.CreateUserInput{
		Email:    "  new@example.com  ",
		Password: "password123",
		FullName: "  New User  ",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if len(h.hasher.hashed) != 1 || h.hasher.hashed[0] != "password123" {
		t.Fatalf("hashed passwords = %v, want [password123]", h.hasher.hashed)
	}
	if created.HashedPassword != "hashed:password123" {
		t.Errorf("HashedPassword = %q, want the hasher output", created.HashedPassword)
	}
	if created.Email != "new@example.com" {
		t.Errorf("Email = %q, want trimmed new@example.com", created.Email)
	}
	if created.FullName != "New User" {
		t.Errorf("FullName = %q, want trimmed New User", created.FullName)
	}
	if !created.IsActive {
		t.Error("IsActive = false, want default true")
	}
	if created.IsSuperuser {
		t.Error("IsSuperuser = true, want default false")
	}
	if len(h.store.created) != 1 || h.store.created[0] != created {
		t.Errorf("repository created = %v, want the returned user", h.store.created)
	}
	if created.ID <= 0 {
		t.Errorf("ID = %d, want the repository-assigned id", created.ID)
	}
}

func TestUserCreateHonorsExplicitFlags(t *testing.T) {
	h := newUserHarness()
	inactive := false
	superuser := true

	created, err := h.service.Create(context.Background(), inbound.CreateUserInput{
		Email:       "admin@example.com",
		Password:    "password123",
		IsActive:    &inactive,
		IsSuperuser: &superuser,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.IsActive {
		t.Error("IsActive = true, want the explicit false")
	}
	if !created.IsSuperuser {
		t.Error("IsSuperuser = false, want the explicit true")
	}
}

func TestUserCreateDuplicateEmail(t *testing.T) {
	h := newUserHarness(userRecord(1, "taken@example.com"))

	_, err := h.service.Create(context.Background(), inbound.CreateUserInput{
		Email:    "taken@example.com",
		Password: "password123",
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Create() error = %v, want ErrConflict", err)
	}
	if len(h.hasher.hashed) != 0 {
		t.Errorf("hashed %d passwords, want 0 before the conflict check", len(h.hasher.hashed))
	}
	if len(h.store.created) != 0 {
		t.Errorf("created %d users, want 0", len(h.store.created))
	}
}

func TestUserCreateValidation(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
	}{
		{name: "invalid email", email: "not-an-email", password: "password123"},
		{name: "display-name email", email: "Ada <ada@example.com>", password: "password123"},
		{name: "short password", email: "valid@example.com", password: "short"},
		{name: "password over 72 bytes", email: "valid@example.com", password: strings.Repeat("a", 73)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newUserHarness()

			_, err := h.service.Create(context.Background(), inbound.CreateUserInput{
				Email:    tt.email,
				Password: tt.password,
			})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("Create() error = %v, want ErrValidation", err)
			}
			if len(h.hasher.hashed) != 0 {
				t.Errorf("hashed %d passwords, want 0", len(h.hasher.hashed))
			}
			if len(h.store.created) != 0 {
				t.Errorf("created %d users, want 0", len(h.store.created))
			}
		})
	}
}

func TestUserUpdatePartialFields(t *testing.T) {
	existing := userRecord(1, "ada@example.com")
	existing.HashedPassword = "hash-existing"
	h := newUserHarness(existing)

	name := "  Ada Byron  "
	inactive := false
	emptyPassword := ""

	updated, err := h.service.Update(context.Background(), 1, inbound.UpdateUserInput{
		FullName: &name,
		IsActive: &inactive,
		Password: &emptyPassword,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.FullName != "Ada Byron" {
		t.Errorf("FullName = %q, want trimmed Ada Byron", updated.FullName)
	}
	if updated.IsActive {
		t.Error("IsActive = true, want false")
	}
	if updated.Email != "ada@example.com" {
		t.Errorf("Email = %q, want the unchanged email", updated.Email)
	}
	if updated.HashedPassword != "hash-existing" {
		t.Errorf("HashedPassword = %q, want the existing hash", updated.HashedPassword)
	}
	if len(h.hasher.hashed) != 0 {
		t.Errorf("hashed %d passwords for an empty password, want 0", len(h.hasher.hashed))
	}
	if len(h.store.updated) != 1 {
		t.Fatalf("repository updates = %d, want 1", len(h.store.updated))
	}
}

func TestUserUpdateEmailAndPassword(t *testing.T) {
	existing := userRecord(1, "ada@example.com")
	h := newUserHarness(existing)
	email := "  ada.byron@example.com "
	password := "newpassword123"
	superuser := true

	updated, err := h.service.Update(context.Background(), 1, inbound.UpdateUserInput{
		Email:       &email,
		Password:    &password,
		IsSuperuser: &superuser,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.Email != "ada.byron@example.com" {
		t.Errorf("Email = %q, want trimmed ada.byron@example.com", updated.Email)
	}
	if updated.HashedPassword != "hashed:newpassword123" {
		t.Errorf("HashedPassword = %q, want the new hash", updated.HashedPassword)
	}
	if !updated.IsSuperuser {
		t.Error("IsSuperuser = false, want true")
	}
	if len(h.hasher.hashed) != 1 || h.hasher.hashed[0] != "newpassword123" {
		t.Errorf("hashed passwords = %v, want [newpassword123]", h.hasher.hashed)
	}
}

func TestUserUpdateDuplicateEmail(t *testing.T) {
	first := userRecord(1, "ada@example.com")
	second := userRecord(2, "grace@example.com")
	h := newUserHarness(first, second)
	email := "grace@example.com"

	_, err := h.service.Update(context.Background(), 1, inbound.UpdateUserInput{Email: &email})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Update() error = %v, want ErrConflict", err)
	}
	if len(h.store.updated) != 0 {
		t.Errorf("repository updates = %d, want 0", len(h.store.updated))
	}
	if first.Email != "ada@example.com" {
		t.Errorf("stored email = %q, want the original", first.Email)
	}
}

func TestUserUpdateMissing(t *testing.T) {
	h := newUserHarness()
	name := "Ghost"

	_, err := h.service.Update(context.Background(), 99, inbound.UpdateUserInput{FullName: &name})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestUserDeleteReturnsDeletedUser(t *testing.T) {
	existing := userRecord(1, "ada@example.com")
	h := newUserHarness(existing)

	deleted, err := h.service.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleted != existing {
		t.Errorf("Delete() = %v, want the deleted user", deleted)
	}
	if _, err := h.service.Get(context.Background(), 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestUserDeleteMissing(t *testing.T) {
	h := newUserHarness()

	if _, err := h.service.Delete(context.Background(), 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}
