package dto

import (
	"core-backend/internal/validator"
	"time"
)

type CreateSessionResponse struct {
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AuthorizeRequest struct {
	Ciphertext       string `json:"ciphertext"`
	Nonce            string `json:"nonce"`
	AndroidX25519Pub string `json:"android_x25519_pub"`
}

func (r *AuthorizeRequest) Validate() error {
	return validator.New().
		Required("ciphertext", r.Ciphertext).
		HexString("ciphertext", r.Ciphertext).
		Required("nonce", r.Nonce).
		HexString("nonce", r.Nonce).
		Required("android_x25519_pub", r.AndroidX25519Pub).
		HexString("android_x25519_pub", r.AndroidX25519Pub).
		Result()
}

type SessionPollResponse struct {
	Ciphertext       string `json:"ciphertext"`
	Nonce            string `json:"nonce"`
	AndroidX25519Pub string `json:"android_x25519_pub"`
	AccessToken      string `json:"access_token,omitempty"`
	DeviceID         string `json:"device_id"`
}
