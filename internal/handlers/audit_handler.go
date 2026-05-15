package handlers

import (
	"core-backend/internal/repositories"
	"core-backend/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AuditHandler struct {
	auditRepo repositories.AuditRepository
}

func NewAuditHandler(auditRepo repositories.AuditRepository) *AuditHandler {
	return &AuditHandler{auditRepo: auditRepo}
}

func (h *AuditHandler) GetMyLogs(c *fiber.Ctx) error {
	rawID := c.Locals("user_id")
	if rawID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userID, err := uuid.Parse(rawID.(string))
	if err != nil {
		logger.Log.Error("invalid user_id in context", zap.Any("raw", rawID))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	logs, err := h.auditRepo.GetByUser(c.UserContext(), userID, 50)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not fetch logs"})
	}

	type AuditLogItem struct {
		ActionType string `json:"action_type"`
		IPAddress  string `json:"ip_address"`
		Timestamp  string `json:"timestamp"`
	}

	result := make([]AuditLogItem, 0, len(logs))
	for _, log := range logs {
		result = append(result, AuditLogItem{
			ActionType: log.ActionType,
			IPAddress:  log.IPAddress,
			Timestamp:  log.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"logs": result})
}
