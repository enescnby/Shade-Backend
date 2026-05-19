package services_test

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/services"
	"core-backend/pkg/logger"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestMain — tüm testlerden önce logger'ı başlat
func TestMain(m *testing.M) {
	logger.InitLogger()
	m.Run()
}

// ── Mock: UserRepository (tam interface) ─────────────────────────────────────

type mockUserRepo struct{ mock.Mock }

func (m *mockUserRepo) CreateUser(user *models.User) error {
	return m.Called(user).Error(0)
}
func (m *mockUserRepo) GetUserByID(userID uuid.UUID) (*models.User, error) {
	args := m.Called(userID)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockUserRepo) GetUserByCoreGuardID(id string) (*models.User, error) {
	args := m.Called(id)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockUserRepo) GetUserForLookup(ctx context.Context, id string) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockUserRepo) GetDeviceByUserID(userID uuid.UUID) (*models.UserDevice, error) {
	args := m.Called(userID)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.UserDevice), args.Error(1)
}
func (m *mockUserRepo) GetDeviceByUserAndID(userID, deviceID uuid.UUID) (*models.UserDevice, error) {
	args := m.Called(userID, deviceID)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.UserDevice), args.Error(1)
}
func (m *mockUserRepo) CreateDevice(dev *models.UserDevice) error {
	return m.Called(dev).Error(0)
}
func (m *mockUserRepo) UpdateDeviceFields(dev *models.UserDevice) error {
	return m.Called(dev).Error(0)
}
func (m *mockUserRepo) ListDevicesByUserID(userID uuid.UUID) ([]models.UserDevice, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.UserDevice), args.Error(1)
}
func (m *mockUserRepo) UpdateDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error {
	return m.Called(ctx, userID, displayName).Error(0)
}
func (m *mockUserRepo) UpdateProfileImage(ctx context.Context, userID uuid.UUID, imageID *uuid.UUID) error {
	return m.Called(ctx, userID, imageID).Error(0)
}

// ── Mock: AuditRepository ─────────────────────────────────────────────────────

type mockAuditRepo struct{ mock.Mock }

func (m *mockAuditRepo) LogEvent(log *models.SecurityAuditLog) error {
	return m.Called(log).Error(0)
}
func (m *mockAuditRepo) ListByUserID(userID uuid.UUID, limit int) ([]models.SecurityAuditLog, error) {
	args := m.Called(userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.SecurityAuditLog), args.Error(1)
}

// ── Yardımcılar ───────────────────────────────────────────────────────────────

func newTestUser(coreGuardID, pubKeyHex string) *models.User {
	return &models.User{
		UserID:      uuid.New(),
		CoreGuardID: coreGuardID,
		Key: models.UserKey{
			IdentityPublicKey:             pubKeyHex,
			EncryptedIdentityPrivateKey:   "enc_identity_priv",
			EncryptedEncryptionPrivateKey: "enc_encryption_priv",
			Salt:                          "test_salt_value",
		},
	}
}

func newTestDevice(userID uuid.UUID) *models.UserDevice {
	return &models.UserDevice{
		DeviceID:   uuid.New(),
		UserID:     userID,
		DeviceModel: "test-device",
		FCMToken:   "test-fcm",
		LastActive: time.Now().UTC(),
	}
}

func newAuthService(u *mockUserRepo, a *mockAuditRepo) services.AuthService {
	return services.NewAuthService(u, a)
}

// ── LoginInit ─────────────────────────────────────────────────────────────────

func TestLoginInit_UserNotFound_ReturnsError(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	u.On("GetUserByCoreGuardID", "CG-NOTF-0000").Return(nil, errors.New("not found"))

	svc := newAuthService(u, a)
	_, err := svc.LoginInit(&dto.LoginInitRequest{CoreGuardID: "CG-NOTF-0000"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	u.AssertExpectations(t)
}

func TestLoginInit_ValidUser_ReturnsChallenge(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	user := newTestUser("CG-AAAA-1111", "aabbcc")
	u.On("GetUserByCoreGuardID", "CG-AAAA-1111").Return(user, nil)

	svc := newAuthService(u, a)
	res, err := svc.LoginInit(&dto.LoginInitRequest{CoreGuardID: "CG-AAAA-1111"})

	require.NoError(t, err)
	assert.NotEmpty(t, res.Challenge)
	assert.Len(t, res.Challenge, 64, "challenge 32 byte = 64 hex olmali")
	assert.Equal(t, user.Key.EncryptedIdentityPrivateKey, res.EncryptedIdentityPrivateKey)
	u.AssertExpectations(t)
}

func TestLoginInit_TwoCalls_DifferentChallenges(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	user := newTestUser("CG-BBBB-2222", "aabbcc")
	u.On("GetUserByCoreGuardID", "CG-BBBB-2222").Return(user, nil).Times(2)

	svc := newAuthService(u, a)
	res1, _ := svc.LoginInit(&dto.LoginInitRequest{CoreGuardID: "CG-BBBB-2222"})
	res2, _ := svc.LoginInit(&dto.LoginInitRequest{CoreGuardID: "CG-BBBB-2222"})

	assert.NotEqual(t, res1.Challenge, res2.Challenge, "her challenge benzersiz olmali")
}

// ── Challenge TTL / Replay Testleri (Issue #4 dogrulamasi) ───────────────────

func TestChallenge_AfterVerify_CannotBeReused(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	user := newTestUser("CG-CCCC-3333", "aabbcc")
	u.On("GetUserByCoreGuardID", "CG-CCCC-3333").Return(user, nil)

	svc := newAuthService(u, a)
	res, err := svc.LoginInit(&dto.LoginInitRequest{CoreGuardID: "CG-CCCC-3333"})
	require.NoError(t, err)

	// Ilk verify — challenge tüketiliyor (imza yanlış ama challenge silinmeli)
	_, _ = svc.LoginVerify(&dto.LoginVerifyRequest{
		CoreGuardID: "CG-CCCC-3333",
		Challenge:   res.Challenge,
		Signature:   "badhex",
		DeviceModel: "test",
	})

	// Ikinci verify — ayni challenge tekrar — NOT FOUND gelmeli
	_, err2 := svc.LoginVerify(&dto.LoginVerifyRequest{
		CoreGuardID: "CG-CCCC-3333",
		Challenge:   res.Challenge,
		Signature:   "badhex",
		DeviceModel: "test",
	})
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "not found",
		"Tüketilmis challenge replay için kullanilamamali")
}

func TestChallenge_NeverIssued_ReturnsNotFound(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	svc := newAuthService(u, a)

	_, err := svc.LoginVerify(&dto.LoginVerifyRequest{
		CoreGuardID: "CG-FFFF-6666",
		Challenge:   "deadbeefdeadbeef",
		Signature:   "sig",
		DeviceModel: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestChallenge_WrongCoreGuardID_ReturnsNotFound(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	user := newTestUser("CG-DDDD-4444", "aabbcc")
	u.On("GetUserByCoreGuardID", "CG-DDDD-4444").Return(user, nil)

	svc := newAuthService(u, a)
	res, err := svc.LoginInit(&dto.LoginInitRequest{CoreGuardID: "CG-DDDD-4444"})
	require.NoError(t, err)

	// Farkli CoreGuardID ile verify — challenge bulunamaz
	_, err2 := svc.LoginVerify(&dto.LoginVerifyRequest{
		CoreGuardID: "CG-EEEE-5555",
		Challenge:   res.Challenge,
		Signature:   "sig",
		DeviceModel: "test",
	})
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "not found")
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegister_Success_ReturnsCoreGuardID(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}

	u.On("CreateUser", mock.AnythingOfType("*models.User")).Return(nil)
	u.On("GetDeviceByUserID", mock.AnythingOfType("uuid.UUID")).
		Return(&models.UserDevice{DeviceID: uuid.New()}, nil)
	a.On("LogEvent", mock.AnythingOfType("*models.SecurityAuditLog")).Return(nil)

	svc := newAuthService(u, a)
	res, err := svc.Register(&dto.RegisterRequest{
		IdentityPublicKey:             "aabb",
		EncryptedIdentityPrivateKey:   "enc",
		EncryptionPublicKey:           "ccdd",
		EncryptedEncryptionPrivateKey: "enc2",
		Salt:                          "salt",
		DeviceModel:                   "test-device",
		FCMToken:                      "fcm-token",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, res.CoreGuardID)
	assert.Regexp(t, `^CG-[A-F0-9]{4}-[A-F0-9]{4}$`, res.CoreGuardID)
	u.AssertExpectations(t)
}

func TestRegister_DBError_ReturnsError(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	u.On("CreateUser", mock.AnythingOfType("*models.User")).
		Return(errors.New("db connection refused"))

	svc := newAuthService(u, a)
	_, err := svc.Register(&dto.RegisterRequest{DeviceModel: "test"})

	require.Error(t, err)
	u.AssertExpectations(t)
}

// ── Shutdown ──────────────────────────────────────────────────────────────────

func TestShutdown_NoPanic(t *testing.T) {
	u := &mockUserRepo{}
	a := &mockAuditRepo{}
	svc := newAuthService(u, a)

	assert.NotPanics(t, func() { svc.Shutdown() })
}
