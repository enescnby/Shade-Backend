package validator

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ── Sabitler ──────────────────────────────────────────────────────────────────

const (
	// Ed25519 public key = 32 byte = 64 hex karakter
	Ed25519PublicKeyHexLen = 64
	// Ed25519 signature  = 64 byte = 128 hex karakter
	Ed25519SignatureHexLen = 128
	// Challenge nonce     = 32 byte = 64 hex karakter
	ChallengeHexLen = 64
	// DeviceModel max uzunluk
	DeviceModelMaxLen = 200
	// Salt minimum uzunluk (güvenlik için)
	SaltMinLen = 16
)

var coreGuardIDRegex = regexp.MustCompile(`^CG-[A-F0-9]{4}-[A-F0-9]{4}$`)

// ── ValidationError ───────────────────────────────────────────────────────────

// ValidationError — hangi alanda ne sorun olduğunu taşır
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors — birden fazla hatayı bir arada tutar
type ValidationErrors []*ValidationError

func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// HasErrors — hata var mı?
func (ve ValidationErrors) HasErrors() bool {
	return len(ve) > 0
}

// ── Builder — zincirleme validasyon ──────────────────────────────────────────

type Validator struct {
	errors ValidationErrors
}

func New() *Validator {
	return &Validator{}
}

// Result — validasyon bitti, sonucu döner
func (v *Validator) Result() error {
	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

// ── Kural fonksiyonları ───────────────────────────────────────────────────────

// Required — alan boş olamaz
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "is required",
		})
	}
	return v
}

// MaxLen — maksimum uzunluk kontrolü
func (v *Validator) MaxLen(field, value string, max int) *Validator {
	if len(value) > max {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must not exceed %d characters (got %d)", max, len(value)),
		})
	}
	return v
}

// MinLen — minimum uzunluk kontrolü
func (v *Validator) MinLen(field, value string, min int) *Validator {
	if len(strings.TrimSpace(value)) > 0 && len(value) < min {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at least %d characters (got %d)", min, len(value)),
		})
	}
	return v
}

// HexString — geçerli hex string mi?
func (v *Validator) HexString(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		return v // Required zaten kontrol eder
	}
	if _, err := hex.DecodeString(value); err != nil {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must be a valid hexadecimal string",
		})
	}
	return v
}

// ExactHexLen — hex string'in tam uzunluğunu kontrol eder
func (v *Validator) ExactHexLen(field, value string, expectedHexLen int) *Validator {
	if strings.TrimSpace(value) == "" {
		return v // Required zaten kontrol eder
	}
	if len(value) != expectedHexLen {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be exactly %d hex characters (got %d)", expectedHexLen, len(value)),
		})
		return v
	}
	if _, err := hex.DecodeString(value); err != nil {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must be a valid hexadecimal string",
		})
	}
	return v
}

// CoreGuardIDFormat — CG-XXXX-XXXX formatı
func (v *Validator) CoreGuardIDFormat(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		return v // Required zaten kontrol eder
	}
	if !coreGuardIDRegex.MatchString(value) {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must match format CG-XXXX-XXXX (uppercase hex)",
		})
	}
	return v
}

// OptionalUUID — boş olabilir ama doluysa geçerli UUID olmalı
func (v *Validator) OptionalUUID(field, value string) *Validator {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return v // Opsiyonel, boş geçebilir
	}
	if _, err := uuid.Parse(trimmed); err != nil {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must be a valid UUID",
		})
	}
	return v
}

// NotEmpty — byte slice boş olamaz
func (v *Validator) NotEmptyBytes(field string, value []byte) *Validator {
	if len(value) == 0 {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "is required and must not be empty",
		})
	}
	return v
}
