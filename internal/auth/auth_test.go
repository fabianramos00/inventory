package auth

import (
	"strings"
	"testing"
	"time"
)

func TestJWTManagerRoundTrip(t *testing.T) {
	manager := NewJWTManager("secret", time.Minute)
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
	if _, err := NewJWTManager("other", time.Minute).Parse(token); err == nil {
		t.Fatal("expected signature validation to fail")
	}
}

func TestJWTManagerRejectsExpiredToken(t *testing.T) {
	token, err := NewJWTManager("secret", -time.Minute).Create(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJWTManager("secret", time.Minute).Parse(token); err == nil {
		t.Fatal("expected expired token to be rejected")
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
