package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeSync = "shade.sync"
	ExchangeUser = "shade.user"

	SyncQueuePrefix = "sync."
	UserQueuePrefix = "user."

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
