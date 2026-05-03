package websocket

import (
	"context"
	"strings"
	"sync"
	"time"

	"core-backend/internal/models"
	"core-backend/internal/rabbitmq"
	"core-backend/internal/repositories"
	"core-backend/internal/services"
	"core-backend/pb"
	"core-backend/pkg/logger"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	userPublishTimeout = 5 * time.Second
	userWriteTimeout   = 10 * time.Second
	userAckTimeout     = 30 * time.Second
	pingInterval       = 20 * time.Second
	pongWait           = 30 * time.Second

	headerSourceDeviceID = "x-source-device-id"
)

type ConnectionManager interface {
	Register(userID, deviceID string, conn *websocket.Conn) string
	ReadPump(userID, deviceID, connID string, conn *websocket.Conn)
	Unregister(userID, deviceID, connID string)
}

type pendingDelivery struct {
	deliveryTag uint64
	timer       *time.Timer
}

type connState struct {
	connID  string
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type userState struct {
	userID   string
	deviceID string

	connsMu sync.RWMutex
	conns   map[string]*connState

	consumerCancel context.CancelFunc

	chMu sync.Mutex
	ch   *amqp.Channel

	pendingMu   sync.Mutex
	pendingAcks map[string]*pendingDelivery
}

func sessionKey(userID, deviceID string) string {
	return userID + "|" + deviceID
}

func (us *userState) ack(deliveryTag uint64) error {
	us.chMu.Lock()
	defer us.chMu.Unlock()
	if us.ch == nil {
		return nil
	}
	return us.ch.Ack(deliveryTag, false)
}

func (us *userState) nack(deliveryTag uint64, requeue bool) error {
	us.chMu.Lock()
	defer us.chMu.Unlock()
	if us.ch == nil {
		return nil
	}
	return us.ch.Nack(deliveryTag, false, requeue)
}

type connectionManager struct {
	clients    map[string]*userState
	mu         sync.RWMutex
	msgRepo    repositories.MessageRepository
	userRepo   repositories.UserRepository
	fcmService services.FCMService
	rabbit     *rabbitmq.Client
}

func NewConnectionManager(
	msgRepo repositories.MessageRepository,
	userRepo repositories.UserRepository,
	fcmService services.FCMService,
	rabbit *rabbitmq.Client,
) ConnectionManager {
	return &connectionManager{
		clients:    make(map[string]*userState),
		msgRepo:    msgRepo,
		userRepo:   userRepo,
		fcmService: fcmService,
		rabbit:     rabbit,
	}
}

func (m *connectionManager) userHasAnyOnlineSession(userID string) bool {
	prefix := userID + "|"
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k := range m.clients {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func (m *connectionManager) declareUserQueue(userID, deviceID string, ch *amqp.Channel) error {
	queueName := rabbitmq.UserDeviceQueueName(userID, deviceID)
	if _, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		rabbitmq.UserQueueArgs(),
	); err != nil {
		return err
	}
	return ch.QueueBind(queueName, userID, rabbitmq.ExchangeUser, false, nil)
}

func (m *connectionManager) Register(userID, deviceID string, conn *websocket.Conn) string {
	connID := uuid.NewString()
	cs := &connState{connID: connID, conn: conn}

	key := sessionKey(userID, deviceID)

	m.mu.Lock()
	us, ok := m.clients[key]
	if !ok {
		ch, err := m.rabbit.Channel()
		if err != nil {
			m.mu.Unlock()
			logger.Log.Error("user channel open failed",
				zap.String("user_id", userID),
				zap.String("device_id", deviceID),
				zap.Error(err))
			_ = conn.Close()
			return connID
		}

		if err := m.declareUserQueue(userID, deviceID, ch); err != nil {
			m.mu.Unlock()
			logger.Log.Error("user queue declare failed",
				zap.String("user_id", userID),
				zap.String("device_id", deviceID),
				zap.Error(err))
			_ = ch.Close()
			_ = conn.Close()
			return connID
		}

		us = &userState{
			userID:      userID,
			deviceID:    deviceID,
			conns:       make(map[string]*connState),
			ch:          ch,
			pendingAcks: make(map[string]*pendingDelivery),
		}
		m.clients[key] = us
		m.startUserConsumer(us)
	}

	us.connsMu.Lock()
	us.conns[connID] = cs
	deviceCount := len(us.conns)
	us.connsMu.Unlock()
	m.mu.Unlock()

	m.setupHeartbeat(userID, deviceID, cs)

	logger.Log.Info("user device connected",
		zap.String("user_id", userID),
		zap.String("device_id", deviceID),
		zap.String("conn_id", connID),
		zap.Int("device_count", deviceCount))
	return connID
}

func (m *connectionManager) setupHeartbeat(userID, deviceID string, cs *connState) {
	_ = cs.conn.SetReadDeadline(time.Now().Add(pongWait))
	cs.conn.SetPongHandler(func(string) error {
		_ = cs.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for range ticker.C {
			cs.writeMu.Lock()
			err := cs.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(5*time.Second),
			)
			cs.writeMu.Unlock()
			if err != nil {
				logger.Log.Info("ping failed, closing",
					zap.String("user_id", userID),
					zap.String("device_id", deviceID),
					zap.String("conn_id", cs.connID),
					zap.Error(err))
				_ = cs.conn.Close()
				return
			}
		}
	}()
}

func (m *connectionManager) startUserConsumer(us *userState) {
	ctx, cancel := context.WithCancel(context.Background())
	us.consumerCancel = cancel
	go m.consumeUserQueue(ctx, us)
}

func (m *connectionManager) consumeUserQueue(ctx context.Context, us *userState) {
	queueName := rabbitmq.UserDeviceQueueName(us.userID, us.deviceID)

	us.chMu.Lock()
	ch := us.ch
	us.chMu.Unlock()
	if ch == nil {
		return
	}

	deliveries, err := ch.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.Log.Error("user consume failed",
			zap.String("user_id", us.userID), zap.Error(err))
		return
	}

	logger.Log.Info("user consumer started", zap.String("user_id", us.userID))

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("user consumer cancelled", zap.String("user_id", us.userID))
			return

		case delivery, ok := <-deliveries:
			if !ok {
				logger.Log.Info("user consumer channel closed", zap.String("user_id", us.userID))
				return
			}
			m.handleDelivery(us, delivery)
		}
	}
}

func (m *connectionManager) handleDelivery(us *userState, delivery amqp.Delivery) {
	var wrapper pb.WebSocketMessage
	if err := proto.Unmarshal(delivery.Body, &wrapper); err != nil {
		logger.Log.Warn("delivery: broken protobuf, dropping",
			zap.String("user_id", us.userID), zap.Error(err))
		_ = us.nack(delivery.DeliveryTag, false)
		return
	}

	skipDeviceID := ""
	if delivery.Headers != nil {
		if v, ok := delivery.Headers[headerSourceDeviceID]; ok {
			if s, ok := v.(string); ok {
				skipDeviceID = strings.TrimSpace(s)
			}
		}
	}
	if skipDeviceID != "" && skipDeviceID == us.deviceID {
		_ = us.ack(delivery.DeliveryTag)
		return
	}

	us.connsMu.RLock()
	targets := make([]*connState, 0, len(us.conns))
	for _, cs := range us.conns {
		targets = append(targets, cs)
	}
	us.connsMu.RUnlock()

	if len(targets) == 0 {
		_ = us.nack(delivery.DeliveryTag, true)
		return
	}

	deliveredTo := 0
	for _, cs := range targets {
		cs.writeMu.Lock()
		_ = cs.conn.SetWriteDeadline(time.Now().Add(userWriteTimeout))
		writeErr := cs.conn.WriteMessage(websocket.BinaryMessage, delivery.Body)
		_ = cs.conn.SetWriteDeadline(time.Time{})
		cs.writeMu.Unlock()

		if writeErr == nil {
			deliveredTo++
			continue
		}

		logger.Log.Error("user message forward failed",
			zap.String("user_id", us.userID),
			zap.String("conn_id", cs.connID),
			zap.Error(writeErr))
		_ = cs.conn.Close()
	}

	if deliveredTo == 0 {
		_ = us.nack(delivery.DeliveryTag, true)
		return
	}

	payload, isPayload := wrapper.Content.(*pb.WebSocketMessage_Payload)
	if !isPayload {
		_ = us.ack(delivery.DeliveryTag)
		logger.Log.Info("non-payload forwarded",
			zap.String("user_id", us.userID),
			zap.Int("recipients", deliveredTo))
		return
	}

	msgID := payload.Payload.MessageId

	us.pendingMu.Lock()
	timer := time.AfterFunc(userAckTimeout, func() {
		us.pendingMu.Lock()
		pending, exists := us.pendingAcks[msgID]
		if exists {
			delete(us.pendingAcks, msgID)
		}
		us.pendingMu.Unlock()

		if exists {
			logger.Log.Warn("ack timeout, requeue",
				zap.String("user_id", us.userID),
				zap.String("msg_id", msgID))
			_ = us.nack(pending.deliveryTag, true)
		}
	})
	us.pendingAcks[msgID] = &pendingDelivery{
		deliveryTag: delivery.DeliveryTag,
		timer:       timer,
	}
	us.pendingMu.Unlock()

	logger.Log.Info("message forwarded, awaiting client ack",
		zap.String("user_id", us.userID),
		zap.String("msg_id", msgID),
		zap.Int("recipients", deliveredTo))
}

func (m *connectionManager) ReadPump(userID, deviceID, connID string, conn *websocket.Conn) {
	for {
		messageType, rawPayload, err := conn.ReadMessage()
		if err != nil {
			logger.Log.Info("user tunnel disconnected",
				zap.String("user_id", userID),
				zap.String("conn_id", connID),
				zap.Error(err))
			break
		}

		_ = conn.SetReadDeadline(time.Now().Add(pongWait))

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
			m.handlePayload(userID, deviceID, msg.Payload, rawPayload)
		case *pb.WebSocketMessage_Receipt:
			m.handleReceipt(userID, deviceID, msg.Receipt, rawPayload)
		}
	}
}

func (m *connectionManager) handlePayload(senderID, senderDeviceID string, payload *pb.EncryptedPayload, rawPayload []byte) {
	receiverID := payload.ReceiverId

	ctx, cancel := context.WithTimeout(context.Background(), userPublishTimeout)
	publishErr := m.publishToUser(ctx, receiverID, rawPayload, "")
	cancel()

	echoCtx, echoCancel := context.WithTimeout(context.Background(), userPublishTimeout)
	if echoErr := m.publishToUser(echoCtx, senderID, rawPayload, senderDeviceID); echoErr != nil {
		logger.Log.Warn("multi-device echo publish failed",
			zap.String("user_id", senderID), zap.Error(echoErr))
	}
	echoCancel()

	if !m.userHasAnyOnlineSession(receiverID) {
		receiverUUID, err := uuid.Parse(receiverID)
		if err == nil {
			devices, err := m.userRepo.ListDevicesByUserID(receiverUUID)
			if err != nil {
				logger.Log.Warn("list devices for FCM failed",
					zap.String("user_id", receiverID), zap.Error(err))
			} else {
				for _, d := range devices {
					if d.FCMToken != "" {
						_ = m.fcmService.SendWakeUpSignal(d.FCMToken)
					}
				}
			}
		}
	}

	if publishErr != nil {
		logger.Log.Warn("rabbitmq publish failed, falling back to DB",
			zap.String("user_id", receiverID), zap.Error(publishErr))

		msgUUID, _ := uuid.Parse(payload.MessageId)
		senderUUID, _ := uuid.Parse(senderID)
		receiverUUID, _ := uuid.Parse(receiverID)
		offlineMsg := &models.EncryptedMessages{
			MessageID:   msgUUID,
			SenderID:    senderUUID,
			ReceiverID:  receiverUUID,
			Ciphertext:  payload.Ciphertext,
			Nonce:       payload.Nonce,
			MessageType: int(payload.Type),
		}
		if saveErr := m.msgRepo.SaveMessage(offlineMsg); saveErr != nil {
			logger.Log.Error("DB fallback save failed",
				zap.String("msg_id", payload.MessageId), zap.Error(saveErr))
		}
	}
}

func (m *connectionManager) handleReceipt(senderID, senderDeviceID string, receipt *pb.DeliveryReceipt, rawPayload []byte) {
	if receipt.Status == pb.ReceiptStatus_DELIVERED {
		msgUUID, _ := uuid.Parse(receipt.MessageId)
		_ = m.msgRepo.MarkAsDelivered([]uuid.UUID{msgUUID})

		m.mu.RLock()
		us, ok := m.clients[sessionKey(senderID, senderDeviceID)]
		m.mu.RUnlock()

		if ok {
			us.pendingMu.Lock()
			pending, exists := us.pendingAcks[receipt.MessageId]
			if exists {
				pending.timer.Stop()
				delete(us.pendingAcks, receipt.MessageId)
			}
			us.pendingMu.Unlock()

			if exists {
				if err := us.ack(pending.deliveryTag); err != nil {
					logger.Log.Error("rabbitmq ack failed",
						zap.String("user_id", senderID),
						zap.String("msg_id", receipt.MessageId), zap.Error(err))
				} else {
					logger.Log.Info("message acked after delivered receipt",
						zap.String("user_id", senderID),
						zap.String("msg_id", receipt.MessageId))
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), userPublishTimeout)
	if err := m.publishToUser(ctx, receipt.ReceiverId, rawPayload, ""); err != nil {
		logger.Log.Error("receipt publish failed",
			zap.String("user_id", receipt.ReceiverId), zap.Error(err))
	}
	cancel()

	echoCtx, echoCancel := context.WithTimeout(context.Background(), userPublishTimeout)
	if err := m.publishToUser(echoCtx, senderID, rawPayload, senderDeviceID); err != nil {
		logger.Log.Warn("multi-device receipt echo failed",
			zap.String("user_id", senderID), zap.Error(err))
	}
	echoCancel()
}

func (m *connectionManager) publishToUser(ctx context.Context, receiverID string, payload []byte, sourceConnID string) error {
	ch, err := m.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	var headers amqp.Table
	if sourceConnID != "" {
		headers = amqp.Table{headerSourceDeviceID: sourceConnID}
	}

	return ch.PublishWithContext(ctx,
		rabbitmq.ExchangeUser,
		receiverID,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			Body:         payload,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Headers:      headers,
		},
	)
}

func (m *connectionManager) Unregister(userID, deviceID, connID string) {
	key := sessionKey(userID, deviceID)

	m.mu.Lock()
	us, exists := m.clients[key]
	if !exists {
		m.mu.Unlock()
		return
	}

	us.connsMu.Lock()
	cs, ok := us.conns[connID]
	if ok {
		delete(us.conns, connID)
	}
	remaining := len(us.conns)
	us.connsMu.Unlock()

	lastConn := remaining == 0
	if lastConn {
		delete(m.clients, key)
	}
	m.mu.Unlock()

	if ok {
		_ = cs.conn.Close()
	}

	if lastConn {
		m.cleanupUserState(us)
		logger.Log.Info("user fully disconnected, cleaned from RAM",
			zap.String("user_id", userID),
			zap.String("device_id", deviceID),
			zap.String("conn_id", connID))
		return
	}

	logger.Log.Info("user device disconnected, others still online",
		zap.String("user_id", userID),
		zap.String("conn_id", connID),
		zap.String("device_id", deviceID),
		zap.Int("remaining_devices", remaining))
}

func (m *connectionManager) cleanupUserState(us *userState) {
	if us.consumerCancel != nil {
		us.consumerCancel()
	}

	us.pendingMu.Lock()
	for _, pending := range us.pendingAcks {
		pending.timer.Stop()
	}
	us.pendingAcks = nil
	us.pendingMu.Unlock()

	us.chMu.Lock()
	if us.ch != nil {
		_ = us.ch.Close()
		us.ch = nil
	}
	us.chMu.Unlock()
}
