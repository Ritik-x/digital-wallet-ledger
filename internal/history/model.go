package history

import "time"

type Transaction struct {
ID                  int64     `json:"id"`
	Type                string    `json:"type"`

	AmountMinor         int64     `json:"amount_minor"`
	Currency            string    `json:"currency"`
	Status              string    `json:"status"`
	CounterpartyWalletID *int64   `json:"counterparty_wallet_id"`
	CreatedAt           time.Time `json:"created_at"`
}

// ListInput contains the filters for retrieving transaction history.
type ListInput struct {
	UserID   int64
	WalletID int64
	Limit    int
	Offset   int
}
type ListResult struct {
	WalletID     int64         `json:"wallet_id"`
	Transactions []Transaction `json:"transactions"`
	Limit        int           `json:"limit"`
	Offset       int           `json:"offset"`
	HasMore      bool          `json:"has_more"`
}