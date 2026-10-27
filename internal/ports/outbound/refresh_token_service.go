package outbound

// RefreshTokenService is the outbound port for creating and hashing refresh tokens.
type RefreshTokenService interface {
	Generate() (string, error)
	Hash(raw string) string
}
