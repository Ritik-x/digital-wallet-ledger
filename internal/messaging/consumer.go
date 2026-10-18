package messaging

import (
	"context"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func StartConsumer(
	ctx context.Context,
	conn *amqp.Connection,
	queueName string,
) error {

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	messages, err := ch.Consume(
		queueName,
		"ledgerx-notification-worker",
		false, // auto-ack disabled
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}
	log.Println("Transaction notification consumer started")

	for {
		select {
		case <-ctx.Done():
			return nil

		case message, ok := <-messages:
			if !ok {
				return nil
			}

			log.Printf(
				"Received event type=%s id=%s payload=%s",
				message.Type,
				message.MessageId,
				message.Body,
			)

			if err := message.Ack(false); err != nil {
				log.Printf("acknowledge message: %v", err)
			}
		}
	}
}
