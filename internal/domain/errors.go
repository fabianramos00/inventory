package domain

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrForeignKeyViolation = errors.New("foreign key violation")
	ErrValidation          = errors.New("validation error")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInactiveUser        = errors.New("inactive user")
	ErrInvalidRefresh      = errors.New("invalid refresh token")
	ErrInvalidAccess       = errors.New("invalid access token")
)
