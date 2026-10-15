package deposit

import "time"

type CreateDepositInput struct {
	WalletID       int64
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
}

type Deposit struct {
	ID             int64     `json:"id"`
	WalletID       int64     `json:"wallet_id"`
	AmountMinor    int64     `json:"amount_minor"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}
