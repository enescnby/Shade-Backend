package handlers

import (
	"core-backend/internal/services"
	"core-backend/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AuditHandler struct {
	auditService services.AuditService
}

func NewAuditHandler(s services.AuditService) *AuditHandler {
	return &AuditHandler{auditService: s}
}

// GetMyLogs returns the last 50 security-audit events for the authenticated
// user, newest first. The user is identified by the JWT (resolved by the
// Protected middleware into c.Locals("user_id")).
func (h *AuditHandler) GetMyLogs(c *fiber.Ctx) error {
	rawUserID, _ := c.Locals("user_id").(string)
	if rawUserID == "" {
		logger.Log.Warn("audit/me reached without user_id in context")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid or expired token",
		})
	}

	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		logger.Log.Warn("audit/me received unparseable user_id", zap.Error(err), zap.String("user_id", rawUserID))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid or expired token",
		})
	}

	res, err := h.auditService.GetMyLogs(userID)
	if err != nil {
		logger.Log.Error("failed to load audit logs", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "could not load audit logs",
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}
