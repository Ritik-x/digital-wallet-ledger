// we create htis for the pending message  if we have a remaig or pending message we send again in queue
package messaging

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type OutboxPublisher struct {
	db        *sql.DB
	channel   *amqp.Channel
	queueName string
}

func NewOutboxPublisher(
	db *sql.DB,
	channel *amqp.Channel,
	queueName string,
) *OutboxPublisher {
	return &OutboxPublisher{
		db:        db,
		channel:   channel,
		queueName: queueName,
	}
}

// Start periodically publishes pending outbox events.
func (p *OutboxPublisher) Start(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.PublishPending(ctx); err != nil {
				log.Printf("publish outbox events: %v", err)
			}
		}
	}
}

func (p *OutboxPublisher) PublishPending(ctx context.Context) error {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT id, event_type, payload
		 FROM outbox_events
		 WHERE published_at IS NULL
		 ORDER BY id
		 LIMIT 100`,
	)
	if err != nil {
		return fmt.Errorf("query pending events: %w", err)
	}

	type event struct {
		ID      int64
		Type    string
		Payload []byte
	}

	var events []event

	for rows.Next() {
		var e event
		if err := rows.Scan(&e.ID, &e.Type, &e.Payload); err != nil {
			rows.Close()
			return fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate outbox events: %w", err)
	}
	rows.Close()

	for _, e := range events {
		if err := p.channel.Confirm(false); err != nil {
			return fmt.Errorf("enable publisher confirms: %w", err)
		}

		confirms := p.channel.NotifyPublish(make(chan amqp.Confirmation, 1))

		err := p.channel.PublishWithContext(
			ctx,
			"",
			p.queueName,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Type:         e.Type,
				MessageId:    fmt.Sprintf("%d", e.ID),
				Body:         e.Payload,
			},
		)
		if err != nil {
			return fmt.Errorf("publish event %d: %w", e.ID, err)
		}

		select {
		case confirmation := <-confirms:
			if !confirmation.Ack {
				return fmt.Errorf("RabbitMQ rejected event %d", e.ID)
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timed out waiting for event %d confirmation", e.ID)
		}

		_, err = p.db.ExecContext(
			ctx,
			`UPDATE outbox_events
			 SET published_at = NOW()
			 WHERE id = $1 AND published_at IS NULL`,
			e.ID,
		)
		if err != nil {
			return fmt.Errorf("mark event %d published: %w", e.ID, err)
		}
	}

	return nil
}
