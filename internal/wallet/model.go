package wallet

import "time"

type CreateWalletInput struct {
	UserID   int64
	Currency string
}

type Wallet struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

type Balance struct {
	WalletID     int64  `json:"wallet_id"`
	Currency     string `json:"currency"`
	BalanceMinor int64  `json:"balance_minor"`
}
