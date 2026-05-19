package services

import (
	"core-backend/internal/dto"
	"core-backend/internal/repositories"
	"core-backend/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const auditLogsHardLimit = 50

// isoMillisLayout produces RFC3339-compatible timestamps with millisecond
// precision and a "Z" suffix for UTC values, e.g. "2026-05-03T12:01:00.000Z".
// This is fully parseable by java.time.Instant.parse on the mobile side.
const isoMillisLayout = "2006-01-02T15:04:05.000Z07:00"

type AuditService interface {
	GetMyLogs(userID uuid.UUID) (*dto.AuditLogsResponse, error)
}

type auditService struct {
	auditRepo repositories.AuditRepository
}

func NewAuditService(auditRepo repositories.AuditRepository) AuditService {
	return &auditService{auditRepo: auditRepo}
}

func (s *auditService) GetMyLogs(userID uuid.UUID) (*dto.AuditLogsResponse, error) {
	rows, err := s.auditRepo.ListByUserID(userID, auditLogsHardLimit)
	if err != nil {
		logger.Log.Error("failed to load audit logs for user",
			zap.Error(err), zap.String("user_id", userID.String()))
		return nil, err
	}

	entries := make([]dto.AuditLogEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, dto.AuditLogEntry{
			ActionType: row.ActionType,
			IPAddress:  row.IPAddress,
			Timestamp:  row.Timestamp.UTC().Format(isoMillisLayout),
		})
	}

	return &dto.AuditLogsResponse{Logs: entries}, nil
}
