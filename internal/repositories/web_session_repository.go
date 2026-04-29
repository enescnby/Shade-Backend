package repositories

import (
	"core-backend/internal/models"
	"core-backend/pkg/logger"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type WebSessionRepository interface {
	Create(session *models.WebSession) error
	GetByID(id uuid.UUID) (*models.WebSession, error)
	Authorize(id uuid.UUID, ciphertext, nonce, androidPub string) error
	DeleteExpired() error
}

type webSessionRepository struct {
	db *gorm.DB
}

func NewWebSessionRepository(db *gorm.DB) WebSessionRepository {
	return &webSessionRepository{db: db}
}

func (r *webSessionRepository) Create(session *models.WebSession) error {
	if err := r.db.Create(session).Error; err != nil {
		logger.Log.Error("failed to create web session", zap.Error(err))
		return err
	}
	return nil
}

func (r *webSessionRepository) GetByID(id uuid.UUID) (*models.WebSession, error) {
	var session models.WebSession
	err := r.db.Where("session_id = ?", id).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		logger.Log.Error("failed to fetch web session", zap.Error(err))
		return nil, err
	}
	return &session, nil
}

func (r *webSessionRepository) Authorize(id uuid.UUID, ciphertext, nonce, androidPub string) error {
	now := time.Now()
	result := r.db.Model(&models.WebSession{}).
		Where("session_id = ? AND status = ?", id, "pending").
		Updates(map[string]interface{}{
			"status":             "authorized",
			"ciphertext":         ciphertext,
			"nonce":              nonce,
			"android_x25519_pub": androidPub,
			"authorized_at":      &now,
		})
	if result.Error != nil {
		logger.Log.Error("failed to authorize web session", zap.Error(result.Error))
		return result.Error
	}
	return nil
}

func (r *webSessionRepository) DeleteExpired() error {
	return r.db.Where("expires_at < ?",
		time.Now()).Delete(&models.WebSession{}).Error
}
