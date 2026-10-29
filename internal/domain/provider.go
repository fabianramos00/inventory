package domain

type Provider struct {
	ID          int64
	Name        string
	ContactInfo *string
	Email       *string
	Phone       *string
	CreatedByID *int64
}
