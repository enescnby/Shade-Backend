package websocket

import (
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	"core-backend/internal/services"
	"core-backend/pb"
	"core-backend/pkg/logger"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	PingInterval = 15 * time.Second
	PongTimeout  = 10 * time.Second
	WriteTimeout = 10 * time.Second
	AckTimeout   = 10 * time.Second
)

type ConnectionManager interface {
	Register(userID string, conn *websocket.Conn)
	ReadPump(userID string, conn *websocket.Conn)
	Unregister(userID string)
	SendToUser(receiverID string, payload []byte) error
	FlushPendingReceipts(userID string)
}

type clientConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type pendingMessage struct {
	msg   *models.EncryptedMessages
	timer *time.Timer
}

type connectionManager struct {
	clients     map[string]*clientConn
	mu          sync.RWMutex
	pendingAcks map[string]*pendingMessage
	ackMu       sync.Mutex
	msgRepo     repositories.MessageRepository
	userRepo    repositories.UserRepository
	fcmService  services.FCMService
}

func NewConnectionManager(
	msgRepo repositories.MessageRepository,
	userRepo repositories.UserRepository,
	fcmService services.FCMService,
) ConnectionManager {
	return &connectionManager{
		clients:     make(map[string]*clientConn),
		pendingAcks: make(map[string]*pendingMessage),
		msgRepo:     msgRepo,
		userRepo:    userRepo,
		fcmService:  fcmService,
	}
}

func (m *connectionManager) Register(userID string, conn *websocket.Conn) {
	conn.SetReadDeadline(time.Now().Add(PingInterval + PongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(PingInterval + PongTimeout))
		return nil
	})

	m.mu.Lock()
	m.clients[userID] = &clientConn{conn: conn}
	m.mu.Unlock()

	logger.Log.Info("user connected to WebSocket", zap.String("user_id", userID))
}

func (m *connectionManager) startPinger(conn *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(WriteTimeout)); err != nil {
				_ = conn.Close()
				return
			}
		case <-done:
			return
		}
	}
}

func (m *connectionManager) ReadPump(userID string, conn *websocket.Conn) {
	done := make(chan struct{})
	go m.startPinger(conn, done)

	for {
		messageType, rawPayload, err := conn.ReadMessage()
		if err != nil {
			logger.Log.Info("user tunnel disconnected", zap.String("user_id", userID))
			break
		}

		if messageType != websocket.BinaryMessage {
			continue
		}

		var wrapper pb.WebSocketMessage
		if err := proto.Unmarshal(rawPayload, &wrapper); err != nil {
			logger.Log.Error("Protobuf can not decode, broken data", zap.Error(err))
			continue
		}

		switch msg := wrapper.Content.(type) {
		case *pb.WebSocketMessage_Payload:
			payload := msg.Payload
			receiverID := payload.ReceiverId

			err := m.sendWithACK(receiverID, rawPayload, payload)
			if err != nil {
				msgUUID, _ := uuid.Parse(payload.MessageId)
				senderUUID, _ := uuid.Parse(userID)
				receiverUUID, _ := uuid.Parse(payload.ReceiverId)

				offlineMsg := &models.EncryptedMessages{
					MessageID:   msgUUID,
					SenderID:    senderUUID,
					ReceiverID:  receiverUUID,
					Ciphertext:  payload.Ciphertext,
					Nonce:       payload.Nonce,
					MessageType: int(payload.Type),
					Status: []models.DeliveryStatus{
						{
							MessageID:   msgUUID,
							IsDelivered: false,
						},
					},
				}

				saveErr := m.msgRepo.SaveMessage(offlineMsg)
				if saveErr == nil {
					logger.Log.Info("user is offline, message saved", zap.String("msg_id", payload.MessageId))

					device, repoErr := m.userRepo.GetDeviceByUserID(receiverUUID)

					if repoErr == nil && device.FCMToken != "" {
						_ = m.fcmService.SendWakeUpSignal(device.FCMToken)
					} else {
						logger.Log.Warn("FCM Token not found for given ID, WakeUp signal can not send", zap.String("user_id", receiverID))
					}
				}
			}
		case *pb.WebSocketMessage_Receipt:
			receipt := msg.Receipt
			receiverID := receipt.ReceiverId

			if receipt.Status == pb.ReceiptStatus_DELIVERED {
				msgUUID, _ := uuid.Parse(receipt.MessageId)
				_ = m.msgRepo.MarkAsDelivered([]uuid.UUID{msgUUID})
			}

			_ = m.SendToUser(receiverID, rawPayload)

		case *pb.WebSocketMessage_Ack:
			m.handleAck(msg.Ack.MessageId)
		}
	}

	close(done)
}

func (m *connectionManager) sendWithACK(receiverID string, rawPayload []byte, payload *pb.EncryptedPayload) error {
	err := m.SendToUser(receiverID, rawPayload)
	if err != nil {
		return err
	}

	msgUUID, _ := uuid.Parse(payload.MessageId)
	senderUUID, _ := uuid.Parse(payload.SenderId)
	receiverUUID, _ := uuid.Parse(payload.ReceiverId)

	pending := &pendingMessage{
		msg: &models.EncryptedMessages{
			MessageID:   msgUUID,
			SenderID:    senderUUID,
			ReceiverID:  receiverUUID,
			Ciphertext:  payload.Ciphertext,
			Nonce:       payload.Nonce,
			MessageType: int(payload.Type),
			Status:      []models.DeliveryStatus{{MessageID: msgUUID, IsDelivered: false}},
		},
	}

	m.ackMu.Lock()
	pending.timer = time.AfterFunc(AckTimeout, func() { m.onAckTimeout(payload.MessageId) })
	m.pendingAcks[payload.MessageId] = pending
	m.ackMu.Unlock()

	return nil
}

func (m *connectionManager) handleAck(messageID string) {
	m.ackMu.Lock()
	defer m.ackMu.Unlock()
	if p, ok := m.pendingAcks[messageID]; ok {
		p.timer.Stop()
		delete(m.pendingAcks, messageID)
		logger.Log.Debug("ACK received", zap.String("msg_id", messageID))
	}
}

func (m *connectionManager) onAckTimeout(messageID string) {
	m.ackMu.Lock()
	p, ok := m.pendingAcks[messageID]
	if !ok {
		m.ackMu.Unlock()
		return
	}
	delete(m.pendingAcks, messageID)
	m.ackMu.Unlock()

	logger.Log.Warn("ACK timeout, saving as undelivered", zap.String("msg_id", messageID))

	if err := m.msgRepo.SaveMessage(p.msg); err == nil {
		device, err := m.userRepo.GetDeviceByUserID(p.msg.ReceiverID)
		if err == nil && device.FCMToken != "" {
			_ = m.fcmService.SendWakeUpSignal(device.FCMToken)
		}
	}
}

func (m *connectionManager) Unregister(userID string) {
	m.mu.Lock()
	if cc, exists := m.clients[userID]; exists {
		_ = cc.conn.Close()
		delete(m.clients, userID)
	}
	m.mu.Unlock()

	m.flushPendingForUser(userID)

	logger.Log.Info("connection closed, user cleaned from RAM", zap.String("user_id", userID))
}

func (m *connectionManager) flushPendingForUser(userID string) {
	m.ackMu.Lock()
	var toSave []*pendingMessage
	for msgID, p := range m.pendingAcks {
		if p.msg.ReceiverID.String() == userID {
			p.timer.Stop()
			toSave = append(toSave, p)
			delete(m.pendingAcks, msgID)
		}
	}
	m.ackMu.Unlock()

	for _, p := range toSave {
		logger.Log.Info("flushing unACKed message on disconnect", zap.String("msg_id", p.msg.MessageID.String()))
		if err := m.msgRepo.SaveMessage(p.msg); err == nil {
			device, err := m.userRepo.GetDeviceByUserID(p.msg.ReceiverID)
			if err == nil && device.FCMToken != "" {
				_ = m.fcmService.SendWakeUpSignal(device.FCMToken)
			}
		}
	}
}

func (m *connectionManager) SendToUser(receiverID string, payload []byte) error {
	m.mu.RLock()
	cc, exists := m.clients[receiverID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("user %s is offline", receiverID)
	}

	cc.writeMu.Lock()
	cc.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	err := cc.conn.WriteMessage(websocket.BinaryMessage, payload)
	cc.conn.SetWriteDeadline(time.Time{})
	cc.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send message to user %s: %w", receiverID, err)
	}

	logger.Log.Info("Message sent to user", zap.String("to", receiverID))
	return nil
}

func (m *connectionManager) FlushPendingReceipts(userID string) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return
	}

	receipts, err := m.msgRepo.GetPendingReceipts(userUUID)
	if err != nil || len(receipts) == 0 {
		return
	}

	sentIDs := make([]int, 0, len(receipts))
	shadeCache := make(map[uuid.UUID]string)

	for _, receipt := range receipts {
		senderShadeID, ok := shadeCache[receipt.FromUserID]
		if !ok {
			senderUser, err := m.userRepo.GetUserByID(receipt.FromUserID)
			if err != nil {
				continue
			}
			senderShadeID = senderUser.CoreGuardID
			shadeCache[receipt.FromUserID] = senderShadeID
		}

		status, ok := mapReceiptStatus(receipt.Status)
		if !ok {
			continue
		}

		wsReceipt := &pb.DeliveryReceipt{
			MessageId:     receipt.MessageID.String(),
			SenderId:      receipt.FromUserID.String(),
			SenderShadeId: senderShadeID,
			ReceiverId:    userID,
			Status:        status,
			Timestamp:     receipt.Timestamp.UnixMilli(),
		}

		wrapper := &pb.WebSocketMessage{
			Content: &pb.WebSocketMessage_Receipt{
				Receipt: wsReceipt,
			},
		}

		rawPayload, err := proto.Marshal(wrapper)
		if err != nil {
			continue
		}

		if err := m.SendToUser(userID, rawPayload); err != nil {
			continue
		}

		sentIDs = append(sentIDs, receipt.ReceiptID)
	}

	if len(sentIDs) > 0 {
		_ = m.msgRepo.DeletePendingReceipts(sentIDs)
	}
}

func mapReceiptStatus(status string) (pb.ReceiptStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DELIVERED":
		return pb.ReceiptStatus_DELIVERED, true
	case "READ":
		return pb.ReceiptStatus_READ, true
	default:
		return pb.ReceiptStatus_DELIVERED, false
	}
}
