package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"core-backend/internal/config"
	"core-backend/internal/dto"
	"core-backend/pkg/logger"

	"github.com/gofiber/fiber/v2"
)

// TranslateHandler POST /api/v1/translate
// Android'den gelen metni Gemini ile çevirir; API key sadece backend .env'de tutulur.
func TranslateHandler(c *fiber.Ctx) error {
	var req dto.TranslateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Text) == "" || strings.TrimSpace(req.TargetLang) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "text and target_lang are required"})
	}

	apiKey := config.AppConfig.GeminiAPIKey
	if apiKey == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "translation service not configured"})
	}

	langNames := map[string]string{
		"en": "İngilizce", "de": "Almanca", "fr": "Fransızca",
		"es": "İspanyolca", "ar": "Arapça", "ru": "Rusça",
		"zh": "Çince", "ja": "Japonca", "it": "İtalyanca",
		"pt": "Portekizce", "tr": "Türkçe",
	}
	targetName := req.TargetLang
	if name, ok := langNames[req.TargetLang]; ok {
		targetName = name
	}

	prompt := fmt.Sprintf(
		"Verilen metni %s diline çevir. "+
			"Metin Türkçe kısaltma veya günlük argo içerebilir (örn: 'naslsn' = 'nasılsın', 'naber' = 'ne haber'). "+
			"Bağlamdan anlamı çıkar ve doğal bir çeviri yap. "+
			"Sadece çeviriyi yaz, başka hiçbir şey ekleme: %s",
		targetName, req.Text,
	)

	geminiBody, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	})

	geminiURL := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s",
		apiKey,
	)

	resp, err := http.Post(geminiURL, "application/json", bytes.NewReader(geminiBody))
	if err != nil {
		logger.Log.Sugar().Errorf("Gemini HTTP error: %v", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "translation request failed"})
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		logger.Log.Sugar().Errorf("Gemini non-200 (%d): %s", resp.StatusCode, string(rawBody))
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "translation service error"})
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(rawBody, &geminiResp); err != nil || len(geminiResp.Candidates) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to parse translation response"})
	}

	parts := geminiResp.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "empty translation result"})
	}

	result := strings.TrimSpace(parts[0].Text)
	return c.JSON(dto.TranslateResponse{Result: result})
}
