package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/repositories"
)

type UserService interface {
	GetUserForLookup(ctx context.Context, coreGuardID string) (*dto.LookupResponse, error)
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

	return &dto.LookupResponse{
		UserID:              user.UserID.String(),
		ShadeID:             user.CoreGuardID,
		EncryptionPublicKey: user.Key.EncryptionPublicKey,
	}, err
}
