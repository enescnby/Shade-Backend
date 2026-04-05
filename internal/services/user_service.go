package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/repositories"
	"time"
)

type UserService interface {
	GetUserForLookup(ctx context.Context, coreGuardID string) (*dto.LookupResponse, error)
	GetUserStatus(ctx context.Context, coreGuardID string) (*dto.UserStatusResponse, error)
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

func (s *userService) GetUserStatus(ctx context.Context, coreGuardID string) (*dto.UserStatusResponse, error) {
	user, err := s.repo.GetUserWithDeviceByShadeID(ctx, coreGuardID)
	if err != nil {
		return nil, err
	}

	lastActive := user.Device.LastActive
	isOnline := !lastActive.IsZero() && time.Since(lastActive) < 5*time.Minute

	lastActiveStr := ""
	if !lastActive.IsZero() {
		lastActiveStr = lastActive.UTC().Format(time.RFC3339)
	}

	return &dto.UserStatusResponse{
		ShadeID:    user.CoreGuardID,
		LastActive: lastActiveStr,
		IsOnline:   isOnline,
	}, nil
}
