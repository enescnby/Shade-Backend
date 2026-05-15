package handlers

import (
	"core-backend/internal/database"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HealthHandler — liveness ve readiness probe'ları için
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Live — sunucu ayakta mı? (Kubernetes liveness probe)
// Sadece process çalışıyor mu kontrol eder, bağımlılıklara bakmaz.
func (h *HealthHandler) Live(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// Ready — servis trafik almaya hazır mı? (Kubernetes readiness probe)
// Veritabanı bağlantısını kontrol eder.
func (h *HealthHandler) Ready(c *fiber.Ctx) error {
	if err := database.PingDB(); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "unavailable",
			"reason": "database unreachable",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":   "ready",
		"database": "ok",
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}
