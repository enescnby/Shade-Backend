package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/rabbitmq"
	"core-backend/internal/repositories"
	"core-backend/pb"
	"core-backend/pkg/logger"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	defaultDrainLimit = 100
	maxDrainLimit     = 500
	receiptPublishTTL = 5 * time.Second
)

type MessageService interface {
	DrainInbox(userID, deviceID string, limit int) (*dto.InboxResponse, error)
	// SendReceipts WebSocket gönderilemediğinde REST fallback olarak çağrılır.
	// Her makbuz için orijinal mesaj gönderenine RabbitMQ üzerinden iletilir.
	// Best-effort: hata varsa loglanır, caller'a 200 dönülür.
	SendReceipts(ctx context.Context, fromUserID, fromShadeID string, receipts []dto.ReceiptRequest) error
}

type messageService struct {
	rabbit  *rabbitmq.Client
	msgRepo repositories.MessageRepository
}

func NewMessageService(rabbit *rabbitmq.Client, msgRepo repositories.MessageRepository) MessageService {
	return &messageService{rabbit: rabbit, msgRepo: msgRepo}
}

func (s *messageService) DrainInbox(userID, deviceID string, limit int) (*dto.InboxResponse, error) {
	if limit <= 0 {
		limit = defaultDrainLimit
	}
	if limit > maxDrainLimit {
		limit = maxDrainLimit
	}

	ch, err := s.rabbit.Channel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	queueName := rabbitmq.UserDeviceQueueName(userID, deviceID)
	response := &dto.InboxResponse{
		Messages: []dto.InboxMessage{},
		Receipts: []dto.InboxReceipt{},
	}

	for i := 0; i < limit; i++ {
		delivery, ok, err := ch.Get(queueName, false)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}

		var wrapper pb.WebSocketMessage
		if err := proto.Unmarshal(delivery.Body, &wrapper); err != nil {
			logger.Log.Warn("inbox: broken protobuf, dropping",
				zap.String("user_id", userID), zap.Error(err))
			_ = delivery.Nack(false, false)
			continue
		}

		switch content := wrapper.Content.(type) {
		case *pb.WebSocketMessage_Payload:
			p := content.Payload
			response.Messages = append(response.Messages, dto.InboxMessage{
				MessageID:   p.MessageId,
				SenderID:    p.SenderId,
				ReceiverID:  p.ReceiverId,
				GroupID:     p.GroupId,
				Ciphertext:  p.Ciphertext,
				Nonce:       p.Nonce,
				MessageType: int32(p.Type),
				Timestamp:   delivery.Timestamp.Unix(),
			})
			if err := s.publishDeliveredReceipt(ch, p.MessageId, userID, p.SenderId, p.GroupId); err != nil {
				logger.Log.Warn("auto delivered receipt publish failed",
					zap.String("user_id", userID),
					zap.String("msg_id", p.MessageId), zap.Error(err))
			}

		case *pb.WebSocketMessage_Receipt:
			r := content.Receipt
			response.Receipts = append(response.Receipts, dto.InboxReceipt{
				MessageID:  r.MessageId,
				SenderID:   r.SenderId,
				ReceiverID: r.ReceiverId,
				GroupID:    r.GroupId,
				Status:     r.Status.String(),
				Timestamp:  delivery.Timestamp.Unix(),
			})
		}

		_ = delivery.Ack(false)
	}

	logger.Log.Info("inbox drained",
		zap.String("user_id", userID),
		zap.Int("messages", len(response.Messages)),
		zap.Int("receipts", len(response.Receipts)))

	return response, nil
}

// SendReceipts — her makbuz için orijinal mesaj sahibine receipt iletir.
func (s *messageService) SendReceipts(ctx context.Context, fromUserID, fromShadeID string, receipts []dto.ReceiptRequest) error {
	if len(receipts) == 0 {
		return nil
	}

	receiverUUID, err := uuid.Parse(fromUserID)
	if err != nil {
		return err
	}

	// Tüm messageId'leri parse et
	var msgUUIDs []uuid.UUID
	validReceipts := make([]dto.ReceiptRequest, 0, len(receipts))
	for _, r := range receipts {
		id, err := uuid.Parse(r.MessageID)
		if err != nil {
			continue
		}
		msgUUIDs = append(msgUUIDs, id)
		validReceipts = append(validReceipts, r)
	}
	if len(msgUUIDs) == 0 {
		return nil
	}

	// Orijinal mesajları DB'den çek — sender_id'yi bulmak için
	msgs, err := s.msgRepo.GetMessagesByIDsForReceiver(ctx, receiverUUID, msgUUIDs)
	if err != nil {
		logger.Log.Warn("SendReceipts: DB lookup failed", zap.Error(err))
		return nil // best-effort
	}

	// messageId → senderID haritası
	senderMap := make(map[string]string, len(msgs))
	for _, m := range msgs {
		senderMap[m.MessageID.String()] = m.SenderID.String()
	}

	ch, err := s.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	for _, r := range validReceipts {
		originalSenderID, ok := senderMap[r.MessageID]
		if !ok {
			// Mesaj DB'de bulunamadı (grup mesajı veya eski kayıt) — atla
			continue
		}

		status := pb.ReceiptStatus_READ
		if r.Status == "DELIVERED" {
			status = pb.ReceiptStatus_DELIVERED
		}

		wrapped := &pb.WebSocketMessage{
			Content: &pb.WebSocketMessage_Receipt{
				Receipt: &pb.DeliveryReceipt{
					MessageId:     r.MessageID,
					SenderId:      fromUserID,
					SenderShadeId: fromShadeID,
					ReceiverId:    originalSenderID,
					Status:        status,
					Timestamp:     time.Now().UnixMilli(),
				},
			},
		}

		body, err := proto.Marshal(wrapped)
		if err != nil {
			logger.Log.Warn("SendReceipts: proto marshal failed", zap.String("msg_id", r.MessageID), zap.Error(err))
			continue
		}

		pubCtx, cancel := context.WithTimeout(ctx, receiptPublishTTL)
		err = ch.PublishWithContext(pubCtx,
			rabbitmq.ExchangeUser,
			originalSenderID, // routing key = orijinal gönderen
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/octet-stream",
				Body:         body,
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
			},
		)
		cancel()

		if err != nil {
			logger.Log.Warn("SendReceipts: publish failed",
				zap.String("msg_id", r.MessageID),
				zap.String("to", originalSenderID),
				zap.Error(err))
		}
	}

	return nil
}

func (s *messageService) publishDeliveredReceipt(ch *amqp.Channel, msgID, fromUserID, toSenderID, groupID string) error {
	receipt := &pb.WebSocketMessage{
		Content: &pb.WebSocketMessage_Receipt{
			Receipt: &pb.DeliveryReceipt{
				MessageId:  msgID,
				SenderId:   fromUserID,
				ReceiverId: toSenderID,
				GroupId:    groupID,
				Status:     pb.ReceiptStatus_DELIVERED,
				Timestamp:  time.Now().UnixMilli(),
			},
		},
	}

	body, err := proto.Marshal(receipt)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), receiptPublishTTL)
	defer cancel()

	return ch.PublishWithContext(ctx,
		rabbitmq.ExchangeUser,
		toSenderID,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
}
