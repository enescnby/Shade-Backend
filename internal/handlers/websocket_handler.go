package handlers

import (
	"strings"

	wsmanager "core-backend/internal/websocket"
	"core-backend/pkg/jwt"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type WebSocketHandler struct {
	cm wsmanager.ConnectionManager
}

func NewWebSocketHandler(cm wsmanager.ConnectionManager) *WebSocketHandler {
	return &WebSocketHandler{cm: cm}
}

func (h *WebSocketHandler) UpgradeAndServe(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}

	token := extractBearerToken(c.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}

	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing token"})
	}

	userID, _, err := jwt.ParseToken(token)
	if err != nil || strings.TrimSpace(userID) == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	c.Locals("userID", userID)

	return websocket.New(h.handleConn)(c)
}

func (h *WebSocketHandler) handleConn(conn *websocket.Conn) {
	userID, _ := conn.Locals("userID").(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		_ = conn.WriteMessage(websocket.CloseMessage, []byte("unauthorized"))
		_ = conn.Close()
		return
	}

	h.cm.Register(userID, conn)
	defer h.cm.Unregister(userID)

	h.cm.ReadPump(userID, conn)
}

func extractBearerToken(authHeader string) string {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}

	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}
