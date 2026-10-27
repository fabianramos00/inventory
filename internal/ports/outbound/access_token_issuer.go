package outbound

// AccessTokenIssuer is the outbound port for issuing and parsing access tokens.
type AccessTokenIssuer interface {
	Issue(userID int64) (string, error)
	Parse(raw string) (int64, error)
}
