package services

import (
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	"core-backend/pkg/jwt"
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

// ── Challenge TTL sabitleri ──────────────────────────────────────────────────

const (
	challengeTTL             = 5 * time.Minute  // Challenge geçerlilik süresi
	challengeCleanupInterval = 2 * time.Minute  // Temizlik goroutine aralığı
	challengeNonceSize       = 32               // Byte cinsinden nonce boyutu
)

// challengeEntry — TTL bilgisiyle birlikte challenge kaydı
type challengeEntry struct {
	value     string
	expiresAt time.Time
}

// ── Service interface ────────────────────────────────────────────────────────

type AuthService interface {
	Register(req *dto.RegisterRequest) (*dto.RegisterResponse, error)
	LoginInit(req *dto.LoginInitRequest) (*dto.LoginInitResponse, error)
	LoginVerify(req *dto.LoginVerifyRequest) (*dto.LoginVerifyResponse, error)
	Shutdown()
}

// ── Service implementation ───────────────────────────────────────────────────

type authService struct {
	userRepo  repositories.UserRepository
	auditRepo repositories.AuditRepository

	// Challenge cache — mutex ile korunan, TTL'li map
	cacheMu        sync.Mutex
	challengeCache map[string]challengeEntry

	// Cleanup goroutine kontrolü
	stopCleanup chan struct{}
}

func NewAuthService(
	userRepo repositories.UserRepository,
	auditRepo repositories.AuditRepository,
) AuthService {
	svc := &authService{
		userRepo:       userRepo,
		auditRepo:      auditRepo,
		challengeCache: make(map[string]challengeEntry),
		stopCleanup:    make(chan struct{}),
	}

	// Arka planda süresi dolmuş challenge'ları temizle
	go svc.runCleanupWorker()

	return svc
}

// Shutdown — uygulama kapanırken cleanup goroutine'i düzgünce durdurur
func (s *authService) Shutdown() {
	close(s.stopCleanup)
}

// runCleanupWorker — periyodik olarak süresi dolmuş challenge'ları temizler
func (s *authService) runCleanupWorker() {
	ticker := time.NewTicker(challengeCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.evictExpiredChallenges()
		case <-s.stopCleanup:
			logger.Log.Info("challenge cleanup worker stopped")
			return
		}
	}
}

// evictExpiredChallenges — lock altında süresi dolmuş tüm kayıtları siler
func (s *authService) evictExpiredChallenges() {
	now := time.Now()
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	evicted := 0
	for key, entry := range s.challengeCache {
		if now.After(entry.expiresAt) {
			delete(s.challengeCache, key)
			evicted++
		}
	}

	if evicted > 0 {
		logger.Log.Info("expired challenges evicted",
			zap.Int("count", evicted),
			zap.Int("remaining", len(s.challengeCache)),
		)
	}
}

// storeChallenge — yeni challenge'ı TTL ile birlikte cache'e yazar
func (s *authService) storeChallenge(coreGuardID, challenge string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.challengeCache[coreGuardID] = challengeEntry{
		value:     challenge,
		expiresAt: time.Now().Add(challengeTTL),
	}
}

// consumeChallenge — challenge'ı doğrular, siler ve TTL kontrolü yapar.
// Her durumda (geçerli, süresi dolmuş, bulunamadı) challenge silinir — replay önlemi.
func (s *authService) consumeChallenge(coreGuardID, challenge string) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	entry, exists := s.challengeCache[coreGuardID]

	// Her durumda sil (replay attack önlemi)
	delete(s.challengeCache, coreGuardID)

	if !exists {
		return errors.New("challenge not found — request a new one via login/init")
	}

	if time.Now().After(entry.expiresAt) {
		return errors.New("challenge expired — request a new one via login/init")
	}

	if entry.value != challenge {
		return errors.New("challenge mismatch")
	}

	return nil
}

// ── Register ─────────────────────────────────────────────────────────────────

func (s *authService) Register(req *dto.RegisterRequest) (*dto.RegisterResponse, error) {
	logger.Log.Info("registration started", zap.String("device_model", req.DeviceModel))

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
		logger.Log.Error("registration failed", zap.Error(err))
		return nil, err
	}

	dev, err := s.userRepo.GetDeviceByUserID(newUser.UserID)
	if err != nil {
		logger.Log.Error("device not found after registration", zap.Error(err))
		return nil, errors.New("could not load device after registration")
	}

	_ = s.auditRepo.LogEvent(&models.SecurityAuditLog{
		UserID:     newUser.UserID,
		ActionType: "USER_REGISTERED",
		IPAddress:  "system",
	})

	logger.Log.Info("registration completed",
		zap.String("core_guard_id", coreGuardID),
		zap.String("user_id", newUser.UserID.String()),
	)

	return &dto.RegisterResponse{
		CoreGuardID: coreGuardID,
		UserID:      newUser.UserID.String(),
		DeviceID:    dev.DeviceID.String(),
		Message:     "Account created successfully. Keep your CoreGuard ID and PIN safe",
	}, nil
}

// ── LoginInit ────────────────────────────────────────────────────────────────

func (s *authService) LoginInit(req *dto.LoginInitRequest) (*dto.LoginInitResponse, error) {
	logger.Log.Info("login init requested", zap.String("core_guard_id", req.CoreGuardID))

	user, err := s.userRepo.GetUserByCoreGuardID(req.CoreGuardID)
	if err != nil {
		// Timing attack önlemi: kullanıcı bulunamasa bile aynı süre geçiyor gibi görünsün
		time.Sleep(50 * time.Millisecond)
		return nil, errors.New("user not found")
	}

	challenge := generateCryptoChallenge()

	// TTL ile birlikte cache'e yaz
	s.storeChallenge(req.CoreGuardID, challenge)

	logger.Log.Info("challenge issued",
		zap.String("core_guard_id", req.CoreGuardID),
		zap.Time("expires_at", time.Now().Add(challengeTTL)),
	)

	return &dto.LoginInitResponse{
		EncryptedIdentityPrivateKey:   user.Key.EncryptedIdentityPrivateKey,
		EncryptedEncryptionPrivateKey: user.Key.EncryptedEncryptionPrivateKey,
		Salt:                          user.Key.Salt,
		Challenge:                     challenge,
	}, nil
}

// ── LoginVerify ───────────────────────────────────────────────────────────────

func (s *authService) LoginVerify(req *dto.LoginVerifyRequest) (*dto.LoginVerifyResponse, error) {
	logger.Log.Info("login verify requested", zap.String("core_guard_id", req.CoreGuardID))

	// 1. Challenge'ı doğrula ve tüket (TTL + replay kontrolü dahil)
	if err := s.consumeChallenge(req.CoreGuardID, req.Challenge); err != nil {
		logger.Log.Warn("challenge validation failed",
			zap.String("core_guard_id", req.CoreGuardID),
			zap.Error(err),
		)
		return nil, err
	}

	// 2. Kullanıcıyı getir
	user, err := s.userRepo.GetUserByCoreGuardID(req.CoreGuardID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// 3. Public key decode
	pubKeyBytes, err := hex.DecodeString(user.Key.IdentityPublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		logger.Log.Error("invalid public key in database",
			zap.String("core_guard_id", req.CoreGuardID),
			zap.Int("key_length", len(pubKeyBytes)),
		)
		return nil, errors.New("invalid key configuration")
	}

	// 4. İmza ve challenge decode
	signatureBytes, err := hex.DecodeString(req.Signature)
	if err != nil {
		return nil, errors.New("invalid signature format")
	}

	challengeBytes, err := hex.DecodeString(req.Challenge)
	if err != nil {
		return nil, errors.New("invalid challenge format")
	}

	// 5. Ed25519 imza doğrulaması
	if !ed25519.Verify(pubKeyBytes, challengeBytes, signatureBytes) {
		logger.Log.Warn("signature verification failed",
			zap.String("core_guard_id", req.CoreGuardID),
		)
		_ = s.auditRepo.LogEvent(&models.SecurityAuditLog{
			UserID:     user.UserID,
			ActionType: "LOGIN_FAILED_INVALID_SIGNATURE",
			IPAddress:  "system",
		})
		return nil, errors.New("invalid cryptographic signature")
	}

	// 6. Cihaz kaydı / güncelleme
	boundDevice, err := s.resolveDevice(user, req)
	if err != nil {
		return nil, err
	}

	// 7. JWT üret
	tokenString, err := jwt.GenerateToken(
		user.UserID.String(),
		user.CoreGuardID,
		boundDevice.DeviceID.String(),
	)
	if err != nil {
		logger.Log.Error("JWT generation failed", zap.Error(err))
		return nil, errors.New("internal server error during token generation")
	}

	_ = s.auditRepo.LogEvent(&models.SecurityAuditLog{
		UserID:     user.UserID,
		ActionType: "LOGIN_SUCCESS",
		IPAddress:  "system",
	})

	logger.Log.Info("login successful",
		zap.String("core_guard_id", req.CoreGuardID),
		zap.String("device_id", boundDevice.DeviceID.String()),
	)

	return &dto.LoginVerifyResponse{
		AccessToken: tokenString,
		CoreGuardID: user.CoreGuardID,
		UserID:      user.UserID.String(),
		DeviceID:    boundDevice.DeviceID.String(),
		Message:     "Welcome back! Cryptographic verification successful.",
	}, nil
}

// resolveDevice — mevcut cihazı günceller veya yeni cihaz oluşturur
func (s *authService) resolveDevice(user *models.User, req *dto.LoginVerifyRequest) (*models.UserDevice, error) {
	deviceIDStr := strings.TrimSpace(req.DeviceID)

	if deviceIDStr != "" {
		deviceUUID, err := uuid.Parse(deviceIDStr)
		if err != nil {
			return nil, errors.New("invalid device_id format")
		}

		dev, err := s.userRepo.GetDeviceByUserAndID(user.UserID, deviceUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("unknown device")
			}
			return nil, errors.New("failed to load device")
		}

		dev.FCMToken = req.FCMToken
		dev.DeviceModel = req.DeviceModel
		dev.LastActive = time.Now().UTC()

		if err := s.userRepo.UpdateDeviceFields(dev); err != nil {
			logger.Log.Error("device update failed", zap.Error(err))
			return nil, errors.New("failed to update device")
		}
		return dev, nil
	}

	// Yeni cihaz
	dev := &models.UserDevice{
		DeviceID:    uuid.New(),
		UserID:      user.UserID,
		FCMToken:    req.FCMToken,
		DeviceModel: req.DeviceModel,
		LastActive:  time.Now().UTC(),
	}
	if err := s.userRepo.CreateDevice(dev); err != nil {
		logger.Log.Error("device creation failed", zap.Error(err))
		return nil, errors.New("failed to register device")
	}
	return dev, nil
}

// ── Yardımcı fonksiyonlar ────────────────────────────────────────────────────

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
