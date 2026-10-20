package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"database/sql"
	"digital-wallet/internal/notification"

	amqp "github.com/rabbitmq/amqp091-go"
)

func StartConsumer(
	ctx context.Context,
	conn *amqp.Connection,
	queueName string,
	db *sql.DB,
	email notification.TransferEmailSender,
) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("configure consumer QoS: %w", err)
	}

	recipients := notification.NewRecipientRepository(db)
	processor := notification.NewProcessor(recipients, email)

	messages, err := ch.Consume(
		queueName,
		"ledgerx-notification-worker",
		false, // Manual acknowledgement
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("start consuming messages: %w", err)
	}

	log.Println("Transfer notification consumer started")

	for {
		select {
		case <-ctx.Done():
			return nil

		case message, ok := <-messages:
			if !ok {
				return fmt.Errorf("RabbitMQ delivery channel closed")
			}

			if message.Type != "transfer.completed" {
				log.Printf("Rejecting unsupported event type: %s", message.Type)
				if err := message.Reject(false); err != nil {
					log.Printf("reject unsupported message: %v", err)
				}
				continue
			}

			var event notification.TransferCompletedEvent
			if err := json.Unmarshal(message.Body, &event); err != nil {
				log.Printf("Invalid transfer event: %v", err)
				if rejectErr := message.Reject(false); rejectErr != nil {
					log.Printf("reject invalid message: %v", rejectErr)
				}
				continue
			}

			processCtx, cancel := context.WithTimeout(ctx, 15_000_000_000)
			err := processor.ProcessTransferCompleted(processCtx, event)
			cancel()

			if err != nil {
				log.Printf(
					"Process transfer notification failed: transfer_id=%d error=%v",
					event.TransferID,
					err,
				)

				// Requeue for now. We will add bounded retries and
				// dead-letter handling before production deployment.
				if rejectErr := message.Nack(false, true); rejectErr != nil {
					log.Printf("requeue failed message: %v", rejectErr)
				}
				continue
			}

			if err := message.Ack(false); err != nil {
				log.Printf("acknowledge processed message: %v", err)
			}
		}
	}
}
