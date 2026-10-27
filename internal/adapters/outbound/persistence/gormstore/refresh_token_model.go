package gormstore

type refreshTokenModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    int64     `gorm:"column:user_id"`
	TokenHash string    `gorm:"column:token_hash"`
	ExpiresAt Timestamp `gorm:"column:expires_at"`
	Revoked   bool      `gorm:"column:revoked"`
}

func (refreshTokenModel) TableName() string {
	return "refresh_tokens"
}
