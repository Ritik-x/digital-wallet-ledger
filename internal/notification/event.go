package notification

// TransferCompletedEvent represents a completed transfer
// event published by the LedgerX outbox.
type TransferCompletedEvent struct {
	TransferID       int64  `json:"transfer_id"`
	SenderWalletID   int64  `json:"sender_wallet_id"`
	ReceiverWalletID int64  `json:"receiver_wallet_id"`
	AmountMinor      int64  `json:"amount_minor"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
}
