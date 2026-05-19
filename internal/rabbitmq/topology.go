package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeSync  = "shade.sync"
	ExchangeUser  = "shade.user"
	ExchangeGroup = "shade.group"

	SyncQueuePrefix = "sync."
	UserQueuePrefix = "user."

	GroupRoutingPrefix = "group."

	userMessageTTLMs = 7 * 24 * 60 * 60 * 1000
)

func DeclareTopology(client *Client) error {
	ch, err := client.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(
		ExchangeSync,
		amqp.ExchangeDirect,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	if err := ch.ExchangeDeclare(
		ExchangeUser,
		amqp.ExchangeDirect,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	if err := ch.ExchangeDeclare(
		ExchangeGroup,
		amqp.ExchangeTopic,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	return nil
}

func SyncQueueName(sessionID string) string {
	return SyncQueuePrefix + sessionID
}

func UserQueueName(userID string) string {
	return UserQueuePrefix + userID
}

func UserQueueArgs() amqp.Table {
	return amqp.Table{
		"x-message-ttl": int32(userMessageTTLMs),
	}
}

func UserDeviceQueueName(userID, deviceID string) string {
	return UserQueuePrefix + userID + "." + deviceID
}

func GroupRoutingKey(groupID string) string {
	return GroupRoutingPrefix + groupID
}

func DeclareUserDeviceQueue(ch *amqp.Channel, userID, deviceID string) error {
	_, err := ch.QueueDeclare(
		UserDeviceQueueName(userID, deviceID),
		true,
		false,
		false,
		false,
		UserQueueArgs(),
	)
	return err
}
