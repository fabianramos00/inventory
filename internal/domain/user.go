package domain

type User struct {
	ID             int64
	FullName       string
	Email          string
	HashedPassword string
	IsActive       bool
	IsSuperuser    bool
}
