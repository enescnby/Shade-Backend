package services

import (
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	pkgjwt "core-backend/pkg/jwt"
	"core-backend/pkg/logger"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuthService interface {
	Register(req *dto.RegisterRequest) (*dto.RegisterResponse, error)
	LoginInit(req *dto.LoginInitRequest) (*dto.LoginInitResponse, error)
	LoginVerify(req *dto.LoginVerifyRequest) (*dto.LoginVerifyResponse, error)
	Refresh(req *dto.RefreshRequest) (*dto.RefreshResponse, error)
}

type authService struct {
	userRepo        repositories.UserRepository
	auditRepo       repositories.AuditRepository
	refreshTokenRepo repositories.RefreshTokenRepository
	challengeCache  sync.Map
}

func NewAuthService(
	userRepo repositories.UserRepository,
	auditRepo repositories.AuditRepository,
	refreshTokenRepo repositories.RefreshTokenRepository,
) AuthService {
	return &authService{
		userRepo:        userRepo,
		auditRepo:       auditRepo,
		refreshTokenRepo: refreshTokenRepo,
	}
}

func (s *authService) Register(req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
	logger.Log.Info("starting registration process for new device", zap.String("device_model", req.DeviceModel))

	coreGuardID := generateCoreGuardID()

	newUser := &models.User{
		CoreGuardID: coreGuardID,
		Key: models.UserKey{
			IdentityPublicKey:             req.IdentityPublicKey,
			EncryptedIdentityPrivateKey:   req.EncryptedIdentityPrivateKey,
			EncryptionPublicKey:           req.EncryptionPublicKey,
			EncryptedEncryptionPrivateKey: req.EncryptedEncryptionPrivateKey,
			Salt:                          req.Salt,
		},
		Device: models.UserDevice{
			FCMToken:    req.FCMToken,
			DeviceModel: req.DeviceModel,
		},
	}

	if err := s.userRepo.CreateUser(newUser); err != nil {
		logger.Log.Error("registration failed at repository layer", zap.Error(err))
		return nil, err
	}

	dev, err := s.userRepo.GetDeviceByUserID(newUser.UserID)
	if err != nil {
		logger.Log.Error("registration succeeded but device row not found", zap.Error(err))
		return nil, errors.New("could not load device after registration")
	}

	auditLog := &models.SecurityAuditLog{
		UserID:     newUser.UserID,
		ActionType: "USER_REGISTERED",
		IPAddress:  "system",
	}

	_ = s.auditRepo.LogEvent(auditLog)

	logger.Log.Info("registration completed successfully", zap.String("core_guard_id", coreGuardID))

	return &dto.RegisterResponse{
		CoreGuardID: coreGuardID,
		UserID:      newUser.UserID.String(),
		DeviceID:    dev.DeviceID.String(),
		Message:     "Account created successfully. Keep your CoreGuard ID and PIN safe",
	}, nil
}

func (s *authService) LoginInit(req *dto.LoginInitRequest) (*dto.LoginInitResponse, error) {
	logger.Log.Info("login init requested", zap.String("core_guard_id", req.CoreGuardID))

	user, err := s.userRepo.GetUserByCoreGuardID(req.CoreGuardID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	challenge := generateCryptoChallenge()
	s.challengeCache.Store(req.CoreGuardID, challenge)

	logger.Log.Info("challenge generated for user", zap.String("core_guard_id", req.CoreGuardID))

	return &dto.LoginInitResponse{
		EncryptedIdentityPrivateKey:   user.Key.EncryptedIdentityPrivateKey,
		EncryptedEncryptionPrivateKey: user.Key.EncryptedEncryptionPrivateKey,
		Salt:                          user.Key.Salt,
		Challenge:                     challenge,
	}, nil
}

func (s *authService) LoginVerify(req *dto.LoginVerifyRequest) (*dto.LoginVerifyResponse, error) {

	logger.Log.Info("login verify requested", zap.String("core_guard_id", req.CoreGuardID))

	cachedChallenge, exists := s.challengeCache.Load(req.CoreGuardID)
	if !exists || cachedChallenge.(string) != req.Challenge {
		logger.Log.Warn("invalid or expired challenge", zap.String("core_guard_id", req.CoreGuardID))
		return nil, errors.New("invalid or expired challenge")
	}

	s.challengeCache.Delete(req.CoreGuardID)

	user, err := s.userRepo.GetUserByCoreGuardID(req.CoreGuardID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	pubKeyBytes, _ := hex.DecodeString(user.Key.IdentityPublicKey)
	signatureBytes, _ := hex.DecodeString(req.Signature)
	challengeBytes, _ := hex.DecodeString(req.Challenge)

	isValid := ed25519.Verify(pubKeyBytes, challengeBytes, signatureBytes)
	if !isValid {
		logger.Log.Warn("cryptographic signature verification failed", zap.String("core_guard_id", req.CoreGuardID))

		_ = s.auditRepo.LogEvent(&models.SecurityAuditLog{
			UserID:     user.UserID,
			ActionType: "LOGIN_FAILED_INVALID_SIGNATURE",
			IPAddress:  "system",
		})

		return nil, errors.New("invalid cryptographic signature")
	}

	var boundDevice *models.UserDevice
	deviceIDStr := strings.TrimSpace(req.DeviceID)

	if deviceIDStr != "" {
		deviceUUID, parseErr := uuid.Parse(deviceIDStr)
		if parseErr != nil {
			return nil, errors.New("invalid device_id")
		}

		dev, getErr := s.userRepo.GetDeviceByUserAndID(user.UserID, deviceUUID)
		if getErr != nil {
			if errors.Is(getErr, gorm.ErrRecordNotFound) {
				return nil, errors.New("unknown device")
			}
			return nil, errors.New("failed to load device")
		}

		dev.FCMToken = req.FCMToken
		dev.DeviceModel = req.DeviceModel
		dev.LastActive = time.Now().UTC()

		if updErr := s.userRepo.UpdateDeviceFields(dev); updErr != nil {
			logger.Log.Error("failed to update device on login", zap.Error(updErr))
			return nil, errors.New("failed to update device")
		}
		boundDevice = dev
	} else {
		dev := &models.UserDevice{
			DeviceID:    uuid.New(),
			UserID:      user.UserID,
			FCMToken:    req.FCMToken,
			DeviceModel: req.DeviceModel,
			LastActive:  time.Now().UTC(),
		}
		if crtErr := s.userRepo.CreateDevice(dev); crtErr != nil {
			logger.Log.Error("failed to create device on login", zap.Error(crtErr))
			return nil, errors.New("failed to register device")
		}
		boundDevice = dev
	}

	accessToken, err := pkgjwt.GenerateAccessToken(user.UserID.String(), user.CoreGuardID)
	if err != nil {
		logger.Log.Error("failed to generate access token", zap.Error(err))
		return nil, errors.New("internal server error during token generation")
	}

	rawRefresh, refreshHash, err := pkgjwt.GenerateOpaqueRefreshToken()
	if err != nil {
		logger.Log.Error("failed to generate refresh token", zap.Error(err))
		return nil, errors.New("internal server error during token generation")
	}

	_ = s.refreshTokenRepo.Save(&models.RefreshToken{
		UserID:    user.UserID,
		TokenHash: refreshHash,
		DeviceID:  0,
		ExpiresAt: time.Now().Add(pkgjwt.RefreshTokenTTL),
	})

	_ = s.auditRepo.LogEvent(&models.SecurityAuditLog{
		UserID:     user.UserID,
		ActionType: "LOGIN_SUCCESS",
		IPAddress:  "system",
	})

	logger.Log.Info("login verified successfully, tokens issued", zap.String("core_guard_id", req.CoreGuardID))

	return &dto.LoginVerifyResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		CoreGuardID:  user.CoreGuardID,
		UserID:       user.UserID.String(),
		DeviceID:     boundDevice.DeviceID.String(),
		Message:      "Welcome back! Cryptographic verification successful.",
	}, nil
}

func (s *authService) Refresh(req *dto.RefreshRequest) (*dto.RefreshResponse, error) {
	if req.RefreshToken == "" {
		return nil, errors.New("refresh_token is required")
	}

	hash, err := pkgjwt.HashRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	rt, err := s.refreshTokenRepo.FindByHash(hash)
	if err != nil {
		return nil, err // "refresh token not found" or "refresh token expired"
	}

	user, err := s.userRepo.GetUserByID(rt.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Rotate: delete old token, issue new pair
	_ = s.refreshTokenRepo.Delete(rt.ID)

	newAccess, err := pkgjwt.GenerateAccessToken(user.UserID.String(), user.CoreGuardID)
	if err != nil {
		return nil, errors.New("failed to generate access token")
	}

	newRawRefresh, newHash, err := pkgjwt.GenerateOpaqueRefreshToken()
	if err != nil {
		return nil, errors.New("failed to generate refresh token")
	}

	_ = s.refreshTokenRepo.Save(&models.RefreshToken{
		UserID:    user.UserID,
		TokenHash: newHash,
		DeviceID:  rt.DeviceID,
		ExpiresAt: time.Now().Add(pkgjwt.RefreshTokenTTL),
	})

	return &dto.RefreshResponse{
		AccessToken:  newAccess,
		RefreshToken: newRawRefresh,
	}, nil
}

func generateCoreGuardID() string {
	bytes := make([]byte, 4)
	_, _ = rand.Read(bytes)
	hexStr := strings.ToUpper(hex.EncodeToString(bytes))
	return "CG-" + hexStr[:4] + "-" + hexStr[4:]
}

func generateCryptoChallenge() string {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
