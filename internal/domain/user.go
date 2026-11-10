package domain

type User struct {
	ID             int64
	FullName       string
	Email          string
	HashedPassword string
	IsActive       bool
	IsSuperuser    bool
}

// UserOption is the minimal user projection used by pickers.
type UserOption struct {
	ID       int64
	FullName string
}
