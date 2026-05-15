package handlers

import (
	"core-backend/internal/services"
	"errors"

	"github.com/gofiber/fiber/v2"
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
