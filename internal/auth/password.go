package auth

import (
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string, rounds int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), rounds)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
