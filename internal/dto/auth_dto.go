package dto

import "core-backend/internal/validator"

// ── Register ──────────────────────────────────────────────────────────────────

type RegisterRequest struct {
	IdentityPublicKey             string `json:"identity_public_key"`
	EncryptedIdentityPrivateKey   string `json:"encrypted_identity_private_key"`
	EncryptionPublicKey           string `json:"encryption_public_key"`
	EncryptedEncryptionPrivateKey string `json:"encrypted_encryption_private_key"`
	Salt                          string `json:"salt"`
	DeviceModel                   string `json:"device_model"`
	FCMToken                      string `json:"fcm_token"`
}

func (r *RegisterRequest) Validate() error {
	return validator.New().
		Required("identity_public_key", r.IdentityPublicKey).
		ExactHexLen("identity_public_key", r.IdentityPublicKey, validator.Ed25519PublicKeyHexLen).
		Required("encrypted_identity_private_key", r.EncryptedIdentityPrivateKey).
		Required("encryption_public_key", r.EncryptionPublicKey).
		ExactHexLen("encryption_public_key", r.EncryptionPublicKey, validator.Ed25519PublicKeyHexLen).
		Required("encrypted_encryption_private_key", r.EncryptedEncryptionPrivateKey).
		Required("salt", r.Salt).
		MinLen("salt", r.Salt, validator.SaltMinLen).
		Required("device_model", r.DeviceModel).
		MaxLen("device_model", r.DeviceModel, validator.DeviceModelMaxLen).
		Result()
}

type RegisterResponse struct {
	CoreGuardID string `json:"core_guard_id"`
	UserID      string `json:"user_id"`
	Message     string `json:"message"`
	DeviceID    string `json:"device_id"`
}

// ── LoginInit ─────────────────────────────────────────────────────────────────

type LoginInitRequest struct {
	CoreGuardID string `json:"core_guard_id"`
}

func (r *LoginInitRequest) Validate() error {
	return validator.New().
		Required("core_guard_id", r.CoreGuardID).
		CoreGuardIDFormat("core_guard_id", r.CoreGuardID).
		Result()
}

type LoginInitResponse struct {
	EncryptedIdentityPrivateKey   string `json:"encrypted_identity_private_key"`
	EncryptedEncryptionPrivateKey string `json:"encrypted_encryption_private_key"`
	Salt                          string `json:"salt"`
	Challenge                     string `json:"challenge"`
}

// ── LoginVerify ───────────────────────────────────────────────────────────────

type LoginVerifyRequest struct {
	CoreGuardID string `json:"core_guard_id"`
	Challenge   string `json:"challenge"`
	Signature   string `json:"signature"`
	DeviceModel string `json:"device_model"`
	DeviceID    string `json:"device_id,omitempty"`
	FCMToken    string `json:"fcm_token"`
}

func (r *LoginVerifyRequest) Validate() error {
	return validator.New().
		Required("core_guard_id", r.CoreGuardID).
		CoreGuardIDFormat("core_guard_id", r.CoreGuardID).
		Required("challenge", r.Challenge).
		ExactHexLen("challenge", r.Challenge, validator.ChallengeHexLen).
		Required("signature", r.Signature).
		ExactHexLen("signature", r.Signature, validator.Ed25519SignatureHexLen).
		Required("device_model", r.DeviceModel).
		MaxLen("device_model", r.DeviceModel, validator.DeviceModelMaxLen).
		OptionalUUID("device_id", r.DeviceID).
		Result()
}

type LoginVerifyResponse struct {
	CoreGuardID string `json:"core_guard_id"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	AccessToken string `json:"access_token"`
	Message     string `json:"message"`
}
