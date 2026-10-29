package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrForeignKeyViolation = errors.New("foreign key violation")
	ErrValidation          = errors.New("validation error")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInactiveUser        = errors.New("inactive user")
	ErrInvalidRefresh      = errors.New("invalid refresh token")
	ErrInvalidAccess       = errors.New("invalid access token")
)

type NotFoundError struct {
	Entity string
	ID     int64
}

func (e *NotFoundError) Error() string {
	switch e.Entity {
	case "Sale":
		return "Sale not found"
	case "Sale item":
		return "Sale item not found"
	case "Purchase order":
		return "Purchase order not found"
	case "Order item":
		return "Order item not found"
	default:
		return fmt.Sprintf("%s with id %d not found", e.Entity, e.ID)
	}
}

// ProviderNotFoundError keeps the legacy 400 response for a missing provider
// reference on purchase order create/update.
type ProviderNotFoundError struct {
	ID int64
}

func (e *ProviderNotFoundError) Error() string {
	return fmt.Sprintf("Provider with id %d not found", e.ID)
}

type InsufficientStockError struct {
	SKU       string
	Available int
	Requested int
}

func (e *InsufficientStockError) Error() string {
	return fmt.Sprintf(
		"Insufficient stock for product %s. Available: %d, Requested: %d",
		e.SKU, e.Available, e.Requested,
	)
}

func (e *InsufficientStockError) Unwrap() error {
	return ErrInsufficientStock
}
