package jwt

import (
	"core-backend/internal/config"
	"core-backend/pkg/logger"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateToken(userID string, coreGuardID string, deviceID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":       userID,
		"core_guard_id": coreGuardID,
		"device_id":     deviceID,
		"exp":           time.Now().Add(time.Hour * 24 * 30).Unix(),
	})

	secretKey := config.AppConfig.JWTSecret
	if secretKey == "" {
		logger.Log.Warn("JWT_SECRET is not set in .env file! Using fallback secret.")
		secretKey = "fallback-dev-secret-key"
	}

	return token.SignedString([]byte(secretKey))
}

func ParseToken(tokenString string) (string, string, string, error) {
	secretKey := config.AppConfig.JWTSecret
	if secretKey == "" {
		logger.Log.Warn("JWT_SECRET is not set in .env file! Using fallback secret.")
		secretKey = "fallback-dev-secret-key"
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return "", "", "", errors.New("invalid or expired token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", "", errors.New("invalid token claims")
	}

	userID, ok1 := claims["user_id"].(string)
	coreGuardID, ok2 := claims["core_guard_id"].(string)
	if !ok1 || !ok2 {
		return "", "", "", errors.New("missing required token claims")
	}
	// device_id eski token'larda olmayabilir — opsiyonel
	deviceID, _ := claims["device_id"].(string)

	return userID, coreGuardID, deviceID, nil
}
