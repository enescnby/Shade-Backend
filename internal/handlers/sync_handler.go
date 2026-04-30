package handlers

import (
	"core-backend/internal/services"
	wsmanager "core-backend/internal/websocket"
	jwtpkg "core-backend/pkg/jwt"
	"core-backend/pkg/logger"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

const syncSessionTTL = 10 * time.Minute

type SyncHandler struct {
	syncMgr    wsmanager.SyncManager
	sessionSvc services.WebSessionService
}

func NewSyncHandler(syncMgr wsmanager.SyncManager, sessionSvc services.WebSessionService) *SyncHandler {
	return &SyncHandler{syncMgr: syncMgr, sessionSvc: sessionSvc}
}

func (h *SyncHandler) UpgradeAndServe(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		logger.Log.Warn("sync upgrade: not a websocket request", zap.String("session_id", c.Params("session_id")))
		return fiber.ErrUpgradeRequired
	}

	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing token"})
	}

	userID, _, err := jwtpkg.ParseToken(token)
	if err != nil || strings.TrimSpace(userID) == "" {
		logger.Log.Warn("sync upgrade: invalid token", zap.String("session_id", c.Params("session_id")), zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	role := strings.TrimSpace(c.Query("role"))
	if role != wsmanager.RoleAndroid && role != wsmanager.RoleWeb {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be 'android' or 'web'"})
	}

	sessionID := c.Params("session_id")
	logger.Log.Info("sync upgrade: attempt", zap.String("session_id", sessionID), zap.String("role", role), zap.String("user_id", userID))

	session, err := h.sessionSvc.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, services.ErrSessionNotFound) {
			logger.Log.Warn("sync upgrade: session not found", zap.String("session_id", sessionID))
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})
		}
		logger.Log.Error("sync upgrade: get session error", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	if session.Status != "authorized" {
		logger.Log.Warn("sync upgrade: session not authorized", zap.String("session_id", sessionID), zap.String("status", session.Status))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "session not authorized"})
	}

	if session.AuthorizedAt == nil || time.Now().After(session.AuthorizedAt.Add(syncSessionTTL)) {
		logger.Log.Warn("sync upgrade: session expired", zap.String("session_id", sessionID))
		return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "sync session expired"})
	}

	expiresAt := session.AuthorizedAt.Add(syncSessionTTL)
	c.Locals("sessionID", sessionID)
	c.Locals("role", role)
	c.Locals("expiresAt", expiresAt)

	return websocket.New(h.handleConn)(c)
}

func (h *SyncHandler) handleConn(conn *websocket.Conn) {
	sessionID, _ := conn.Locals("sessionID").(string)
	role, _ := conn.Locals("role").(string)
	expiresAt, _ := conn.Locals("expiresAt").(time.Time)

	if err := h.syncMgr.Register(sessionID, role, conn, expiresAt); err != nil {
		logger.Log.Error("failed to register sync connection", zap.Error(err))
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4404, "session not found"))
		return
	}
	defer h.syncMgr.Unregister(sessionID, role, conn)

	h.syncMgr.ReadPump(sessionID, role, conn)
}
