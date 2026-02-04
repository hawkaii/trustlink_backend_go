package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/trustlink/common/log"
	"go.uber.org/zap"
)

const (
	ExchangeName = "trustlink.events"
	ExchangeType = "topic"
)

// Connection holds the RabbitMQ connection and channel
type Connection struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
}

// Connect establishes a connection to RabbitMQ
func Connect(url string) (*Connection, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare the exchange
	err = channel.ExchangeDeclare(
		ExchangeName, // name
		ExchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	log.Info("Connected to RabbitMQ", zap.String("exchange", ExchangeName))

	return &Connection{
		conn:    conn,
		channel: channel,
	}, nil
}

// Publish publishes a message to the exchange with a routing key
func (c *Connection) Publish(ctx context.Context, routingKey string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	err = c.channel.PublishWithContext(
		ctx,
		ExchangeName, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp091.Persistent,
			Timestamp:    time.Now(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	log.Debug("Published message",
		zap.String("routingKey", routingKey),
		zap.ByteString("payload", body))

	return nil
}

// ConsumeOptions holds options for consuming messages
type ConsumeOptions struct {
	QueueName   string
	RoutingKeys []string
	Handler     func([]byte) error
}

// Consume sets up a consumer for the given queue and routing keys
func (c *Connection) Consume(ctx context.Context, opts ConsumeOptions) error {
	// Declare queue
	queue, err := c.channel.QueueDeclare(
		opts.QueueName, // name
		true,           // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to routing keys
	for _, routingKey := range opts.RoutingKeys {
		err = c.channel.QueueBind(
			queue.Name,   // queue name
			routingKey,   // routing key
			ExchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to bind queue to routing key %s: %w", routingKey, err)
		}
		log.Info("Bound queue to routing key",
			zap.String("queue", queue.Name),
			zap.String("routingKey", routingKey))
	}

	// Start consuming
	msgs, err := c.channel.Consume(
		queue.Name, // queue
		"",         // consumer
		false,      // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Info("Started consuming messages", zap.String("queue", queue.Name))

	// Process messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info("Stopping consumer", zap.String("queue", queue.Name))
				return
			case msg, ok := <-msgs:
				if !ok {
					log.Warn("Message channel closed")
					return
				}

				log.Debug("Received message",
					zap.String("routingKey", msg.RoutingKey),
					zap.ByteString("body", msg.Body))

				// Handle message
				if err := opts.Handler(msg.Body); err != nil {
					log.Error("Failed to handle message",
						zap.Error(err),
						zap.String("routingKey", msg.RoutingKey))
					msg.Nack(false, true) // Requeue on error
				} else {
					msg.Ack(false)
				}
			}
		}
	}()

	return nil
}

// Close closes the RabbitMQ connection and channel
func (c *Connection) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
