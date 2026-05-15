package repositories

import (
	"context"
	"core-backend/internal/models"
	"core-backend/pkg/logger"
	"errors"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UserRepository interface {
	CreateUser(user *models.User) error
	GetUserByID(userID uuid.UUID) (*models.User, error)
	GetDeviceByUserID(userID uuid.UUID) (*models.UserDevice, error)
	GetUserByCoreGuardID(coreGuardID string) (*models.User, error)
	GetUserForLookup(ctx context.Context, coreGuardID string) (*models.User, error)
	CreateDevice(userDevice *models.UserDevice) error
	GetDeviceByUserAndID(userID, deviceID uuid.UUID) (*models.UserDevice, error)
	UpdateDeviceFields(device *models.UserDevice) error
	ListDevicesByUserID(userID uuid.UUID) ([]models.UserDevice, error)
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

func (r *userRepository) CreateDevice(userDevice *models.UserDevice) error {
	if err := r.db.Create(userDevice).Error; err != nil {
		logger.Log.Error("failed to create device", zap.Error(err))
		return err
	}
	return nil
}

func (r *userRepository) GetDeviceByUserAndID(userID, deviceID uuid.UUID) (*models.UserDevice, error) {
	var device models.UserDevice
	err := r.db.Where("user_id = ? AND device_id = ?", userID, deviceID).First(&device).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		logger.Log.Error("failed to get device by user and id", zap.Error(err))
		return nil, err
	}
	return &device, nil
}

func (r *userRepository) UpdateDeviceFields(device *models.UserDevice) error {
	res := r.db.Model(&models.UserDevice{}).
		Where("device_id = ? AND user_id = ?", device.DeviceID, device.UserID).
		Updates(map[string]interface{}{
			"fcm_token":    device.FCMToken,
			"device_model": device.DeviceModel,
			"last_active":  device.LastActive,
		})
	if res.Error != nil {
		logger.Log.Error("failed to update device fields", zap.Error(res.Error))
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *userRepository) ListDevicesByUserID(userID uuid.UUID) ([]models.UserDevice, error) {
	var devices []models.UserDevice
	if err := r.db.Where("user_id = ?", userID).Order("device_id ASC").Find(&devices).Error; err != nil {
		logger.Log.Error("failed to list devices by user", zap.Error(err))
		return nil, err
	}
	return devices, nil
}
