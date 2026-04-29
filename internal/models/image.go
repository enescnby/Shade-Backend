package models

import (
	"time"

	"github.com/google/uuid"
)

type ImageMetaData struct {
	ImageID    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UploaderID uuid.UUID `gorm:"type:uuid;index;not null"`
	Size       int64     `gorm:"type:bigint;not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}
