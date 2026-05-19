package dto

type LookupResponse struct {
	UserID              string  `json:"user_id"`
	ShadeID             string  `json:"shade_id"`
	EncryptionPublicKey string  `json:"encryption_public_key"`
	DisplayName         *string `json:"display_name,omitempty"`
	ProfileImageID      *string `json:"profile_image_id,omitempty"`
}

type UpdateDisplayNameRequest struct {
	DisplayName string `json:"display_name"`
}

type UpdateAvatarRequest struct {
	ImageID string `json:"image_id"`
}
