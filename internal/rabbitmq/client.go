package rabbitmq

import (
	"context"
	"core-backend/internal/config"
	"core-backend/pkg/logger"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	reconnectDelay = 5 * time.Second
)

type Client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
	mu      sync.RWMutex
	closed  bool
	done    chan struct{}
}

func NewClient(cfg config.Config) (*Client, error) {
	url := fmt.Sprintf("amqp://%s:%s@%s:%s%s",
		cfg.RabbitMQUser,
		cfg.RabbitMQPassword,
		cfg.RabbitMQHost,
		cfg.RabbitMQPort,
		cfg.RabbitMQVHost,
	)

	c := &Client{
		url:  url,
		done: make(chan struct{}),
	}

	if err := c.connect(); err != nil {
		return nil, fmt.Errorf("rabbitmq initial connect: %w", err)
	}

	go c.handleReconnect()

	logger.Log.Info("rabbitmq connected", zap.String("host", cfg.RabbitMQHost))
	return c, nil
}

func (c *Client) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.channel = ch
	c.mu.Unlock()
	return nil
}

func (c *Client) handleReconnect() {
	for {
		c.mu.RLock()
		conn := c.conn
		closed := c.closed
		c.mu.RUnlock()

		if closed {
			return
		}

		notifyClose := conn.NotifyClose(make(chan *amqp.Error, 1))

		select {
		case <-c.done:
			return
		case err := <-notifyClose:
			if err != nil {
				logger.Log.Warn("rabbitmq connection lost, reconnecting", zap.Error(err))
			}
			for {
				time.Sleep(reconnectDelay)
				if err := c.connect(); err != nil {
					logger.Log.Warn("rabbitmq reconnect failed, retrying", zap.Error(err))
					continue
				}
				logger.Log.Info("rabbitmq reconnected")
				break
			}
		}
	}
}

func (c *Client) Channel() (*amqp.Channel, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq connection not available")
	}

	return conn.Channel()
}

func (c *Client) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	ch, err := c.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	return ch.PublishWithContext(ctx,
		exchange,
		routingKey,
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

func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	close(c.done)
	conn := c.conn
	c.mu.Unlock()

	if conn != nil && !conn.IsClosed() {
		return conn.Close()
	}
	return nil
}
