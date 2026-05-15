package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const baseHTTP = "http://127.0.0.1:8080/api/v1"
const baseWS = "ws://127.0.0.1:8080/api/v1/ws"

// ── Request / Response tipleri — gerçek DTO ile eşleşiyor ──────────────────

type RegisterRequest struct {
	IdentityPublicKey             string `json:"identity_public_key"`
	EncryptedIdentityPrivateKey   string `json:"encrypted_identity_private_key"`
	EncryptionPublicKey           string `json:"encryption_public_key"`
	EncryptedEncryptionPrivateKey string `json:"encrypted_encryption_private_key"`
	Salt                          string `json:"salt"`
	DeviceModel                   string `json:"device_model"`
	FCMToken                      string `json:"fcm_token"`
}

type RegisterResponse struct {
	CoreGuardID string `json:"core_guard_id"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	Message     string `json:"message"`
}

type LoginInitRequest struct {
	CoreGuardID string `json:"core_guard_id"`
}

type LoginInitResponse struct {
	EncryptedIdentityPrivateKey   string `json:"encrypted_identity_private_key"`
	EncryptedEncryptionPrivateKey string `json:"encrypted_encryption_private_key"`
	Salt                          string `json:"salt"`
	Challenge                     string `json:"challenge"`
}

type LoginVerifyRequest struct {
	CoreGuardID string `json:"core_guard_id"`
	Challenge   string `json:"challenge"`
	Signature   string `json:"signature"`
	DeviceModel string `json:"device_model"`
	DeviceID    string `json:"device_id,omitempty"`
	FCMToken    string `json:"fcm_token"`
}

type LoginVerifyResponse struct {
	AccessToken string `json:"access_token"`
	CoreGuardID string `json:"core_guard_id"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	Message     string `json:"message"`
}

// ── HTTP yardımcısı ─────────────────────────────────────────────────────────

func postJSON[T any](path string, body any) (int, T, []byte) {
	var out T
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseHTTP+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("HTTP request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, raw
}

// ── Ana akış ────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("=== E2E Test: register → login/init → login/verify → WebSocket ===")
	fmt.Println()

	// 1. Ed25519 anahtar çifti üret
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		fmt.Printf("key generation failed: %v\n", err)
		os.Exit(1)
	}
	pubHex := hex.EncodeToString(pub) // 64 hex karakter — validation geçer
	fmt.Printf("✓ Ed25519 key pair generated. Public key: %s...\n", pubHex[:16])

	// 2. Kayıt — tüm alanlar validation kurallarını karşılamalı
	regStatus, regRes, regRaw := postJSON[RegisterResponse]("/auth/register", RegisterRequest{
		IdentityPublicKey:             pubHex,                          // 64 hex ✓
		EncryptedIdentityPrivateKey:   "DUMMY_ENCRYPTED_IDENTITY_KEY",  // required ✓
		EncryptionPublicKey:           pubHex,                          // 64 hex ✓
		EncryptedEncryptionPrivateKey: "DUMMY_ENCRYPTED_ENCRYPTION_KEY", // required ✓
		Salt:                          "DUMMY_SALT_FOR_E2E_TEST_32CH",  // >= 16 char ✓
		DeviceModel:                   "e2e-test-device",               // required ✓
		FCMToken:                      "e2e-test-fcm-token",
	})
	if regStatus != 201 && regStatus != 200 {
		fmt.Printf("✗ register failed: status=%d body=%s\n", regStatus, string(regRaw))
		os.Exit(1)
	}
	fmt.Printf("✓ register ok: CoreGuardID=%s  UserID=%s\n", regRes.CoreGuardID, regRes.UserID)

	// 3. Login Init
	initStatus, initRes, initRaw := postJSON[LoginInitResponse]("/auth/login/init", LoginInitRequest{
		CoreGuardID: regRes.CoreGuardID,
	})
	if initStatus != 200 {
		fmt.Printf("✗ login/init failed: status=%d body=%s\n", initStatus, string(initRaw))
		os.Exit(1)
	}
	fmt.Printf("✓ login/init ok. Challenge: %s...\n", initRes.Challenge[:16])

	// 4. Challenge'ı imzala
	chBytes, err := hex.DecodeString(initRes.Challenge)
	if err != nil {
		fmt.Printf("✗ challenge decode failed: %v\n", err)
		os.Exit(1)
	}
	sig := ed25519.Sign(priv, chBytes)
	sigHex := hex.EncodeToString(sig) // 128 hex karakter — validation geçer

	// 5. Login Verify
	verifyStatus, verifyRes, verifyRaw := postJSON[LoginVerifyResponse]("/auth/login/verify", LoginVerifyRequest{
		CoreGuardID: regRes.CoreGuardID,
		Challenge:   initRes.Challenge, // 64 hex ✓
		Signature:   sigHex,            // 128 hex ✓
		DeviceModel: "e2e-test-device",
		DeviceID:    regRes.DeviceID,
		FCMToken:    "e2e-test-fcm-token",
	})
	if verifyStatus != 200 {
		fmt.Printf("✗ login/verify failed: status=%d body=%s\n", verifyStatus, string(verifyRaw))
		os.Exit(1)
	}
	if verifyRes.AccessToken == "" {
		fmt.Printf("✗ login/verify: no access token: %s\n", string(verifyRaw))
		os.Exit(1)
	}
	fmt.Printf("✓ login/verify ok. Token (first 20): %s...\n", verifyRes.AccessToken[:20])

	// 6. WebSocket bağlantısı
	u, _ := url.Parse(baseWS)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+verifyRes.AccessToken)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, resp, err := dialer.DialContext(ctx, u.String(), header)
	if err != nil {
		if resp != nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Printf("✗ WebSocket failed: %v | status=%d | body=%s\n", err, resp.StatusCode, string(b))
		} else {
			fmt.Printf("✗ WebSocket failed: %v\n", err)
		}
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Printf("✓ WebSocket connected. Status: %s\n", resp.Status)

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		fmt.Printf("  WS read timeout (expected): %v\n", err)
	} else {
		fmt.Printf("✓ WS message: %s\n", string(msg))
	}

	fmt.Println()
	fmt.Println("=== ✅ Tüm testler başarılı — Validation + Backend tam çalışıyor ===")
}
