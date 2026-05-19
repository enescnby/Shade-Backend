package services

import (
	"context"
	"core-backend/internal/rabbitmq"
	"core-backend/internal/repositories"
	"core-backend/pkg/logger"
	"errors"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type GroupBindingService interface {
	ProvisionDeviceQueue(ctx context.Context, userID, deviceID uuid.UUID) error
	BindUserToGroup(ctx context.Context, groupID, userID uuid.UUID) error
	UnbindUserFromGroup(ctx context.Context, groupID, userID uuid.UUID) error
	BindDeviceToAllGroups(ctx context.Context, userID, deviceID uuid.UUID) error
	UnbindAllFromGroup(ctx context.Context, groupID uuid.UUID) error
}

type groupBindingService struct {
	rabbit    *rabbitmq.Client
	userRepo  repositories.UserRepository
	groupRepo repositories.GroupRepository
}

func NewGroupBindingService(
	rabbit *rabbitmq.Client,
	userRepo repositories.UserRepository,
	groupRepo repositories.GroupRepository,
) GroupBindingService {
	return &groupBindingService{
		rabbit:    rabbit,
		userRepo:  userRepo,
		groupRepo: groupRepo,
	}
}

func (s *groupBindingService) ProvisionDeviceQueue(ctx context.Context, userID, deviceID uuid.UUID) error {
	userIDStr := userID.String()
	deviceIDStr := deviceID.String()

	ch, err := s.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := rabbitmq.DeclareUserDeviceQueue(ch, userIDStr, deviceIDStr); err != nil {
		return err
	}

	queueName := rabbitmq.UserDeviceQueueName(userIDStr, deviceIDStr)
	if err := ch.QueueBind(queueName, userIDStr, rabbitmq.ExchangeUser, false, nil); err != nil {
		return err
	}

	if err := s.bindDeviceToAllGroups(ctx, ch, userID, deviceID); err != nil {
		return err
	}

	logger.Log.Info("device queue provisioned",
		zap.String("user_id", userIDStr),
		zap.String("device_id", deviceIDStr))
	return nil
}

func (s *groupBindingService) BindUserToGroup(ctx context.Context, groupID, userID uuid.UUID) error {
	devices, err := s.userRepo.ListDevicesByUserID(userID)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		logger.Log.Info("group bind: no devices yet, skipping",
			zap.String("user_id", userID.String()),
			zap.String("group_id", groupID.String()))
		return nil
	}

	ch, err := s.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	routingKey := rabbitmq.GroupRoutingKey(groupID.String())

	var errs []error
	for _, d := range devices {
		userIDStr := userID.String()
		deviceIDStr := d.DeviceID.String()
		queueName := rabbitmq.UserDeviceQueueName(userIDStr, deviceIDStr)

		if err := rabbitmq.DeclareUserDeviceQueue(ch, userIDStr, deviceIDStr); err != nil {
			logger.Log.Warn("group bind: queue declare failed",
				zap.String("queue", queueName),
				zap.Error(err))
			errs = append(errs, err)
			continue
		}

		if err := ch.QueueBind(queueName, routingKey, rabbitmq.ExchangeGroup, false, nil); err != nil {
			logger.Log.Warn("group bind: QueueBind failed",
				zap.String("queue", queueName),
				zap.String("routing_key", routingKey),
				zap.Error(err))
			errs = append(errs, err)
			continue
		}

		logger.Log.Info("group bind: queue bound",
			zap.String("queue", queueName),
			zap.String("routing_key", routingKey))
	}

	return errors.Join(errs...)
}

func (s *groupBindingService) UnbindUserFromGroup(ctx context.Context, groupID, userID uuid.UUID) error {
	devices, err := s.userRepo.ListDevicesByUserID(userID)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return nil
	}
	ch, err := s.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	routingKey := rabbitmq.GroupRoutingKey(groupID.String())
	var errs []error
	for _, d := range devices {
		queueName := rabbitmq.UserDeviceQueueName(userID.String(), d.DeviceID.String())
		if err := ch.QueueUnbind(queueName, routingKey, rabbitmq.ExchangeGroup, nil); err != nil {
			logger.Log.Warn("group unbind: QueueUnbind failed",
				zap.String("queue", queueName),
				zap.String("routing_key", routingKey),
				zap.Error(err))
			errs = append(errs, err)
			continue
		}
		logger.Log.Info("group unbind: queue unbound",
			zap.String("queue", queueName),
			zap.String("routing_key", routingKey))
	}
	return errors.Join(errs...)
}

func (s *groupBindingService) BindDeviceToAllGroups(ctx context.Context, userID, deviceID uuid.UUID) error {
	ch, err := s.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return s.bindDeviceToAllGroups(ctx, ch, userID, deviceID)
}

func (s *groupBindingService) bindDeviceToAllGroups(ctx context.Context, ch *amqp.Channel, userID, deviceID uuid.UUID) error {
	groups, err := s.groupRepo.ListGroupsForUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	queueName := rabbitmq.UserDeviceQueueName(userID.String(), deviceID.String())
	var errs []error
	for _, g := range groups {
		routingKey := rabbitmq.GroupRoutingKey(g.GroupID.String())
		if err := ch.QueueBind(queueName, routingKey, rabbitmq.ExchangeGroup, false, nil); err != nil {
			logger.Log.Warn("group bind: device-to-group QueueBind failed",
				zap.String("queue", queueName),
				zap.String("routing_key", routingKey),
				zap.Error(err))
			errs = append(errs, err)
			continue
		}
		logger.Log.Info("group bind: device-to-group bound",
			zap.String("queue", queueName),
			zap.String("routing_key", routingKey))
	}
	return errors.Join(errs...)
}
func (s *groupBindingService) UnbindAllFromGroup(ctx context.Context, groupID uuid.UUID) error {
	g, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if len(g.Members) == 0 {
		return nil
	}
	ch, err := s.rabbit.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	routingKey := rabbitmq.GroupRoutingKey(groupID.String())
	var errs []error
	for _, m := range g.Members {
		devices, err := s.userRepo.ListDevicesByUserID(m.UserID)
		if err != nil {
			logger.Log.Warn("group unbind: list devices failed",
				zap.String("user_id", m.UserID.String()),
				zap.Error(err))
			errs = append(errs, err)
			continue
		}
		for _, d := range devices {
			queueName := rabbitmq.UserDeviceQueueName(m.UserID.String(), d.DeviceID.String())
			if err := ch.QueueUnbind(queueName, routingKey, rabbitmq.ExchangeGroup, nil); err != nil {
				logger.Log.Warn("group unbind-all: QueueUnbind failed",
					zap.String("queue", queueName),
					zap.String("routing_key", routingKey),
					zap.Error(err))
				errs = append(errs, err)
				continue
			}
		}
	}
	return errors.Join(errs...)
}
