package transfer

import "time"

type CreateTransferInput struct {
	SenderWalletID   int64
	ReceiverWalletID int64
	AmountMinor      int64
	Currency         string
}

type Transfer struct {
	ID               int64     `json:"id"`
	SenderWalletID   int64     `json:"sender_wallet_id"`
	ReceiverWalletID int64     `json:"receiver_wallet_id"`
	AmountMinor      int64     `json:"amount_minor"`
	Currency         string    `json:"currency"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}
