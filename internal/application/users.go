package application

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

var _ inbound.UserUseCase = (*UserService)(nil)

type UserService struct {
	users     outbound.UserRepository
	passwords outbound.PasswordHasher
}

func NewUserService(users outbound.UserRepository, passwords outbound.PasswordHasher) *UserService {
	return &UserService{users: users, passwords: passwords}
}

func (s *UserService) List(ctx context.Context, skip, limit int, search string) (*inbound.UserList, error) {
	items, hasNext, err := s.users.List(ctx, skip, limit, strings.TrimSpace(search))
	if err != nil {
		return nil, err
	}
	return &inbound.UserList{Items: items, HasNext: hasNext, Skip: skip, Limit: limit}, nil
}

func (s *UserService) Get(ctx context.Context, id int64) (*domain.User, error) {
	return s.users.GetByID(ctx, id)
}

func (s *UserService) Create(ctx context.Context, input inbound.CreateUserInput) (*domain.User, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}
	hash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	isSuperuser := false
	if input.IsSuperuser != nil {
		isSuperuser = *input.IsSuperuser
	}
	user := &domain.User{
		Email:          email,
		FullName:       strings.TrimSpace(input.FullName),
		HashedPassword: hash,
		IsActive:       isActive,
		IsSuperuser:    isSuperuser,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Update(ctx context.Context, id int64, input inbound.UpdateUserInput) (*domain.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Email != nil {
		email, err := normalizeEmail(*input.Email)
		if err != nil {
			return nil, err
		}
		user.Email = email
	}
	if input.Password != nil && *input.Password != "" {
		if err := validatePassword(*input.Password); err != nil {
			return nil, err
		}
		hash, err := s.passwords.Hash(*input.Password)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		user.HashedPassword = hash
	}
	if input.FullName != nil {
		user.FullName = strings.TrimSpace(*input.FullName)
	}
	if input.IsActive != nil {
		user.IsActive = *input.IsActive
	}
	if input.IsSuperuser != nil {
		user.IsSuperuser = *input.IsSuperuser
	}
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Delete(ctx context.Context, id int64) (*domain.User, error) {
	return s.users.Delete(ctx, id)
}

func normalizeEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", fmt.Errorf("%w: invalid email", domain.ErrValidation)
	}
	return email, nil
}

func validatePassword(password string) error {
	characters := utf8.RuneCountInString(password)
	if characters < 8 || characters > 128 {
		return fmt.Errorf("%w: password must be 8-128 characters", domain.ErrValidation)
	}
	if len(password) > 72 {
		return fmt.Errorf("%w: password must be at most 72 bytes", domain.ErrValidation)
	}
	return nil
}
