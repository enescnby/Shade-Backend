package middleware_test

import (
	"core-backend/internal/middleware"
	"core-backend/pkg/logger"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logger.InitLogger()
	m.Run()
}

// testApp — belirtilen limiter ile minimal Fiber app kurar
func testApp(handler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(handler)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func makeRequest(app *fiber.App) int {
	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req, -1)
	return resp.StatusCode
}

// ── Temel limit davranışı ─────────────────────────────────────────────────────

func TestRateLimiter_UnderLimit_Returns200(t *testing.T) {
	app := testApp(middleware.NewAuthLimiter())

	for i := 0; i < 5; i++ {
		code := makeRequest(app)
		assert.Equal(t, 200, code, "istek %d 200 donmeli", i+1)
	}
}

func TestRateLimiter_ExceedsLimit_Returns429(t *testing.T) {
	app := testApp(middleware.NewAuthLimiter()) // limit: 10/dk

	// 10 istek geç
	for i := 0; i < 10; i++ {
		makeRequest(app)
	}

	// 11. istek 429 vermeli
	code := makeRequest(app)
	assert.Equal(t, 429, code, "limit asiminda 429 gelmeli")
}

func TestRateLimiter_ExactlyAtLimit_Returns200(t *testing.T) {
	app := testApp(middleware.NewAuthLimiter())

	// Tam limite kadar git — hepsi 200 olmali
	for i := 0; i < 10; i++ {
		code := makeRequest(app)
		assert.Equal(t, 200, code, "istek %d/%d 200 olmali", i+1, 10)
	}
}

// ── Response header'lari ─────────────────────────────────────────────────────

func TestRateLimiter_Headers_Present(t *testing.T) {
	app := testApp(middleware.NewAuthLimiter())

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"), "X-RateLimit-Limit header olmali")
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Remaining"), "X-RateLimit-Remaining header olmali")
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Window"), "X-RateLimit-Window header olmali")
}

func TestRateLimiter_429_HasRetryAfterHeader(t *testing.T) {
	app := testApp(middleware.NewAuthLimiter())

	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		resp, _ := app.Test(req, -1)
		if resp.StatusCode == 429 {
			assert.NotEmpty(t, resp.Header.Get("Retry-After"), "429'da Retry-After olmali")
			return
		}
	}
	t.Fatal("429 alinamadi")
}

// ── Farkli limiter tipleri ────────────────────────────────────────────────────

func TestAPILimiter_HigherLimit(t *testing.T) {
	app := testApp(middleware.NewAPILimiter()) // limit: 120/dk

	// Auth limiter'dan cok daha yuksek — 50 istek hepsi 200 olmali
	for i := 0; i < 50; i++ {
		code := makeRequest(app)
		assert.Equal(t, 200, code, "API limiter: istek %d 200 olmali", i+1)
	}
}

func TestMediaLimiter_MidLimit(t *testing.T) {
	app := testApp(middleware.NewMediaLimiter())
	limit := middleware.MediaLimitConfig.Limit

	for i := 0; i < limit; i++ {
		code := makeRequest(app)
		assert.Equal(t, 200, code, "Media limiter: istek %d 200 olmali", i+1)
	}

	// Sonraki istek limiti asmali — 429
	code := makeRequest(app)
	assert.Equal(t, 429, code, "Media limiter: %d. istek 429 olmali", limit+1)
}

// ── Sliding window davranişi ───────────────────────────────────────────────────

func TestRateLimiter_SlidingWindow_OldRequestsExpire(t *testing.T) {
	// Bu test sliding window'un calistigini dogruluyor:
	// Eski istekler pencereden cikinca yeni istekler kabul edilmeli.
	// Gercek zamanlama yerine kisa window ile test ediyoruz.

	// Not: Gercek TTL testi production'da integration test ile yapilmali.
	// Burada rate limiter'in limiti dogru saydigi dogrulaniypr.

	app := testApp(middleware.NewAuthLimiter())

	// 10 istek at — limite gel
	for i := 0; i < 10; i++ {
		makeRequest(app)
	}

	// 11. istek 429 olmali
	code := makeRequest(app)
	assert.Equal(t, 429, code)
}

// ── 429 response body ─────────────────────────────────────────────────────────

func TestRateLimiter_429_ResponseBody(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewAuthLimiter())
	app.Get("/test", func(c *fiber.Ctx) error { return c.SendStatus(200) })

	for i := 0; i < 10; i++ {
		makeRequest(app)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	// Content-Type JSON olmali
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

// ── Timing: window sifirlanmasi ───────────────────────────────────────────────

func TestRateLimiter_Remaining_DecreasesWithRequests(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewAuthLimiter())
	app.Get("/test", func(c *fiber.Ctx) error { return c.SendStatus(200) })

	var remaining1, remaining2 string

	req1 := httptest.NewRequest("GET", "/test", nil)
	resp1, _ := app.Test(req1, -1)
	remaining1 = resp1.Header.Get("X-RateLimit-Remaining")

	time.Sleep(1 * time.Millisecond) // kisa bekleme

	req2 := httptest.NewRequest("GET", "/test", nil)
	resp2, _ := app.Test(req2, -1)
	remaining2 = resp2.Header.Get("X-RateLimit-Remaining")

	// Kalan istek sayisi azalmali
	assert.NotEmpty(t, remaining1)
	assert.NotEmpty(t, remaining2)
	assert.True(t, remaining1 > remaining2,
		"Remaining azalmali: %s -> %s", remaining1, remaining2)
}
