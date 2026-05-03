package dto

type RegisterRequest struct {
	IdentityPublicKey             string `json:"identity_public_key"`
	EncryptedIdentityPrivateKey   string `json:"encrypted_identity_private_key"`
	EncryptionPublicKey           string `json:"encryption_public_key"`
	EncryptedEncryptionPrivateKey string `json:"encrypted_encryption_private_key"`
	Salt                          string `json:"salt"`
	DeviceModel                   string `json:"device_model"`
	FCMToken                      string `json:"fcm_token"`
}

type RegisterResponse struct {
	CoreGuardID string `json:"core_guard_id"`
	UserID      string `json:"user_id"`
	Message     string `json:"message"`
	DeviceID    string `json:"device_id"`
}

type LoginInitRequest struct {
	CoreGuardID string `json:"core_guard_id"`
}

type LoginInitResponse struct {
	EncryptedIdentityPrivateKey   string `json:"encrypted_identity_private_key"`
	EncryptedEncryptionPrivateKey string `json:"encrypted_encryption_private_key"`
	Salt                          string `json:"salt"`
	Challenge                     string `json:"challenge"`
}

type LoginVerifyRequest struct {
	CoreGuardID string `json:"core_guard_id"`
	Challenge   string `json:"challenge"`
	Signature   string `json:"signature"`
	DeviceModel string `json:"device_model"`
	DeviceID    string `json:"device_id,omitempty"`
	FCMToken    string `json:"fcm_token"`
}

type LoginVerifyResponse struct {
	CoreGuardID string `json:"core_guard_id"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	AccessToken string `json:"access_token"`
	Message     string `json:"message"`
}
