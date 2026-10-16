package deposit

import "time"
type ReverseDepositInput struct {
	UserID        int64
	DepositID     int64
	IdempotencyKey string
}


type DepositReversal struct {
	ID             int64     `json:"id"`
	DepositID      int64     `json:"deposit_id"`
	IdempotencyKey string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}