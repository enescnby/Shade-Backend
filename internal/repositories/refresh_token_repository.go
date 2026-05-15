package repositories

import (
	"core-backend/internal/models"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RefreshTokenRepository interface {
	// Upsert: replace any existing token for this (userID, deviceID) pair.
	Save(rt *models.RefreshToken) error
	// FindByHash returns the token row for a given SHA-256 hash.
	FindByHash(hash string) (*models.RefreshToken, error)
	// Delete removes the row (called after rotation or logout).
	Delete(id int) error
	// DeleteByUser removes all refresh tokens for a user (e.g. on logout-all).
	DeleteByUser(userID uuid.UUID) error
}

type refreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

func (r *refreshTokenRepository) Save(rt *models.RefreshToken) error {
	// Delete any previous token for this device so each device holds at most one token.
	r.db.Where("user_id = ? AND device_id = ?", rt.UserID, rt.DeviceID).
		Delete(&models.RefreshToken{})
	return r.db.Create(rt).Error
}

func (r *refreshTokenRepository) FindByHash(hash string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := r.db.Where("token_hash = ?", hash).First(&rt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("refresh token not found")
		}
		return nil, err
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, errors.New("refresh token expired")
	}
	return &rt, nil
}

func (r *refreshTokenRepository) Delete(id int) error {
	return r.db.Delete(&models.RefreshToken{}, id).Error
}

func (r *refreshTokenRepository) DeleteByUser(userID uuid.UUID) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.RefreshToken{}).Error
}
