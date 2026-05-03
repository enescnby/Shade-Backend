package models

import (
	"time"

	"github.com/google/uuid"
)

type WebSession struct {
	SessionID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID           uuid.UUID `gorm:"type:uuid"`
	WebDeviceID      uuid.UUID `gorm:"type:uuid"`
	Status           string    `gorm:"type:varchar(16);not null;default:'pending'"`
	Ciphertext       string    `gorm:"type:text"`
	Nonce            string    `gorm:"type:varchar(64)"`
	AndroidX25519Pub string    `gorm:"type:varchar(128)"`
	ExpiresAt        time.Time `gorm:"not null"`
	AuthorizedAt     *time.Time
	CreatedAt        time.Time `gorm:"autoCreateTime"`
}
