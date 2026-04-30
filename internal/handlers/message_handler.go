package handlers

import (
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
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			limit = v
		}
	}

	response, err := h.msgSvc.DrainInbox(userID, limit)
	if err != nil {
		logger.Log.Error("inbox drain failed",
			zap.String("user_id", userID), zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(fiber.StatusOK).JSON(response)
}
