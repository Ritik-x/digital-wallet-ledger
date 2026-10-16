package transfer

import "time"

type ReverseTransferInput struct {
	UserID         int64
	TransferID     int64
	IdempotencyKey string
}

type TransferReversal struct {
	ID             int64     `json:"id"`
	TransferID     int64     `json:"transfer_id"`
	IdempotencyKey string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}
