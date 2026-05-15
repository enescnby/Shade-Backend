package websocket

import (
	"core-backend/pkg/logger"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"go.uber.org/zap"
)

const (
	SyncWriteTimeout = 10 * time.Second
	WebReadyTimeout  = 15 * time.Second
	RoleAndroid      = "android"
	RoleWeb          = "web"
)

type SyncManager interface {
	Register(sessionID, role string, conn *websocket.Conn, expiresAt time.Time) error
	ReadPump(sessionID, role string, conn *websocket.Conn)
	Unregister(sessionID, role string, conn *websocket.Conn)
}

type syncConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type syncSession struct {
	android   *syncConn
	web       *syncConn
	connMu    sync.Mutex
	webReady  chan struct{}
	expiresAt time.Time
}

type syncManager struct {
	sessions map[string]*syncSession
	mu       sync.RWMutex
}

func NewSyncManager() SyncManager {
	return &syncManager{
		sessions: make(map[string]*syncSession),
	}
}

func (m *syncManager) getOrCreate(sessionID string, expiresAt time.Time) *syncSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		return s
	}
	s := &syncSession{
		expiresAt: expiresAt,
		webReady:  make(chan struct{}),
	}
	m.sessions[sessionID] = s
	return s
}

func (m *syncManager) Register(sessionID, role string, conn *websocket.Conn, expiresAt time.Time) error {
	s := m.getOrCreate(sessionID, expiresAt)

	s.connMu.Lock()
	sc := &syncConn{conn: conn}
	switch role {
	case RoleAndroid:
		s.android = sc
	case RoleWeb:
		s.web = sc
		// Signal android that web is ready (non-blocking, idempotent)
		select {
		case <-s.webReady:
		default:
			close(s.webReady)
		}
	}
	s.connMu.Unlock()

	logger.Log.Info("sync connection registered", zap.String("session_id", sessionID), zap.String("role", role))
	return nil
}

func (m *syncManager) ReadPump(sessionID, role string, conn *websocket.Conn) {
	if role == RoleAndroid {
		m.mu.RLock()
		s, ok := m.sessions[sessionID]
		m.mu.RUnlock()

		if ok {
			select {
			case <-s.webReady:
			case <-time.After(WebReadyTimeout):
				logger.Log.Warn("android timed out waiting for web", zap.String("session_id", sessionID))
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(4408, "web client did not connect in time"))
				return
			}
		}
	}

	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			logger.Log.Info("sync read pump closed", zap.String("session_id", sessionID), zap.String("role", role), zap.Error(err))
			break
		}

		logger.Log.Info("sync message received", zap.String("session_id", sessionID), zap.String("role", role), zap.Int("msg_type", msgType), zap.Int("payload_bytes", len(payload)))

		if role != RoleAndroid || (msgType != websocket.TextMessage && msgType != websocket.BinaryMessage) {
			continue
		}

		m.mu.RLock()
		s, ok := m.sessions[sessionID]
		m.mu.RUnlock()

		if !ok {
			break
		}

		if time.Now().After(s.expiresAt) {
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(4410, "session expired"))
			break
		}

		s.connMu.Lock()
		web := s.web
		s.connMu.Unlock()

		if web == nil {
			continue
		}

		web.writeMu.Lock()
		_ = web.conn.SetWriteDeadline(time.Now().Add(SyncWriteTimeout))
		err = web.conn.WriteMessage(msgType, payload)
		_ = web.conn.SetWriteDeadline(time.Time{})
		web.writeMu.Unlock()
		if err != nil {
			logger.Log.Error("sync forward to web failed", zap.String("session_id", sessionID), zap.Error(err))
		} else {
			logger.Log.Info("sync message forwarded to web", zap.String("session_id", sessionID), zap.Int("payload_bytes", len(payload)))
		}
	}
}

func (m *syncManager) Unregister(sessionID, role string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return
	}

	s.connMu.Lock()
	switch role {
	case RoleAndroid:
		if s.android != nil && s.android.conn == conn {
			_ = s.android.conn.Close()
			s.android = nil
		}
	case RoleWeb:
		if s.web != nil && s.web.conn == conn {
			_ = s.web.conn.Close()
			s.web = nil
		}
	}
	bothNil := s.android == nil && s.web == nil
	s.connMu.Unlock()

	if bothNil {
		delete(m.sessions, sessionID)
		logger.Log.Info("sync session cleaned up", zap.String("session_id", sessionID))
	}
}
