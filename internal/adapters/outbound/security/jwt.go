package security

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"inventory-desktop/internal/ports/outbound"
)

var ErrInvalidToken = errors.New("invalid token")

var _ outbound.AccessTokenIssuer = (*AccessTokenIssuer)(nil)

type AccessTokenIssuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

func NewAccessTokenIssuer(secret string, ttl time.Duration, issuer string) *AccessTokenIssuer {
	return &AccessTokenIssuer{secret: []byte(secret), ttl: ttl, issuer: issuer}
}

func (i *AccessTokenIssuer) Issue(userID int64) (string, error) {
	id, err := newTokenID()
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    i.issuer,
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        id,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(i.secret)
}

func (i *AccessTokenIssuer) Parse(raw string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return i.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}

func newTokenID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
