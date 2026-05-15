package middleware

import (
	"core-backend/pkg/logger"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ── Sliding Window Rate Limiter ───────────────────────────────────────────────
//
// Her IP için son N saniyedeki istek zamanlarını tutar.
// Fixed window'dan farklı olarak dakika sınırında burst saldırısına izin vermez.
//
// Örnek — limit=5, window=60s için:
//   Fixed window:  0:59'da 5 istek + 1:00'da 5 istek = 10 istek (burst)
//   Sliding window: herhangi 60 saniyelik pencerede max 5 istek

// RateLimitConfig — her endpoint grubu için ayrı konfigürasyon
type RateLimitConfig struct {
	// Window içinde izin verilen maksimum istek sayısı
	Limit int
	// Sliding window süresi
	Window time.Duration
	// Log mesajlarında görünecek kısa isim
	Name string
}

// Hazır konfigürasyonlar — main.go'da direkt kullanılır
var (
	// Auth endpoint'leri — en sıkı limit
	// Brute force ve credential stuffing'e karşı
	AuthLimitConfig = RateLimitConfig{
		Limit:  10,
		Window: time.Minute,
		Name:   "auth",
	}

	// Medya yükleme — bant genişliği koruması
	MediaLimitConfig = RateLimitConfig{
		Limit:  20,
		Window: time.Minute,
		Name:   "media",
	}

	// Genel API — authenticated endpoint'ler
	APILimitConfig = RateLimitConfig{
		Limit:  120,
		Window: time.Minute,
		Name:   "api",
	}
)

// windowEntry — bir IP'nin istek geçmişi
type windowEntry struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// rateLimiter — tüm IP'lerin pencere kayıtlarını tutar
type rateLimiter struct {
	cfg     RateLimitConfig
	entries sync.Map // map[string]*windowEntry
}

// newRateLimiter — belirtilen konfigürasyonla yeni bir limiter oluşturur
// ve arka planda periyodik temizlik goroutine'i başlatır
func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	rl := &rateLimiter{cfg: cfg}
	go rl.cleanupWorker()
	return rl
}

// record — IP için yeni bir istek kaydeder ve penceredeki toplam sayıyı döner
func (rl *rateLimiter) record(ip string) (count int, oldest time.Time) {
	now := time.Now()
	cutoff := now.Add(-rl.cfg.Window)

	val, _ := rl.entries.LoadOrStore(ip, &windowEntry{})
	entry := val.(*windowEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Pencere dışındaki eski kayıtları temizle
	valid := entry.timestamps[:0]
	for _, t := range entry.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Yeni isteği ekle
	valid = append(valid, now)
	entry.timestamps = valid

	if len(valid) > 0 {
		oldest = valid[0]
	}

	return len(valid), oldest
}

// cleanupWorker — RAM'de biriken atıl IP kayıtlarını periyodik olarak siler
func (rl *rateLimiter) cleanupWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-rl.cfg.Window)
		cleaned := 0

		rl.entries.Range(func(key, val any) bool {
			entry := val.(*windowEntry)
			entry.mu.Lock()
			defer entry.mu.Unlock()

			// Tüm timestamp'ler eskiyse entry'yi sil
			allExpired := true
			for _, t := range entry.timestamps {
				if t.After(cutoff) {
					allExpired = false
					break
				}
			}
			if allExpired {
				rl.entries.Delete(key)
				cleaned++
			}
			return true
		})

		if cleaned > 0 {
			logger.Log.Debug("rate limiter entries cleaned",
				zap.String("limiter", rl.cfg.Name),
				zap.Int("cleaned", cleaned),
			)
		}
	}
}

// Middleware — Fiber handler olarak döner, route group'lara eklenir
func (rl *rateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()

		count, oldest := rl.record(ip)

		// Retry-After hesapla — pencere ne zaman sıfırlanır
		retryAfter := 0
		if count > rl.cfg.Limit && !oldest.IsZero() {
			retryAfter = int(time.Until(oldest.Add(rl.cfg.Window)).Seconds()) + 1
			if retryAfter < 0 {
				retryAfter = 1
			}
		}

		// Rate limit header'larını her response'a ekle (RFC 6585 uyumlu)
		remaining := rl.cfg.Limit - count
		if remaining < 0 {
			remaining = 0
		}

		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.cfg.Limit))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Set("X-RateLimit-Window", rl.cfg.Window.String())

		if count > rl.cfg.Limit {
			logger.Log.Warn("rate limit exceeded",
				zap.String("limiter", rl.cfg.Name),
				zap.String("ip", ip),
				zap.Int("count", count),
				zap.Int("limit", rl.cfg.Limit),
				zap.Int("retry_after_seconds", retryAfter),
			)

			c.Set("Retry-After", fmt.Sprintf("%d", retryAfter))

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":               "too_many_requests",
				"message":             "Rate limit exceeded. Please slow down.",
				"retry_after_seconds": retryAfter,
				"limit":               rl.cfg.Limit,
				"window":              rl.cfg.Window.String(),
			})
		}

		return c.Next()
	}
}

// ── Public factory fonksiyonları — main.go'da kullanılır ─────────────────────

// NewAuthLimiter — /auth/* endpoint'leri için sıkı limiter
func NewAuthLimiter() fiber.Handler {
	return newRateLimiter(AuthLimitConfig).Middleware()
}

// NewMediaLimiter — /media/* endpoint'leri için orta limiter
func NewMediaLimiter() fiber.Handler {
	return newRateLimiter(MediaLimitConfig).Middleware()
}

// NewAPILimiter — genel authenticated endpoint'ler için gevşek limiter
func NewAPILimiter() fiber.Handler {
	return newRateLimiter(APILimitConfig).Middleware()
}
