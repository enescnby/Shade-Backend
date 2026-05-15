package models

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken stores a SHA-256 hash of the issued refresh token.
// One row per active device session; old rows for the same device are replaced on re-login.
type RefreshToken struct {
	ID        int       `gorm:"primaryKey;autoIncrement"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"type:varchar(64);not null;unique;index"` // hex(SHA-256)
	DeviceID  int       `gorm:"not null;index"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}
