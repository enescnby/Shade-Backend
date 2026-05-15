package repositories

import (
	"core-backend/internal/models"
	"core-backend/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuditRepository interface {
	LogEvent(audit *models.SecurityAuditLog) error
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
