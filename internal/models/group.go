package models

import (
	"time"

	"github.com/google/uuid"
)

// Group represents a multi-party chat room.
type Group struct {
	GroupID     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Name        string    `gorm:"type:varchar(128);not null"`
	OwnerID     uuid.UUID `gorm:"type:uuid;not null;index"`
	AvatarURL   string    `gorm:"type:varchar"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`

	Members []GroupMember `gorm:"foreignKey:GroupID"`
}

// GroupMember is a join table between Group and User.
type GroupMember struct {
	ID       int       `gorm:"primaryKey;autoIncrement"`
	GroupID  uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID   uuid.UUID `gorm:"type:uuid;not null;index"`
	ShadeID  string    `gorm:"type:varchar;not null"`
	Role     string    `gorm:"type:varchar(16);not null;default:'member'"` // "owner" | "member"
	JoinedAt time.Time `gorm:"autoCreateTime"`
}

// ContactInvite holds a one-time or multi-use invite code for adding contacts / joining groups.
type ContactInvite struct {
	InviteID  int       `gorm:"primaryKey;autoIncrement"`
	Code      string    `gorm:"type:varchar(32);not null;unique;index"`
	OwnerID   uuid.UUID `gorm:"type:uuid;not null;index"`
	GroupID   *uuid.UUID `gorm:"type:uuid;index"` // nil = personal contact invite
	MaxUses   int       `gorm:"not null;default:1"`
	UseCount  int       `gorm:"not null;default:0"`
	ExpiresAt *time.Time
	CreatedAt time.Time `gorm:"autoCreateTime"`
}
