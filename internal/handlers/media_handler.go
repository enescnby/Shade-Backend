package handlers

import (
	"context"
	"core-backend/internal/services"
	"core-backend/pkg/logger"
	"errors"
	"fmt"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type MediaHandler struct {
	mediaService services.MediaService
}

func NewMediaHandler(s services.MediaService) *MediaHandler {
	return &MediaHandler{mediaService: s}
}

func (h *MediaHandler) Upload(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	uploaderID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	fileHeader, err := c.FormFile("image")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image field is required"})
	}

	file, err := fileHeader.Open()
	if err != nil {
		logger.Log.Error("failed to open uploaded file", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to process uploaded file"})
	}
	defer file.Close()

	res, err := h.mediaService.Upload(c.Context(), uploaderID, file, fileHeader.Size)
	if err != nil {
		if err.Error() == "file size exceeds maximum allowed size" {
			return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": err.Error()})
		}
		logger.Log.Error("failed to upload image", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to upload image"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *MediaHandler) Download(c *fiber.Ctx) error {
	imageIDStr := c.Params("imageId")

	imageID, err := uuid.Parse(imageIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid image id"})
	}

	body, err := h.mediaService.Download(context.Background(), imageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "image not found"})
		}
		logger.Log.Error("failed to download image", zap.String("image_id", imageIDStr), zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to download image"})
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		logger.Log.Error("failed to read image body", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read image"})
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	return c.Send(data)
}
