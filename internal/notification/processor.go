package notification

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type TransferEmailSender interface {
	SendTransferEmail(
		ctx context.Context,
		to string,
		transferID int64,
		amountMinor int64,
		currency string,
	) error
}

type Processor struct {
	recipients *RecipientRepository
	email      TransferEmailSender
}

func NewProcessor(
	recipients *RecipientRepository,
	email TransferEmailSender,
) *Processor {
	return &Processor{
		recipients: recipients,
		email:      email,
	}
}

func (p *Processor) ProcessTransferCompleted(
	ctx context.Context,
	event TransferCompletedEvent,
) error {
	if event.TransferID <= 0 {
		return fmt.Errorf("invalid transfer ID")
	}
	if event.SenderWalletID <= 0 || event.ReceiverWalletID <= 0 {
		return fmt.Errorf("invalid wallet ID")
	}
	if event.SenderWalletID == event.ReceiverWalletID {
		return fmt.Errorf("sender and receiver wallets must differ")
	}
	if event.AmountMinor <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	event.Currency = strings.ToUpper(strings.TrimSpace(event.Currency))
	if len(event.Currency) != 3 {
		return fmt.Errorf("invalid currency")
	}
	if event.Status != "posted" {
		return fmt.Errorf("transfer is not posted: %s", event.Status)
	}

	senderEmail, err := p.recipients.GetWalletOwnerEmail(
		ctx,
		event.SenderWalletID,
	)
	if err != nil {
		return fmt.Errorf("get sender email: %w", err)
	}

	receiverEmail, err := p.recipients.GetWalletOwnerEmail(
		ctx,
		event.ReceiverWalletID,
	)
	if err != nil {
		return fmt.Errorf("get receiver email: %w", err)
	}

	if err := p.email.SendTransferEmail(
		ctx,
		senderEmail,
		event.TransferID,
		event.AmountMinor,
		event.Currency,
	); err != nil {
		return fmt.Errorf("notify sender: %w", err)
	}

	if receiverEmail != senderEmail {
		if err := p.email.SendTransferEmail(
			ctx,
			receiverEmail,
			event.TransferID,
			event.AmountMinor,
			event.Currency,
		); err != nil {
			return fmt.Errorf("notify receiver: %w", err)
		}
	}

	log.Printf("Transfer notifications processed for transfer_id=%d", event.TransferID)
	return nil
}
