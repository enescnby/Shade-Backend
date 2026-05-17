package services

import (
	"context"
	"core-backend/internal/rabbitmq"
	"core-backend/pb"
	"core-backend/pkg/logger"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// GroupEventPublisher, grup üyelik değişikliklerini topic exchange üzerinden
// tüm üyelere yayınlar. İstemciler bu event'leri sender key rotation
// tetikleyicisi olarak kullanır (REMOVED → kalan üyeler kendi sender
// key'lerini rotate eder; JOINED → mevcut üyeler yeni üyeye SKDM gönderir).
//
// GroupBindingService'ten ayrı bir servis tutuyoruz çünkü sorumluluklar
// farklı: binding broker routing state'i, event ise mesaj akışı.
type GroupEventPublisher interface {
	PublishMembershipEvent(ctx context.Context, event *pb.GroupMembershipEvent) error
}

type groupEventPublisher struct {
	rabbit *rabbitmq.Client
}

func NewGroupEventPublisher(rabbit *rabbitmq.Client) GroupEventPublisher {
	return &groupEventPublisher{rabbit: rabbit}
}

func (p *groupEventPublisher) PublishMembershipEvent(ctx context.Context, event *pb.GroupMembershipEvent) error {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UnixMilli()
	}

	wrapper := &pb.WebSocketMessage{
		Content: &pb.WebSocketMessage_Gme{Gme: event},
	}

	body, err := proto.Marshal(wrapper)
	if err != nil {
		return err
	}

	ch, err := p.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.PublishWithContext(ctx,
		rabbitmq.ExchangeGroup,
		rabbitmq.GroupRoutingKey(event.GroupId),
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	); err != nil {
		logger.Log.Warn("group event publish failed",
			zap.String("group_id", event.GroupId),
			zap.String("kind", event.Kind.String()),
			zap.String("subject_id", event.SubjectId),
			zap.Error(err))
		return err
	}

	logger.Log.Info("group event published",
		zap.String("group_id", event.GroupId),
		zap.String("kind", event.Kind.String()),
		zap.String("subject_id", event.SubjectId),
		zap.String("actor_id", event.ActorId))
	return nil
}
