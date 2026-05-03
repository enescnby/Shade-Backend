package middleware

import (
	"core-backend/pkg/jwt"
	"core-backend/pkg/logger"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func Protected() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		authHeader := ctx.Get("Authorization")

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			logger.Log.Warn("unauthorized access attempt: missing or invalid header", zap.String("ip", ctx.IP()))
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing or invalid authorization header (format: Bearer <token>)",
			})
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		userID, coreGuardID, deviceID, err := jwt.ParseToken(tokenString)
		if err != nil {
			logger.Log.Warn("unauthorized access attempt: invalid token", zap.Error(err), zap.String("ip", ctx.IP()))
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		ctx.Locals("user_id", userID)
		ctx.Locals("core_guard_id", coreGuardID)
		ctx.Locals("device_id", deviceID)

		return ctx.Next()
	}
}
