package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

var _ inbound.AuthUseCase = (*AuthService)(nil)

const testRefreshTTL = time.Hour

type fakeUserRepository struct {
	users      map[int64]*domain.User
	getByIDErr error
}

func (f *fakeUserRepository) GetByID(_ context.Context, id int64) (*domain.User, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	user, ok := f.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return user, nil
}

func (f *fakeUserRepository) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, user := range f.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUserRepository) Create(context.Context, *domain.User) error { return nil }
func (f *fakeUserRepository) Update(context.Context, *domain.User) error { return nil }
func (f *fakeUserRepository) List(context.Context, int, int, string) ([]*domain.User, bool, error) {
	return nil, false, nil
}
func (f *fakeUserRepository) Delete(context.Context, int64) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

type createdRefreshToken struct {
	userID    int64
	hash      string
	expiresAt time.Time
}

type rotatedRefreshToken struct {
	oldHash   string
	newHash   string
	now       time.Time
	expiresAt time.Time
}

type fakeRefreshTokenRepository struct {
	validUserID  int64
	hasValid     bool
	findErr      error
	rotateErr    error
	revokeErr    error
	revokeAllErr error

	findHash     string
	findNow      time.Time
	created      []createdRefreshToken
	rotated      []rotatedRefreshToken
	revoked      []string
	revokedUsers []int64
}

func (f *fakeRefreshTokenRepository) Create(_ context.Context, userID int64, hash string, expiresAt time.Time) error {
	f.created = append(f.created, createdRefreshToken{userID: userID, hash: hash, expiresAt: expiresAt})
	return nil
}

func (f *fakeRefreshTokenRepository) FindValid(_ context.Context, hash string, now time.Time) (int64, error) {
	f.findHash = hash
	f.findNow = now
	if f.findErr != nil {
		return 0, f.findErr
	}
	if !f.hasValid {
		return 0, domain.ErrNotFound
	}
	return f.validUserID, nil
}

func (f *fakeRefreshTokenRepository) Rotate(_ context.Context, oldHash, newHash string, now, expiresAt time.Time) error {
	if f.rotateErr != nil {
		return f.rotateErr
	}
	f.rotated = append(f.rotated, rotatedRefreshToken{oldHash: oldHash, newHash: newHash, now: now, expiresAt: expiresAt})
	return nil
}

func (f *fakeRefreshTokenRepository) Revoke(_ context.Context, hash string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revoked = append(f.revoked, hash)
	return nil
}

func (f *fakeRefreshTokenRepository) RevokeAllForUser(_ context.Context, userID int64) error {
	if f.revokeAllErr != nil {
		return f.revokeAllErr
	}
	f.revokedUsers = append(f.revokedUsers, userID)
	if f.validUserID == userID {
		f.hasValid = false
	}
	return nil
}

type fakePasswordHasher struct {
	hashValue string
	hashErr   error
	verify    func(password, hash string) bool
}

func (f *fakePasswordHasher) Hash(string) (string, error) {
	if f.hashErr != nil {
		return "", f.hashErr
	}
	return f.hashValue, nil
}

func (f *fakePasswordHasher) Verify(password, hash string) bool {
	return f.verify(password, hash)
}

type fakeAccessTokenIssuer struct {
	valid    map[string]int64
	issueErr error
	parseErr error
	issued   []int64
}

func (f *fakeAccessTokenIssuer) Issue(userID int64) (string, error) {
	if f.issueErr != nil {
		return "", f.issueErr
	}
	f.issued = append(f.issued, userID)
	return fmt.Sprintf("access-%d", userID), nil
}

func (f *fakeAccessTokenIssuer) Parse(raw string) (int64, error) {
	if f.parseErr != nil {
		return 0, f.parseErr
	}
	userID, ok := f.valid[raw]
	if !ok {
		return 0, errors.New("bad token")
	}
	return userID, nil
}

type fakeRefreshTokenService struct {
	sequence    []string
	generateErr error
	generated   int
}

func (f *fakeRefreshTokenService) Generate() (string, error) {
	if f.generateErr != nil {
		return "", f.generateErr
	}
	f.generated++
	if f.generated <= len(f.sequence) {
		return f.sequence[f.generated-1], nil
	}
	return fmt.Sprintf("refresh-%d", f.generated), nil
}

func (f *fakeRefreshTokenService) Hash(raw string) string { return "sha256:" + raw }

type authHarness struct {
	service *AuthService
	users   *fakeUserRepository
	tokens  *fakeRefreshTokenRepository
	hasher  *fakePasswordHasher
	access  *fakeAccessTokenIssuer
	refresh *fakeRefreshTokenService
}

func newAuthHarness(user *domain.User) *authHarness {
	h := &authHarness{
		users:  &fakeUserRepository{users: map[int64]*domain.User{}},
		tokens: &fakeRefreshTokenRepository{},
		hasher: &fakePasswordHasher{
			hashValue: "hashed-by-fake",
			verify:    func(password, hash string) bool { return hash == "hash:"+password },
		},
		access:  &fakeAccessTokenIssuer{valid: map[string]int64{}},
		refresh: &fakeRefreshTokenService{},
	}
	if user != nil {
		h.users.users[user.ID] = user
	}
	h.service = NewAuthService(h.users, h.tokens, h.hasher, h.access, h.refresh, testRefreshTTL)
	return h
}

func activeUser() *domain.User {
	return &domain.User{
		ID:             7,
		FullName:       "Ana Pérez",
		Email:          "ana@example.com",
		HashedPassword: "hash:secret",
		IsActive:       true,
	}
}

func TestLoginSuccess(t *testing.T) {
	user := activeUser()
	h := newAuthHarness(user)
	h.refresh.sequence = []string{"refresh-token"}

	before := time.Now()
	result, err := h.service.Login(context.Background(), user.Email, "secret")
	after := time.Now()
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if result.AccessToken != "access-7" {
		t.Errorf("AccessToken = %q, want access-7", result.AccessToken)
	}
	if result.RefreshToken != "refresh-token" {
		t.Errorf("RefreshToken = %q, want refresh-token", result.RefreshToken)
	}
	if result.User != user {
		t.Errorf("User = %v, want the repository user", result.User)
	}

	if len(h.tokens.created) != 1 {
		t.Fatalf("persisted %d refresh tokens, want 1", len(h.tokens.created))
	}
	stored := h.tokens.created[0]
	if stored.userID != user.ID {
		t.Errorf("persisted user ID = %d, want %d", stored.userID, user.ID)
	}
	if stored.hash != "sha256:refresh-token" {
		t.Errorf("persisted hash = %q, want sha256:refresh-token", stored.hash)
	}
	if stored.expiresAt.Before(before.Add(testRefreshTTL)) {
		t.Errorf("expiresAt = %v, want no earlier than %v", stored.expiresAt, before.Add(testRefreshTTL))
	}
	if stored.expiresAt.After(after.Add(testRefreshTTL)) {
		t.Errorf("expiresAt = %v, want no later than %v", stored.expiresAt, after.Add(testRefreshTTL))
	}
	if !stored.expiresAt.After(after) {
		t.Errorf("expiresAt = %v, want a future time", stored.expiresAt)
	}

	if len(h.access.issued) != 1 || h.access.issued[0] != user.ID {
		t.Errorf("issued access tokens = %v, want [%d]", h.access.issued, user.ID)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h := newAuthHarness(activeUser())

	_, err := h.service.Login(context.Background(), "ana@example.com", "wrong")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if len(h.tokens.created) != 0 {
		t.Errorf("persisted %d refresh tokens, want 0", len(h.tokens.created))
	}
	if len(h.access.issued) != 0 {
		t.Errorf("issued %d access tokens, want 0", len(h.access.issued))
	}
}

func TestLoginInactiveUser(t *testing.T) {
	user := activeUser()
	user.IsActive = false
	h := newAuthHarness(user)

	_, err := h.service.Login(context.Background(), user.Email, "secret")
	if !errors.Is(err, domain.ErrInactiveUser) {
		t.Fatalf("Login() error = %v, want ErrInactiveUser", err)
	}
	if len(h.tokens.created) != 0 {
		t.Errorf("persisted %d refresh tokens, want 0", len(h.tokens.created))
	}
}

func TestRefreshSuccessRotatesToken(t *testing.T) {
	user := activeUser()
	h := newAuthHarness(user)
	h.tokens.validUserID = user.ID
	h.tokens.hasValid = true
	h.refresh.sequence = []string{"new-token"}

	pair, err := h.service.Refresh(context.Background(), "old-token")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if pair.AccessToken != "access-7" {
		t.Errorf("AccessToken = %q, want access-7", pair.AccessToken)
	}
	if pair.RefreshToken != "new-token" {
		t.Errorf("RefreshToken = %q, want new-token", pair.RefreshToken)
	}

	if h.tokens.findHash != "sha256:old-token" {
		t.Errorf("FindValid hash = %q, want sha256:old-token", h.tokens.findHash)
	}
	if h.tokens.findNow.IsZero() {
		t.Error("FindValid received a zero time")
	}
	if len(h.tokens.rotated) != 1 {
		t.Fatalf("rotated %d tokens, want 1", len(h.tokens.rotated))
	}
	rotated := h.tokens.rotated[0]
	if rotated.oldHash != "sha256:old-token" {
		t.Errorf("rotated old hash = %q, want sha256:old-token", rotated.oldHash)
	}
	if rotated.newHash != "sha256:new-token" {
		t.Errorf("rotated new hash = %q, want sha256:new-token", rotated.newHash)
	}
	if rotated.now.IsZero() {
		t.Error("Rotate received a zero now")
	}
	if want := rotated.now.Add(testRefreshTTL); !rotated.expiresAt.Equal(want) {
		t.Errorf("rotation expiresAt = %v, want %v", rotated.expiresAt, want)
	}
}

func TestRefreshInvalidToken(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*fakeRefreshTokenRepository)
	}{
		{
			name:    "unknown token",
			prepare: func(*fakeRefreshTokenRepository) {},
		},
		{
			name: "expired token",
			prepare: func(tokens *fakeRefreshTokenRepository) {
				tokens.findErr = domain.ErrNotFound
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAuthHarness(activeUser())
			tt.prepare(h.tokens)

			_, err := h.service.Refresh(context.Background(), "old-token")
			if !errors.Is(err, domain.ErrInvalidRefresh) {
				t.Fatalf("Refresh() error = %v, want ErrInvalidRefresh", err)
			}
			if len(h.tokens.rotated) != 0 {
				t.Errorf("rotated %d tokens, want 0", len(h.tokens.rotated))
			}
		})
	}
}

func TestRefreshInactiveUser(t *testing.T) {
	user := activeUser()
	user.IsActive = false
	h := newAuthHarness(user)
	h.tokens.validUserID = user.ID
	h.tokens.hasValid = true

	_, err := h.service.Refresh(context.Background(), "old-token")
	if !errors.Is(err, domain.ErrInvalidRefresh) {
		t.Fatalf("Refresh() error = %v, want ErrInvalidRefresh", err)
	}
	if len(h.tokens.rotated) != 0 {
		t.Errorf("rotated %d tokens, want 0", len(h.tokens.rotated))
	}
	if len(h.access.issued) != 0 {
		t.Errorf("issued %d access tokens, want 0", len(h.access.issued))
	}
}

func TestPasswordResetRevokesRefreshToken(t *testing.T) {
	user := activeUser()
	h := newAuthHarness(user)
	h.tokens.validUserID = user.ID
	h.tokens.hasValid = true
	users := NewUserService(h.users, h.tokens, h.hasher)

	password := "newsecret1"
	if _, err := users.Update(context.Background(), user.ID, inbound.UpdateUserInput{Password: &password}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(h.tokens.revokedUsers) != 1 || h.tokens.revokedUsers[0] != user.ID {
		t.Fatalf("revoked users = %v, want [%d]", h.tokens.revokedUsers, user.ID)
	}

	if _, err := h.service.Refresh(context.Background(), "old-token"); !errors.Is(err, domain.ErrInvalidRefresh) {
		t.Fatalf("Refresh() after reset error = %v, want ErrInvalidRefresh", err)
	}
	if len(h.tokens.rotated) != 0 {
		t.Errorf("rotated %d tokens, want 0", len(h.tokens.rotated))
	}
}

func TestLogoutRevokesHashedToken(t *testing.T) {
	h := newAuthHarness(activeUser())

	if err := h.service.Logout(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if len(h.tokens.revoked) != 1 || h.tokens.revoked[0] != "sha256:raw-token" {
		t.Errorf("revoked = %v, want [sha256:raw-token]", h.tokens.revoked)
	}
}

func TestLogoutEmptyTokenIsNoOp(t *testing.T) {
	h := newAuthHarness(activeUser())

	if err := h.service.Logout(context.Background(), ""); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if len(h.tokens.revoked) != 0 {
		t.Errorf("revoked %d tokens, want 0", len(h.tokens.revoked))
	}
}

func TestAuthenticateAccessToken(t *testing.T) {
	user := activeUser()
	h := newAuthHarness(user)
	h.access.valid["access-good"] = user.ID

	got, err := h.service.AuthenticateAccessToken(context.Background(), "access-good")
	if err != nil {
		t.Fatalf("AuthenticateAccessToken() error = %v", err)
	}
	if got != user {
		t.Errorf("user = %v, want the repository user", got)
	}
}

func TestAuthenticateAccessTokenRejectsBadToken(t *testing.T) {
	h := newAuthHarness(activeUser())

	_, err := h.service.AuthenticateAccessToken(context.Background(), "access-bad")
	if !errors.Is(err, domain.ErrInvalidAccess) {
		t.Fatalf("AuthenticateAccessToken() error = %v, want ErrInvalidAccess", err)
	}
}

func TestAuthenticateAccessTokenRejectsInactiveUser(t *testing.T) {
	user := activeUser()
	user.IsActive = false
	h := newAuthHarness(user)
	h.access.valid["access-inactive"] = user.ID

	_, err := h.service.AuthenticateAccessToken(context.Background(), "access-inactive")
	if !errors.Is(err, domain.ErrInvalidAccess) {
		t.Fatalf("AuthenticateAccessToken() error = %v, want ErrInvalidAccess", err)
	}
}

var (
	_ outbound.UserRepository         = (*fakeUserRepository)(nil)
	_ outbound.RefreshTokenRepository = (*fakeRefreshTokenRepository)(nil)
	_ outbound.PasswordHasher         = (*fakePasswordHasher)(nil)
	_ outbound.AccessTokenIssuer      = (*fakeAccessTokenIssuer)(nil)
	_ outbound.RefreshTokenService    = (*fakeRefreshTokenService)(nil)
)
