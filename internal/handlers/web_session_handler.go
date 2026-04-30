package handlers

import (
	"core-backend/internal/dto"
	"core-backend/internal/services"
	"core-backend/pkg/logger"
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type WebSessionHandler struct {
	service services.WebSessionService
}

func NewWebSessionHandler(service services.WebSessionService) *WebSessionHandler {
	return &WebSessionHandler{service: service}
}

func (h *WebSessionHandler) CreateSession(c *fiber.Ctx) error {
	res, err := h.service.CreateSession()
	if err != nil {
		logger.Log.Error("failed to create web session", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not create session"})
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

func (h *WebSessionHandler) PollSession(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	res, err := h.service.PollSession(sessionID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrSessionNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})

		case errors.Is(err, services.ErrSessionExpired):
			return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "session expired"})

		default:
			logger.Log.Error("poll session error", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})

		}
	}
	if res == nil {
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "pending"})
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *WebSessionHandler) AuthorizeSession(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	var req dto.AuthorizeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "invalid JSON format"})
	}

	err := h.service.AuthorizeSession(sessionID, &req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrSessionNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})
		case errors.Is(err, services.ErrSessionExpired):
			return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "session expired"})
		case errors.Is(err, services.ErrAlreadyAuthorized):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "session already authorized"})
		case errors.Is(err, services.ErrMissingFields):
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		default:
			logger.Log.Error("authorize session error", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
}
