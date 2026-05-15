package handlers

import (
	"core-backend/internal/dto"
	"core-backend/internal/services"
	"core-backend/internal/validator"
	"core-backend/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type AuthHandler struct {
	authService services.AuthService
}

func NewAuthHandler(s services.AuthService) *AuthHandler {
	return &AuthHandler{authService: s}
}

// validationError — validation hatalarını standart formatta döner
func validationError(c *fiber.Ctx, err error) error {
	if ve, ok := err.(validator.ValidationErrors); ok {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":  "validation_failed",
			"fields": ve,
		})
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"error": err.Error(),
	})
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterRequest

	if err := c.BodyParser(&req); err != nil {
		logger.Log.Warn("register: failed to parse request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON format",
		})
	}

	if err := req.Validate(); err != nil {
		logger.Log.Warn("register: validation failed", zap.Error(err))
		return validationError(c, err)
	}

	res, err := h.authService.Register(&req)
	if err != nil {
		logger.Log.Error("register: service error", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "could not process registration",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(res)
}

func (h *AuthHandler) LoginInit(c *fiber.Ctx) error {
	var req dto.LoginInitRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON format",
		})
	}

	if err := req.Validate(); err != nil {
		logger.Log.Warn("login/init: validation failed", zap.Error(err))
		return validationError(c, err)
	}

	res, err := h.authService.LoginInit(&req)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *AuthHandler) LoginVerify(c *fiber.Ctx) error {
	var req dto.LoginVerifyRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON format",
		})
	}

	if err := req.Validate(); err != nil {
		logger.Log.Warn("login/verify: validation failed", zap.Error(err))
		return validationError(c, err)
	}

	res, err := h.authService.LoginVerify(&req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}
