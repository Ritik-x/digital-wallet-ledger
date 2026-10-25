package messaging

import (
	"fmt"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connect opens a RabbitMQ connection and declares the notification queue.
func Connect() (*amqp.Connection, *amqp.Channel, error) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		return nil, nil, fmt.Errorf("RABBITMQ_URL is not configured")
	}

	queueName := os.Getenv("RABBITMQ_QUEUE")
	if queueName == "" {
		return nil, nil, fmt.Errorf("RABBITMQ_QUEUE is not configured")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}

	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable: queue survives broker restart
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("declare notification queue: %w", err)
	}

	return conn, ch, nil
}
