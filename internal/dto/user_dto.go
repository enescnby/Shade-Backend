package dto

type LookupResponse struct {
	UserID              string `json:"user_id"`
	ShadeID             string `json:"shade_id"`
	EncryptionPublicKey string `json:"encryption_public_key"`
}

type UserStatusResponse struct {
	ShadeID    string `json:"shade_id"`
	LastActive string `json:"last_active"` // ISO 8601 formatında
	IsOnline   bool   `json:"is_online"`   // Son 5 dakika içinde aktifse true
}
