package validator_test

import (
	"core-backend/internal/validator"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Required ──────────────────────────────────────────────────────────────────

func TestRequired_EmptyString_ReturnsError(t *testing.T) {
	err := validator.New().Required("field", "").Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field")
	assert.Contains(t, err.Error(), "required")
}

func TestRequired_WhitespaceOnly_ReturnsError(t *testing.T) {
	err := validator.New().Required("field", "   ").Result()
	require.Error(t, err)
}

func TestRequired_ValidValue_NoError(t *testing.T) {
	err := validator.New().Required("field", "value").Result()
	assert.NoError(t, err)
}

// ── ExactHexLen ───────────────────────────────────────────────────────────────

func TestExactHexLen_CorrectLength_NoError(t *testing.T) {
	validKey := strings.Repeat("a", 64)
	err := validator.New().ExactHexLen("key", validKey, 64).Result()
	assert.NoError(t, err)
}

func TestExactHexLen_TooShort_ReturnsError(t *testing.T) {
	err := validator.New().ExactHexLen("key", "abc", 64).Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "64")
}

func TestExactHexLen_TooLong_ReturnsError(t *testing.T) {
	longKey := strings.Repeat("a", 128)
	err := validator.New().ExactHexLen("key", longKey, 64).Result()
	require.Error(t, err)
}

func TestExactHexLen_InvalidHex_ReturnsError(t *testing.T) {
	invalidHex := strings.Repeat("g", 64)
	err := validator.New().ExactHexLen("key", invalidHex, 64).Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hexadecimal")
}

func TestExactHexLen_EmptyString_NoError(t *testing.T) {
	err := validator.New().ExactHexLen("key", "", 64).Result()
	assert.NoError(t, err)
}

// ── CoreGuardIDFormat ─────────────────────────────────────────────────────────

func TestCoreGuardIDFormat_ValidFormat_NoError(t *testing.T) {
	validIDs := []string{"CG-A1B2-C3D4", "CG-FFFF-0000", "CG-1234-ABCD"}
	for _, id := range validIDs {
		t.Run(id, func(t *testing.T) {
			err := validator.New().CoreGuardIDFormat("id", id).Result()
			assert.NoError(t, err)
		})
	}
}

func TestCoreGuardIDFormat_InvalidFormat_ReturnsError(t *testing.T) {
	invalidIDs := []string{
		"WRONG",
		"cg-a1b2-c3d4",
		"CG-A1B2C3D4",
		"CG-A1B2-C3D",
		"CG-A1B2-C3D45",
	}
	for _, id := range invalidIDs {
		t.Run(id, func(t *testing.T) {
			err := validator.New().CoreGuardIDFormat("id", id).Result()
			assert.Error(t, err)
		})
	}
}

// ── OptionalUUID ──────────────────────────────────────────────────────────────

func TestOptionalUUID_EmptyString_NoError(t *testing.T) {
	err := validator.New().OptionalUUID("id", "").Result()
	assert.NoError(t, err)
}

func TestOptionalUUID_ValidUUID_NoError(t *testing.T) {
	err := validator.New().OptionalUUID("id", "550e8400-e29b-41d4-a716-446655440000").Result()
	assert.NoError(t, err)
}

func TestOptionalUUID_InvalidUUID_ReturnsError(t *testing.T) {
	err := validator.New().OptionalUUID("id", "not-a-uuid").Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UUID")
}

// ── MinLen ────────────────────────────────────────────────────────────────────

func TestMinLen_AboveMinimum_NoError(t *testing.T) {
	err := validator.New().MinLen("salt", "exactly16chars!!", 16).Result()
	assert.NoError(t, err)
}

func TestMinLen_BelowMinimum_ReturnsError(t *testing.T) {
	err := validator.New().MinLen("salt", "short", 16).Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "16")
}

func TestMinLen_EmptyString_NoError(t *testing.T) {
	err := validator.New().MinLen("salt", "", 16).Result()
	assert.NoError(t, err)
}

// ── MaxLen ────────────────────────────────────────────────────────────────────

func TestMaxLen_WithinLimit_NoError(t *testing.T) {
	err := validator.New().MaxLen("model", "iPhone 15 Pro", 200).Result()
	assert.NoError(t, err)
}

func TestMaxLen_ExceedsLimit_ReturnsError(t *testing.T) {
	longValue := strings.Repeat("x", 201)
	err := validator.New().MaxLen("model", longValue, 200).Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "200")
}

// ── Zincirleme ────────────────────────────────────────────────────────────────

func TestChaining_MultipleErrors_AllReported(t *testing.T) {
	err := validator.New().
		Required("field1", "").
		Required("field2", "").
		Required("field3", "value").
		Result()

	require.Error(t, err)
	ve, ok := err.(validator.ValidationErrors)
	require.True(t, ok)
	assert.Len(t, ve, 2)
}

func TestChaining_AllValid_NoError(t *testing.T) {
	pubKey := strings.Repeat("a", 64)
	err := validator.New().
		Required("key", pubKey).
		ExactHexLen("key", pubKey, 64).
		CoreGuardIDFormat("id", "CG-ABCD-1234").
		Result()
	assert.NoError(t, err)
}

func TestValidationErrors_ContainsAllFields(t *testing.T) {
	err := validator.New().
		Required("alpha", "").
		Required("beta", "").
		Result()

	require.Error(t, err)
	errStr := err.Error()
	assert.Contains(t, errStr, "alpha")
	assert.Contains(t, errStr, "beta")
}

// ── Gerçek kullanım senaryoları ───────────────────────────────────────────────

func TestRegisterRequest_AllFieldsValid_NoError(t *testing.T) {
	pubKey := strings.Repeat("a", 64)
	err := validator.New().
		Required("identity_public_key", pubKey).
		ExactHexLen("identity_public_key", pubKey, validator.Ed25519PublicKeyHexLen).
		Required("encrypted_identity_private_key", "ENCRYPTED_DATA").
		Required("encryption_public_key", pubKey).
		ExactHexLen("encryption_public_key", pubKey, validator.Ed25519PublicKeyHexLen).
		Required("encrypted_encryption_private_key", "ENCRYPTED_DATA").
		Required("salt", "sixteen_chars!!!").
		MinLen("salt", "sixteen_chars!!!", validator.SaltMinLen).
		Required("device_model", "TestDevice").
		MaxLen("device_model", "TestDevice", validator.DeviceModelMaxLen).
		Result()
	assert.NoError(t, err)
}

func TestRegisterRequest_MissingFields_Returns6Errors(t *testing.T) {
	err := validator.New().
		Required("identity_public_key", "").
		Required("encrypted_identity_private_key", "").
		Required("encryption_public_key", "").
		Required("encrypted_encryption_private_key", "").
		Required("salt", "").
		Required("device_model", "").
		Result()

	require.Error(t, err)
	ve, ok := err.(validator.ValidationErrors)
	require.True(t, ok)
	assert.Len(t, ve, 6)
}

func TestLoginVerifyRequest_ValidHexFields_NoError(t *testing.T) {
	challenge := strings.Repeat("b", validator.ChallengeHexLen)
	signature := strings.Repeat("c", validator.Ed25519SignatureHexLen)

	err := validator.New().
		Required("challenge", challenge).
		ExactHexLen("challenge", challenge, validator.ChallengeHexLen).
		Required("signature", signature).
		ExactHexLen("signature", signature, validator.Ed25519SignatureHexLen).
		Result()
	assert.NoError(t, err)
}

func TestLoginVerifyRequest_WrongSignatureLength_ReturnsError(t *testing.T) {
	shortSig := strings.Repeat("c", 64) // 128 olmalı
	err := validator.New().
		Required("signature", shortSig).
		ExactHexLen("signature", shortSig, validator.Ed25519SignatureHexLen).
		Result()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "128")
}
