package domain

type Category struct {
	ID          int64
	Name        string
	Code        *string
	Description *string
}

type Brand struct {
	ID      int64
	Name    string
	LogoURL *string
}

type Material struct {
	ID   int64
	Name string
}

type MeasurementUnit struct {
	ID           int64
	Name         string
	Abbreviation string
}
