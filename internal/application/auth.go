package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

var _ inbound.AuthUseCase = (*AuthService)(nil)

type AuthService struct {
	users         outbound.UserRepository
	tokens        outbound.RefreshTokenRepository
	passwords     outbound.PasswordHasher
	accessTokens  outbound.AccessTokenIssuer
	refreshTokens outbound.RefreshTokenService
	refreshTTL    time.Duration
}

func NewAuthService(
	users outbound.UserRepository,
	tokens outbound.RefreshTokenRepository,
	passwords outbound.PasswordHasher,
	accessTokens outbound.AccessTokenIssuer,
	refreshTokens outbound.RefreshTokenService,
	refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		users:         users,
		tokens:        tokens,
		passwords:     passwords,
		accessTokens:  accessTokens,
		refreshTokens: refreshTokens,
		refreshTTL:    refreshTTL,
	}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*inbound.LoginResult, error) {
	user, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !s.passwords.Verify(password, user.HashedPassword) {
		return nil, domain.ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, domain.ErrInactiveUser
	}

	refreshToken, err := s.refreshTokens.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	expiresAt := time.Now().Add(s.refreshTTL)
	if err := s.tokens.Create(ctx, user.ID, s.refreshTokens.Hash(refreshToken), expiresAt); err != nil {
		return nil, err
	}

	accessToken, err := s.accessTokens.Issue(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}
	return &inbound.LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, User: user}, nil
}

func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*inbound.TokenPair, error) {
	if rawToken == "" {
		return nil, domain.ErrInvalidRefresh
	}

	oldHash := s.refreshTokens.Hash(rawToken)
	userID, err := s.tokens.FindValid(ctx, oldHash, time.Now())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidRefresh
	}
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidRefresh
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrInvalidRefresh
	}

	newToken, err := s.refreshTokens.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	now := time.Now()
	if err := s.tokens.Rotate(ctx, oldHash, s.refreshTokens.Hash(newToken), now, now.Add(s.refreshTTL)); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidRefresh
		}
		return nil, err
	}

	accessToken, err := s.accessTokens.Issue(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}
	return &inbound.TokenPair{AccessToken: accessToken, RefreshToken: newToken}, nil
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.tokens.Revoke(ctx, s.refreshTokens.Hash(rawToken))
}

func (s *AuthService) AuthenticateAccessToken(ctx context.Context, rawToken string) (*domain.User, error) {
	userID, err := s.accessTokens.Parse(rawToken)
	if err != nil {
		return nil, domain.ErrInvalidAccess
	}
	user, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidAccess
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrInvalidAccess
	}
	return user, nil
}
