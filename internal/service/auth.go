package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"inventory-desktop/internal/auth"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrInvalidAccess      = errors.New("invalid access token")
)

type LoginResult struct {
	AccessToken  string
	RefreshToken string
	User         *domain.User
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type AuthService struct {
	users      ports.UserRepository
	tokens     ports.RefreshTokenRepository
	jwt        *auth.JWTManager
	refreshTTL time.Duration
}

func NewAuthService(users ports.UserRepository, tokens ports.RefreshTokenRepository, secret, backend string, accessTTL, refreshTTL time.Duration) *AuthService {
	return &AuthService{
		users:      users,
		tokens:     tokens,
		jwt:        auth.NewJWTManager(secret, accessTTL, "inventory-desktop/"+backend),
		refreshTTL: refreshTTL,
	}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	user, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(password, user.HashedPassword) {
		return nil, ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, ErrInactiveUser
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	expiresAt := time.Now().Add(s.refreshTTL)
	if err := s.tokens.Create(ctx, user.ID, auth.HashRefreshToken(refreshToken), expiresAt); err != nil {
		return nil, err
	}

	accessToken, err := s.jwt.Create(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}
	return &LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, User: user}, nil
}

func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*TokenPair, error) {
	if rawToken == "" {
		return nil, ErrInvalidRefresh
	}

	oldHash := auth.HashRefreshToken(rawToken)
	stored, err := s.tokens.FindValid(ctx, oldHash, time.Now())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, ErrInvalidRefresh
	}
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetByID(ctx, stored.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, ErrInvalidRefresh
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInvalidRefresh
	}

	newToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	now := time.Now()
	if err := s.tokens.Rotate(ctx, oldHash, auth.HashRefreshToken(newToken), now, now.Add(s.refreshTTL)); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrInvalidRefresh
		}
		return nil, err
	}

	accessToken, err := s.jwt.Create(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: newToken}, nil
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.tokens.Revoke(ctx, auth.HashRefreshToken(rawToken))
}

func (s *AuthService) AuthenticateAccessToken(ctx context.Context, rawToken string) (*domain.User, error) {
	userID, err := s.jwt.Parse(rawToken)
	if err != nil {
		return nil, ErrInvalidAccess
	}
	user, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, ErrInvalidAccess
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInvalidAccess
	}
	return user, nil
}
