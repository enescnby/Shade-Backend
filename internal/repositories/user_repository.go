package repositories

import (
	"context"
	"core-backend/internal/models"
	"core-backend/pkg/logger"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UserRepository interface {
	CreateUser(user *models.User) error
	GetUserByID(userID uuid.UUID) (*models.User, error)
	GetDeviceByUserID(userID uuid.UUID) (*models.UserDevice, error)
	GetUserByCoreGuardID(coreGuardID string) (*models.User, error)
	UpdateDevice(userID uuid.UUID, newDevice *models.UserDevice) error
	GetUserForLookup(ctx context.Context, coreGuardID string) (*models.User, error)
	GetUserWithDeviceByShadeID(ctx context.Context, coreGuardID string) (*models.User, error)
	UpdateDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error
	UpdateLastActive(userID uuid.UUID) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) CreateUser(user *models.User) error {
	if err := r.db.Create(user).Error; err != nil {
		logger.Log.Error("Failed to create user", zap.Error(err))
		return err
	}
	return nil
}

func (r *userRepository) GetUserByID(userID uuid.UUID) (*models.User, error) {
	var user models.User

	if err := r.db.Where("user_id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Log.Warn("user not found", zap.String("userID", userID.String()))
			return nil, err
		}
		logger.Log.Error("database error while fetching user", zap.Error(err))
		return nil, err
	}

	return &user, nil
}

func (r *userRepository) GetUserByCoreGuardID(coreGuardID string) (*models.User, error) {
	var user models.User

	err := r.db.Preload("Key").Preload("Device").Where("core_guard_id = ?", coreGuardID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Log.Warn("user not found", zap.String("coreGuardID", coreGuardID))
			return nil, err
		}
		logger.Log.Error("database error while fetching user", zap.Error(err))
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetDeviceByUserID(userID uuid.UUID) (*models.UserDevice, error) {
	var device models.UserDevice

	err := r.db.Where("user_id = ?", userID).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *userRepository) UpdateDevice(userID uuid.UUID, newDevice *models.UserDevice) error {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserDevice{}).Error; err != nil {
			return err
		}
		return tx.Create(newDevice).Error
	})

	if err != nil {
		logger.Log.Error("failed to update user device", zap.Error(err))
		return err
	}
	return nil
}

func (r *userRepository) GetUserForLookup(ctx context.Context, coreGuardID string) (*models.User, error) {
	var user models.User

	err := r.db.WithContext(ctx).
		Preload("Key").
		Where("core_guard_id = ?", coreGuardID).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Log.Warn("User can not found for lookup", zap.String("core_guard_id", coreGuardID))
			return nil, err
		}
		logger.Log.Error("Database error while lookup", zap.Error(err))
		return nil, err
	}

	return &user, err
}

func (r *userRepository) UpdateLastActive(userID uuid.UUID) error {
	return r.db.Model(&models.UserDevice{}).
		Where("user_id = ?", userID).
		Update("last_active", time.Now()).Error
}

func (r *userRepository) UpdateDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("user_id = ?", userID).
		Update("display_name", displayName).Error
}

func (r *userRepository) GetUserWithDeviceByShadeID(ctx context.Context, coreGuardID string) (*models.User, error) {
	var user models.User

	err := r.db.WithContext(ctx).
		Preload("Device").
		Where("core_guard_id = ?", coreGuardID).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Log.Warn("user not found for status", zap.String("core_guard_id", coreGuardID))
			return nil, err
		}
		logger.Log.Error("database error while fetching user status", zap.Error(err))
		return nil, err
	}

	return &user, nil
}
