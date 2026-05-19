package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/repositories"

	"github.com/google/uuid"
)

type UserService interface {
	GetUserForLookup(ctx context.Context, coreGuardID string) (*dto.LookupResponse, error)
	UpdateDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error
	UpdateAvatar(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) error
	RemoveAvatar(ctx context.Context, userID uuid.UUID) error
}

type userService struct {
	repo repositories.UserRepository
}

func NewUserService(repo repositories.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) GetUserForLookup(ctx context.Context, coreGuardID string) (*dto.LookupResponse, error) {
	user, err := s.repo.GetUserForLookup(ctx, coreGuardID)
	if err != nil {
		return nil, err
	}

	resp := &dto.LookupResponse{
		UserID:              user.UserID.String(),
		ShadeID:             user.CoreGuardID,
		EncryptionPublicKey: user.Key.EncryptionPublicKey,
		DisplayName:         user.DisplayName,
	}
	if user.ProfileImageID != nil {
		s := user.ProfileImageID.String()
		resp.ProfileImageID = &s
	}
	return resp, nil
}

func (s *userService) UpdateAvatar(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) error {
	return s.repo.UpdateProfileImage(ctx, userID, &imageID)
}

func (s *userService) RemoveAvatar(ctx context.Context, userID uuid.UUID) error {
	return s.repo.UpdateProfileImage(ctx, userID, nil)
}

func (s *userService) UpdateDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error {
	return s.repo.UpdateDisplayName(ctx, userID, displayName)
}
