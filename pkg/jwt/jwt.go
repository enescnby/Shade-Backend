package jwt

import (
	"core-backend/internal/config"
	"core-backend/pkg/logger"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenTTL  = 1 * time.Hour        // short-lived: 1 hour
	RefreshTokenTTL = 90 * 24 * time.Hour  // long-lived: 90 days
)

func secret() []byte {
	key := config.AppConfig.JWTSecret
	if key == "" {
		logger.Log.Warn("JWT_SECRET is not set — using insecure fallback")
		key = "fallback-dev-secret-key"
	}
	return []byte(key)
}

// GenerateAccessToken issues a short-lived JWT (1 h).
func GenerateAccessToken(userID string, coreGuardID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":       userID,
		"core_guard_id": coreGuardID,
		"type":          "access",
		"exp":           time.Now().Add(AccessTokenTTL).Unix(),
	})
	return token.SignedString(secret())
}

// GenerateToken is kept as an alias so existing call-sites continue to compile.
// It delegates to GenerateAccessToken.
func GenerateToken(userID string, coreGuardID string) (string, error) {
	return GenerateAccessToken(userID, coreGuardID)
}

// ParseToken validates an access JWT and returns (userID, coreGuardID, error).
func ParseToken(tokenString string) (string, string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret(), nil
	})
	if err != nil {
		return "", "", errors.New("invalid or expired token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", errors.New("invalid token claims")
	}

	// Reject refresh tokens presented as access tokens
	if t, _ := claims["type"].(string); t == "refresh" {
		return "", "", errors.New("refresh token cannot be used as access token")
	}

	userID, _ := claims["user_id"].(string)
	coreGuardID, _ := claims["core_guard_id"].(string)
	if userID == "" || coreGuardID == "" {
		return "", "", errors.New("missing claims")
	}
	return userID, coreGuardID, nil
}

// GenerateOpaqueRefreshToken returns a 32-byte cryptographically-random token
// as a hex string (64 chars) and its SHA-256 hash for DB storage.
func GenerateOpaqueRefreshToken() (token string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = hex.EncodeToString(b)
	sum := sha256.Sum256(b)
	hash = hex.EncodeToString(sum[:])
	return
}

// HashRefreshToken returns the SHA-256 hex hash of a raw refresh token.
func HashRefreshToken(raw string) (string, error) {
	b, err := hex.DecodeString(raw)
	if err != nil {
		return "", errors.New("invalid refresh token format")
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
