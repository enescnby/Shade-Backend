package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	UserID      uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CoreGuardID string    `gorm:"type:varchar;not null;unique"`
	LastLoginAt time.Time
	CreatedAt   time.Time

	Key    UserKey    `gorm:"foreignKey:UserID"`
	Device UserDevice `gorm:"foreignKey:UserID"`
}

type UserKey struct {
	KeyID                         int       `gorm:"primaryKey;autoIncrement"`
	UserID                        uuid.UUID `gorm:"type:uuid;index;not null"`
	IdentityPublicKey             string    `gorm:"type:varchar;not null"`
	EncryptedIdentityPrivateKey   string    `gorm:"type:varchar;not null"`
	EncryptionPublicKey           string    `gorm:"type:varchar;not null"`
	EncryptedEncryptionPrivateKey string    `gorm:"type:varchar;not null"`
	Salt                          string    `gorm:"type:varchar;not null"`
	CreatedAt                     time.Time `gorm:"autoCreateTime"`
}

type UserDevice struct {
	DeviceID    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID      uuid.UUID `gorm:"type:uuid;index;not null"`
	FCMToken    string    `gorm:"type:varchar"`
	DeviceModel string    `gorm:"type:varchar"`
	LastActive  time.Time
}
