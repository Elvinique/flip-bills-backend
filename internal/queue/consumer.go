package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

type EventHandler func(ctx context.Context, event Event) error

func NewConsumer(amqpURL string) (*Consumer, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("queue: failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("queue: failed to open a channel: %w", err)
	}

	// Declare exchange to ensure it exists
	err = ch.ExchangeDeclare(
		"flipbills_events", "topic", true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: failed to declare exchange: %w", err)
	}

	return &Consumer{
		conn: conn,
		ch:   ch,
	}, nil
}

func (c *Consumer) Close() {
	c.ch.Close()
	c.conn.Close()
}

func (c *Consumer) StartConsuming(queueName, routingKey string, handler EventHandler) error {
	q, err := c.ch.QueueDeclare(
		queueName, true, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("queue: failed to declare a queue: %w", err)
	}

	err = c.ch.QueueBind(
		q.Name, routingKey, "flipbills_events", false, nil,
	)
	if err != nil {
		return fmt.Errorf("queue: failed to bind a queue: %w", err)
	}

	msgs, err := c.ch.Consume(
		q.Name, "", false, false, false, false, nil, // autoAck is false, we ack manually
	)
	if err != nil {
		return fmt.Errorf("queue: failed to register a consumer: %w", err)
	}

	go func() {
		for d := range msgs {
			var event Event
			if err := json.Unmarshal(d.Body, &event); err != nil {
				log.Printf("queue: failed to unmarshal event: %s", err)
				d.Reject(false) // bad format, drop it
				continue
			}

			if err := handler(context.Background(), event); err != nil {
				log.Printf("queue: handler error: %s", err)
				d.Nack(false, true) // requeue
			} else {
				d.Ack(false)
			}
		}
	}()

	return nil
}
