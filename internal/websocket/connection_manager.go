package websocket

import (
	"context"
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
)

type ConnectionManager interface {
	Register(userID string, conn *websocket.Conn)
	ReadPump(userID string, conn *websocket.Conn)
	Unregister(userID string)
}

type pendingDelivery struct {
	deliveryTag uint64
	timer       *time.Timer
}

type clientState struct {
	conn           *websocket.Conn
	writeMu        sync.Mutex
	consumerCancel context.CancelFunc

	chMu sync.Mutex
	ch   *amqp.Channel

	pendingMu   sync.Mutex
	pendingAcks map[string]*pendingDelivery
}

func (cs *clientState) ack(deliveryTag uint64) error {
	cs.chMu.Lock()
	defer cs.chMu.Unlock()
	if cs.ch == nil {
		return nil
	}
	return cs.ch.Ack(deliveryTag, false)
}

func (cs *clientState) nack(deliveryTag uint64, requeue bool) error {
	cs.chMu.Lock()
	defer cs.chMu.Unlock()
	if cs.ch == nil {
		return nil
	}
	return cs.ch.Nack(deliveryTag, false, requeue)
}

type connectionManager struct {
	clients    map[string]*clientState
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
		clients:    make(map[string]*clientState),
		msgRepo:    msgRepo,
		userRepo:   userRepo,
		fcmService: fcmService,
		rabbit:     rabbit,
	}
}

func (m *connectionManager) declareUserQueue(userID string, ch *amqp.Channel) error {
	queueName := rabbitmq.UserQueueName(userID)
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

func (m *connectionManager) Register(userID string, conn *websocket.Conn) {
	ch, err := m.rabbit.Channel()
	if err != nil {
		logger.Log.Error("user channel open failed",
			zap.String("user_id", userID), zap.Error(err))
		return
	}

	if err := m.declareUserQueue(userID, ch); err != nil {
		logger.Log.Error("user queue declare failed",
			zap.String("user_id", userID), zap.Error(err))
		_ = ch.Close()
		return
	}

	state := &clientState{
		conn:        conn,
		ch:          ch,
		pendingAcks: make(map[string]*pendingDelivery),
	}

	m.mu.Lock()
	if existing, ok := m.clients[userID]; ok {
		go m.cleanupClientState(userID, existing)
	}
	m.clients[userID] = state
	m.mu.Unlock()

	m.setupHeartbeat(userID, state)
	m.startUserConsumer(userID, state)

	logger.Log.Info("user connected to WebSocket", zap.String("user_id", userID))
}

func (m *connectionManager) setupHeartbeat(userID string, state *clientState) {
	_ = state.conn.SetReadDeadline(time.Now().Add(pongWait))
	state.conn.SetPongHandler(func(string) error {
		_ = state.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for range ticker.C {
			state.writeMu.Lock()
			err := state.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(5*time.Second),
			)
			state.writeMu.Unlock()
			if err != nil {
				logger.Log.Info("ping failed, closing",
					zap.String("user_id", userID), zap.Error(err))
				_ = state.conn.Close()
				return
			}
		}
	}()
}

func (m *connectionManager) startUserConsumer(userID string, state *clientState) {
	ctx, cancel := context.WithCancel(context.Background())
	state.consumerCancel = cancel
	go m.consumeUserQueue(ctx, userID, state)
}

func (m *connectionManager) consumeUserQueue(ctx context.Context, userID string, state *clientState) {
	queueName := rabbitmq.UserQueueName(userID)

	state.chMu.Lock()
	deliveries, err := state.ch.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	state.chMu.Unlock()

	if err != nil {
		logger.Log.Error("user consume failed",
			zap.String("user_id", userID), zap.Error(err))
		return
	}

	logger.Log.Info("user consumer started", zap.String("user_id", userID))

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("user consumer cancelled", zap.String("user_id", userID))
			return

		case delivery, ok := <-deliveries:
			if !ok {
				logger.Log.Info("user consumer channel closed", zap.String("user_id", userID))
				return
			}
			m.handleDelivery(userID, state, delivery)
		}
	}
}

func (m *connectionManager) handleDelivery(userID string, state *clientState, delivery amqp.Delivery) {
	var wrapper pb.WebSocketMessage
	if err := proto.Unmarshal(delivery.Body, &wrapper); err != nil {
		logger.Log.Warn("delivery: broken protobuf, dropping",
			zap.String("user_id", userID), zap.Error(err))
		_ = state.nack(delivery.DeliveryTag, false)
		return
	}

	state.writeMu.Lock()
	_ = state.conn.SetWriteDeadline(time.Now().Add(userWriteTimeout))
	writeErr := state.conn.WriteMessage(websocket.BinaryMessage, delivery.Body)
	_ = state.conn.SetWriteDeadline(time.Time{})
	state.writeMu.Unlock()

	if writeErr != nil {
		logger.Log.Error("user message forward failed",
			zap.String("user_id", userID), zap.Error(writeErr))
		_ = state.nack(delivery.DeliveryTag, true)
		_ = state.conn.Close()
		return
	}

	payload, isPayload := wrapper.Content.(*pb.WebSocketMessage_Payload)
	if !isPayload {
		_ = state.ack(delivery.DeliveryTag)
		logger.Log.Info("non-payload forwarded",
			zap.String("user_id", userID),
			zap.Int("payload_bytes", len(delivery.Body)))
		return
	}

	msgID := payload.Payload.MessageId

	state.pendingMu.Lock()
	timer := time.AfterFunc(userAckTimeout, func() {
		state.pendingMu.Lock()
		pending, exists := state.pendingAcks[msgID]
		if exists {
			delete(state.pendingAcks, msgID)
		}
		state.pendingMu.Unlock()

		if exists {
			logger.Log.Warn("ack timeout, requeue + close",
				zap.String("user_id", userID), zap.String("msg_id", msgID))
			_ = state.nack(pending.deliveryTag, true)
			_ = state.conn.Close()
		}
	})
	state.pendingAcks[msgID] = &pendingDelivery{
		deliveryTag: delivery.DeliveryTag,
		timer:       timer,
	}
	state.pendingMu.Unlock()

	logger.Log.Info("message forwarded, awaiting client ack",
		zap.String("user_id", userID), zap.String("msg_id", msgID))
}

func (m *connectionManager) ReadPump(userID string, conn *websocket.Conn) {
	for {
		messageType, rawPayload, err := conn.ReadMessage()
		if err != nil {
			logger.Log.Info("user tunnel disconnected",
				zap.String("user_id", userID), zap.Error(err))
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
			m.handlePayload(userID, msg.Payload, rawPayload)
		case *pb.WebSocketMessage_Receipt:
			m.handleReceipt(userID, msg.Receipt, rawPayload)
		}
	}
}

func (m *connectionManager) handlePayload(senderID string, payload *pb.EncryptedPayload, rawPayload []byte) {
	receiverID := payload.ReceiverId

	ctx, cancel := context.WithTimeout(context.Background(), userPublishTimeout)
	publishErr := m.publishToUser(ctx, receiverID, rawPayload)
	cancel()

	m.mu.RLock()
	_, isOnline := m.clients[receiverID]
	m.mu.RUnlock()

	if !isOnline {
		receiverUUID, _ := uuid.Parse(receiverID)
		if device, deviceErr := m.userRepo.GetDeviceByUserID(receiverUUID); deviceErr == nil && device.FCMToken != "" {
			_ = m.fcmService.SendWakeUpSignal(device.FCMToken)
		} else {
			logger.Log.Warn("FCM Token not found, WakeUp signal skipped",
				zap.String("user_id", receiverID))
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

func (m *connectionManager) handleReceipt(senderID string, receipt *pb.DeliveryReceipt, rawPayload []byte) {
	if receipt.Status == pb.ReceiptStatus_DELIVERED {
		msgUUID, _ := uuid.Parse(receipt.MessageId)
		_ = m.msgRepo.MarkAsDelivered([]uuid.UUID{msgUUID})

		m.mu.RLock()
		state, ok := m.clients[senderID]
		m.mu.RUnlock()

		if ok {
			state.pendingMu.Lock()
			pending, exists := state.pendingAcks[receipt.MessageId]
			if exists {
				pending.timer.Stop()
				delete(state.pendingAcks, receipt.MessageId)
			}
			state.pendingMu.Unlock()

			if exists {
				if err := state.ack(pending.deliveryTag); err != nil {
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
	defer cancel()

	if err := m.publishToUser(ctx, receipt.ReceiverId, rawPayload); err != nil {
		logger.Log.Error("receipt publish failed",
			zap.String("user_id", receipt.ReceiverId), zap.Error(err))
	}
}

func (m *connectionManager) publishToUser(ctx context.Context, receiverID string, payload []byte) error {
	ch, err := m.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

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
		},
	)
}

func (m *connectionManager) Unregister(userID string) {
	m.mu.Lock()
	state, exists := m.clients[userID]
	if !exists {
		m.mu.Unlock()
		return
	}
	delete(m.clients, userID)
	m.mu.Unlock()

	m.cleanupClientState(userID, state)
	logger.Log.Info("connection closed, user cleaned from RAM",
		zap.String("user_id", userID))
}

func (m *connectionManager) cleanupClientState(userID string, state *clientState) {
	if state.consumerCancel != nil {
		state.consumerCancel()
	}

	state.pendingMu.Lock()
	for msgID, pending := range state.pendingAcks {
		pending.timer.Stop()
		if err := state.nack(pending.deliveryTag, true); err != nil {
			logger.Log.Error("requeue on disconnect failed",
				zap.String("user_id", userID),
				zap.String("msg_id", msgID), zap.Error(err))
		}
	}
	state.pendingAcks = nil
	state.pendingMu.Unlock()

	state.chMu.Lock()
	if state.ch != nil {
		_ = state.ch.Close()
		state.ch = nil
	}
	state.chMu.Unlock()

	_ = state.conn.Close()
}
