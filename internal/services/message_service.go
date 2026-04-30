package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/rabbitmq"
	"core-backend/pb"
	"core-backend/pkg/logger"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	defaultDrainLimit = 100
	maxDrainLimit     = 500
	receiptPublishTTL = 5 * time.Second
)

type MessageService interface {
	DrainInbox(userID string, limit int) (*dto.InboxResponse, error)
}

type messageService struct {
	rabbit *rabbitmq.Client
}

func NewMessageService(rabbit *rabbitmq.Client) MessageService {
	return &messageService{rabbit: rabbit}
}

func (s *messageService) DrainInbox(userID string, limit int) (*dto.InboxResponse, error) {
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

	queueName := rabbitmq.UserQueueName(userID)
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
				Ciphertext:  p.Ciphertext,
				Nonce:       p.Nonce,
				MessageType: int32(p.Type),
				Timestamp:   delivery.Timestamp.Unix(),
			})

			if err := s.publishDeliveredReceipt(ch, p.MessageId, userID, p.SenderId); err != nil {
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

func (s *messageService) publishDeliveredReceipt(ch *amqp.Channel, msgID, fromUserID, toSenderID string) error {
	receipt := &pb.WebSocketMessage{
		Content: &pb.WebSocketMessage_Receipt{
			Receipt: &pb.DeliveryReceipt{
				MessageId:  msgID,
				SenderId:   fromUserID,
				ReceiverId: toSenderID,
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
