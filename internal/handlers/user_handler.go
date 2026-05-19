package handlers

import (
	"context"
	"core-backend/internal/dto"
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

	// context.Background() — client disconnect olsa bile DB sorgusu tamamlansın
	res, err := h.userService.GetUserForLookup(context.Background(), coreGuardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "an error occurred"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *UserHandler) UpdateDisplayName(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("user_id").(string)
	if !ok || userIDStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userIDRaw, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	var req dto.UpdateDisplayNameRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.DisplayName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "display_name is required"})
	}

	if err := h.userService.UpdateDisplayName(c.UserContext(), userIDRaw, req.DisplayName); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "an error occurred"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "display name updated"})
}

func (h *UserHandler) UpdateAvatar(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("user_id").(string)
	if !ok || userIDStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userIDRaw, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	var req dto.UpdateAvatarRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	imageID, err := uuid.Parse(req.ImageID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid image_id"})
	}

	if err := h.userService.UpdateAvatar(c.UserContext(), userIDRaw, imageID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update avatar"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "avatar updated"})
}

func (h *UserHandler) RemoveAvatar(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("user_id").(string)
	if !ok || userIDStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userIDRaw, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	if err := h.userService.RemoveAvatar(c.UserContext(), userIDRaw); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not remove avatar"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "avatar removed"})
}
