package dto

import "time"

type CreateSessionResponse struct {
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AuthorizeRequest struct {
	Ciphertext       string `json:"ciphertext"`
	Nonce            string `json:"nonce"`
	AndroidX25519Pub string `json:"android_x25519_pub"`
}

type SessionPollResponse struct {
	Ciphertext       string `json:"ciphertext"`
	Nonce            string `json:"nonce"`
	AndroidX25519Pub string `json:"android_x25519_pub"`
}
