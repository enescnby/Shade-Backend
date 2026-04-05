package repositories

import (
	"context"
	"core-backend/internal/models"
	"core-backend/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuditRepository interface {
	LogEvent(audit *models.SecurityAuditLog) error
	GetByUser(ctx context.Context, userID uuid.UUID, limit int) ([]models.SecurityAuditLog, error)
}

type auditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) LogEvent(audit *models.SecurityAuditLog) error {
	if err := r.db.Create(audit).Error; err != nil {
		logger.Log.Error("failed to save security audit log", zap.Error(err))
		return err
	}
	return nil
}

func (r *auditRepository) GetByUser(ctx context.Context, userID uuid.UUID, limit int) ([]models.SecurityAuditLog, error) {
	var logs []models.SecurityAuditLog
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		logger.Log.Error("failed to fetch audit logs", zap.Error(err))
		return nil, err
	}
	return logs, nil
}
