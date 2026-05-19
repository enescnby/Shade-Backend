package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/repositories"

	"github.com/google/uuid"
)

type KeyService interface {
	GetPublicKey(ctx context.Context, targetID string) (*dto.GetPublicKeyResponse, error)
}

type keyService struct {
	keyRepo  repositories.KeyRepository
	userRepo repositories.UserRepository
}

func NewKeyService(keyRepo repositories.KeyRepository, userRepo repositories.UserRepository) KeyService {
	return &keyService{keyRepo: keyRepo, userRepo: userRepo}
}

// GetPublicKey returns the user's CoreGuard ID and X25519 encryption public key
// (same field as registration `encryption_public_key`). SKDM / 1-to-1 E2EE use
// this material — not the Ed25519 identity key used for login signatures.
func (s *keyService) GetPublicKey(ctx context.Context, targetID string) (*dto.GetPublicKeyResponse, error) {
	userUUID, err := uuid.Parse(targetID)
	if err != nil {
		return nil, err
	}
	uk, err := s.keyRepo.GetByUserID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetUserByID(userUUID)
	if err != nil {
		return nil, err
	}
	return &dto.GetPublicKeyResponse{
		CoreGuardID: user.CoreGuardID,
		PublicKey:   uk.EncryptionPublicKey,
	}, nil
}
