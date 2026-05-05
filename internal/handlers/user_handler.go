package handlers

import (
	"core-backend/internal/services"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserHandler struct {
	userService services.UserService
}

func NewUserHandler(service services.UserService) *UserHandler {
	return &UserHandler{userService: service}
}

func (h *UserHandler) GetUserForLookup(c *fiber.Ctx) error {
	coreGuardID := c.Params("shadeId")
	if coreGuardID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ID needed"})
	}

	res, err := h.userService.GetUserForLookup(c.UserContext(), coreGuardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "an error occurred"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *UserHandler) UpdateFCMToken(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	var body struct {
		FCMToken string `json:"fcm_token"`
	}
	if err := c.BodyParser(&body); err != nil || body.FCMToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "fcm_token required"})
	}

	if err := h.userService.UpdateFCMToken(c.UserContext(), userID, body.FCMToken); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update fcm token"})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
}

func (h *UserHandler) GetUserStatus(c *fiber.Ctx) error {
	coreGuardID := c.Params("shadeId")
	if coreGuardID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "shade ID required"})
	}

	res, err := h.userService.GetUserStatus(c.UserContext(), coreGuardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "an error occurred"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}
