package services

import (
	"context"
	"encoding/base64"
	"time"

	"core-backend/internal/dto"
	"core-backend/internal/rabbitmq"
	"core-backend/internal/repositories"
	"core-backend/pb"
	"core-backend/pkg/logger"

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
}

type messageService struct {
	rabbit     *rabbitmq.Client
	bindingSvc GroupBindingService
	skdmRepo   repositories.SenderKeyDistributionRepository
}

func NewMessageService(
	rabbit *rabbitmq.Client,
	bindingSvc GroupBindingService,
	skdmRepo repositories.SenderKeyDistributionRepository,
) MessageService {
	return &messageService{rabbit: rabbit, bindingSvc: bindingSvc, skdmRepo: skdmRepo}
}

func (s *messageService) DrainInbox(userID, deviceID string, limit int) (*dto.InboxResponse, error) {
	if limit <= 0 {
		limit = defaultDrainLimit
	}
	if limit > maxDrainLimit {
		limit = maxDrainLimit
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	did, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, err
	}

	provisionCtx, provisionCancel := context.WithTimeout(context.Background(), receiptPublishTTL)
	if err := s.bindingSvc.ProvisionDeviceQueue(provisionCtx, uid, did); err != nil {
		provisionCancel()
		return nil, err
	}
	provisionCancel()

	ch, err := s.rabbit.Channel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	// Kalıcı SKDM kayıtlarını queue'ya geri yayınla. Bu çağrı, kuyruk
	// oluşmadan ÖNCE yayınlanmış SKDM'lerin (RabbitMQ tarafından drop edilen)
	// yeni cihazlarda da geçerli olmasını sağlar.
	s.republishStoredSKDMs(ch, uid)

	queueName := rabbitmq.UserDeviceQueueName(userID, deviceID)
	response := &dto.InboxResponse{
		Items: []dto.InboxItem{},
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

		response.Items = append(response.Items, dto.InboxItem{
			Data: base64.StdEncoding.EncodeToString(delivery.Body),
		})

		if payload, ok := wrapper.Content.(*pb.WebSocketMessage_Payload); ok {
			p := payload.Payload
			if err := s.publishDeliveredReceipt(ch, p.MessageId, userID, p.SenderId, p.GroupId); err != nil {
				logger.Log.Warn("auto delivered receipt publish failed",
					zap.String("user_id", userID),
					zap.String("msg_id", p.MessageId), zap.Error(err))
			}
		}

		_ = delivery.Ack(false)
	}

	response.Count = len(response.Items)

	if response.Count == limit {
		peek, ok, err := ch.Get(queueName, false)
		if err != nil {
			return nil, err
		}
		if ok {
			response.HasMore = true
			_ = peek.Nack(false, true)
		}
	}

	logger.Log.Info("inbox drained",
		zap.String("user_id", userID),
		zap.Int("items", response.Count),
		zap.Bool("has_more", response.HasMore))

	return response, nil
}

// republishStoredSKDMs, alıcı için kayıtlı her SKDM'i (sender_user, sender_device,
// group) başına bir GKD frame'i olarak yeniden inşa edip ExchangeUser'a
// yayınlar — kuyruktaki sıraya eklenir, normal inbox drain ile servis edilir.
//
// Hata olursa loglar; replay best-effort'tür ve inbox drain'i bloklamaz.
func (s *messageService) republishStoredSKDMs(ch *amqp.Channel, recipientUUID uuid.UUID) {
	if s.skdmRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), receiptPublishTTL)
	rows, err := s.skdmRepo.ListForRecipient(ctx, recipientUUID)
	cancel()
	if err != nil {
		logger.Log.Warn("SKDM replay list failed",
			zap.String("recipient_user_id", recipientUUID.String()), zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}

	recipientStr := recipientUUID.String()
	for _, row := range rows {
		frame := &pb.WebSocketMessage{
			Content: &pb.WebSocketMessage_Gkd{
				Gkd: &pb.GroupKeyDistribution{
					GroupId:        row.GroupID.String(),
					SenderUserId:   row.SenderUserID.String(),
					SenderDeviceId: row.SenderDeviceID.String(),
					RecipientUserId: recipientStr,
					EncryptedSkdm:  row.EncryptedSKDM,
					Nonce:          row.Nonce,
				},
			},
		}
		body, err := proto.Marshal(frame)
		if err != nil {
			logger.Log.Warn("SKDM replay marshal failed",
				zap.String("group_id", row.GroupID.String()), zap.Error(err))
			continue
		}
		pubCtx, pubCancel := context.WithTimeout(context.Background(), receiptPublishTTL)
		if err := ch.PublishWithContext(pubCtx,
			rabbitmq.ExchangeUser,
			recipientStr,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/octet-stream",
				Body:         body,
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
			},
		); err != nil {
			logger.Log.Warn("SKDM replay publish failed",
				zap.String("recipient_user_id", recipientStr),
				zap.String("group_id", row.GroupID.String()),
				zap.Error(err))
		}
		pubCancel()
	}

	logger.Log.Info("SKDM replay published",
		zap.String("recipient_user_id", recipientStr),
		zap.Int("count", len(rows)))
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
