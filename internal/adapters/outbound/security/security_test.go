package security

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAccessTokenIssuerRoundTrip(t *testing.T) {
	issuer := NewAccessTokenIssuer("secret", time.Minute, "inventory-desktop/sqlite")
	token, err := issuer.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	id, err := issuer.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("got user id %d, want 42", id)
	}
	if _, err := NewAccessTokenIssuer("other", time.Minute, "inventory-desktop/sqlite").Parse(token); err == nil {
		t.Fatal("expected signature validation to fail")
	}
}

func TestAccessTokenIssuerRejectsExpiredToken(t *testing.T) {
	token, err := NewAccessTokenIssuer("secret", -time.Minute, "inventory-desktop/sqlite").Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAccessTokenIssuer("secret", time.Minute, "inventory-desktop/sqlite").Parse(token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestAccessTokenIssuerRejectsTokenWithoutExpiration(t *testing.T) {
	issuer := NewAccessTokenIssuer("secret", time.Minute, "inventory-desktop/sqlite")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:  "inventory-desktop/sqlite",
		Subject: "7",
	})
	raw, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := issuer.Parse(raw); err == nil {
		t.Fatal("expected a token without exp to be rejected")
	}
}

func TestAccessTokenIssuerBindsIssuerAndTokenID(t *testing.T) {
	issuer := NewAccessTokenIssuer("secret", time.Minute, "inventory-desktop/sqlite")
	token, err := issuer.Issue(7)
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

	other := NewAccessTokenIssuer("secret", time.Minute, "inventory-desktop/postgres")
	if _, err := other.Parse(token); err == nil {
		t.Fatal("expected a token from another backend issuer to be rejected")
	}

	second, err := issuer.Issue(7)
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
	service := NewRefreshTokenService()
	first, err := service.Generate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("refresh tokens must be unique")
	}
	hash := service.Hash(first)
	if len(hash) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hash))
	}
	if strings.Contains(hash, first) {
		t.Fatal("hash must not contain the raw token")
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hasher := NewPasswordHasher(4)
	hash, err := hasher.Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !hasher.Verify("correct horse", hash) {
		t.Fatal("expected password to verify")
	}
	if hasher.Verify("wrong horse", hash) {
		t.Fatal("expected wrong password to fail")
	}
	if strings.Contains(hash, "correct horse") {
		t.Fatal("hash must not contain the password")
	}
}
