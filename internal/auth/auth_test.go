package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTManagerRoundTrip(t *testing.T) {
	manager := NewJWTManager("secret", time.Minute, "inventory-desktop/sqlite")
	token, err := manager.Create(42)
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("got user id %d, want 42", id)
	}
	if _, err := NewJWTManager("other", time.Minute, "inventory-desktop/sqlite").Parse(token); err == nil {
		t.Fatal("expected signature validation to fail")
	}
}

func TestJWTManagerRejectsExpiredToken(t *testing.T) {
	token, err := NewJWTManager("secret", -time.Minute, "inventory-desktop/sqlite").Create(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJWTManager("secret", time.Minute, "inventory-desktop/sqlite").Parse(token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestJWTManagerBindsIssuerAndTokenID(t *testing.T) {
	manager := NewJWTManager("secret", time.Minute, "inventory-desktop/sqlite")
	token, err := manager.Create(7)
	if err != nil {
		t.Fatal(err)
	}

	claims := &jwt.RegisteredClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		t.Fatal(err)
	}
	if claims.Issuer != "inventory-desktop/sqlite" {
		t.Fatalf("issuer = %q", claims.Issuer)
	}
	if claims.IssuedAt == nil {
		t.Fatal("issued-at claim is missing")
	}
	if claims.ID == "" {
		t.Fatal("token id claim is missing")
	}

	other := NewJWTManager("secret", time.Minute, "inventory-desktop/postgres")
	if _, err := other.Parse(token); err == nil {
		t.Fatal("expected a token from another backend issuer to be rejected")
	}

	second, err := manager.Create(7)
	if err != nil {
		t.Fatal(err)
	}
	secondClaims := &jwt.RegisteredClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(second, secondClaims); err != nil {
		t.Fatal(err)
	}
	if secondClaims.ID == claims.ID {
		t.Fatal("token ids must be unique")
	}
}

func TestRefreshTokensAreRandomAndHashed(t *testing.T) {
	first, err := GenerateRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("refresh tokens must be unique")
	}
	hash := HashRefreshToken(first)
	if len(hash) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hash))
	}
	if strings.Contains(hash, first) {
		t.Fatal("hash must not contain the raw token")
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong horse", hash) {
		t.Fatal("expected wrong password to fail")
	}
	if strings.Contains(hash, "correct horse") {
		t.Fatal("hash must not contain the password")
	}
}
