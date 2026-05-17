package repositories

import (
	"core-backend/internal/models"
	"core-backend/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuditRepository interface {
	LogEvent(audit *models.SecurityAuditLog) error
	ListByUserID(userID uuid.UUID, limit int) ([]models.SecurityAuditLog, error)
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

func (r *auditRepository) ListByUserID(userID uuid.UUID, limit int) ([]models.SecurityAuditLog, error) {
	var rows []models.SecurityAuditLog
	err := r.db.
		Where("user_id = ?", userID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		logger.Log.Error("failed to list security audit logs", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, err
	}
	return rows, nil
}
