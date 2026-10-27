package security

import (
	"golang.org/x/crypto/bcrypt"

	"inventory-desktop/internal/ports/outbound"
)

var _ outbound.PasswordHasher = (*PasswordHasher)(nil)

type PasswordHasher struct {
	rounds int
}

func NewPasswordHasher(rounds int) *PasswordHasher {
	return &PasswordHasher{rounds: rounds}
}

func (h *PasswordHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.rounds)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (h *PasswordHasher) Verify(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
