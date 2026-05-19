package handlers

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/services"
	"core-backend/pkg/logger"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type MessageHandler struct {
	msgSvc services.MessageService
}

func NewMessageHandler(msgSvc services.MessageService) *MessageHandler {
	return &MessageHandler{msgSvc: msgSvc}
}

func (h *MessageHandler) GetInbox(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)
	userID = strings.TrimSpace(userID)

	deviceID, _ := c.Locals("device_id").(string)
	deviceID = strings.TrimSpace(deviceID)

	if userID == "" || deviceID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			limit = v
		}
	}

	response, err := h.msgSvc.DrainInbox(userID, deviceID, limit)
	if err != nil {
		logger.Log.Error("inbox drain failed",
			zap.String("user_id", userID), zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// PostReceipts — WebSocket gönderilemediğinde Android'in kullandığı REST fallback.
// Makbuzları orijinal mesaj sahiplerine RabbitMQ üzerinden iletir.
func (h *MessageHandler) PostReceipts(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)
	shadeID, _ := c.Locals("core_guard_id").(string)
	userID = strings.TrimSpace(userID)

	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req dto.BatchReceiptRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(req.Receipts) == 0 {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	}

	// Best-effort: hata varsa logla ama 200 dön (receipt delivery isteğe bağlı)
	if err := h.msgSvc.SendReceipts(context.Background(), userID, shadeID, req.Receipts); err != nil {
		logger.Log.Warn("REST receipt batch processing failed",
			zap.String("user_id", userID),
			zap.Error(err))
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
}
