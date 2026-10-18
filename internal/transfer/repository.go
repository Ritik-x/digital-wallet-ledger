package transfer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletUnavailable   = errors.New("wallet is not active")
	ErrCurrencyMismatch    = errors.New("currency mismatch")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrIdempotencyConflict = errors.New(
		"idempotency key already used for a different transfer",
	)
)

type Repository interface {
	Create(
		ctx context.Context,
		input CreateTransferInput,
	) (*Transfer, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

type lockedWallet struct {
	ID       int64
	UserID   int64
	Currency string
	Status   string
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	input CreateTransferInput,
) (*Transfer, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transfer transaction: %w", err)
	}
	defer tx.Rollback()

	walletIDs := []int64{
		input.SenderWalletID,
		input.ReceiverWalletID,
	}

	sort.Slice(walletIDs, func(i, j int) bool {
		return walletIDs[i] < walletIDs[j]
	})

	wallets := make(map[int64]lockedWallet, 2)

	for _, walletID := range walletIDs {
		var wallet lockedWallet

		err := tx.QueryRowContext(
			ctx,
			`SELECT id, user_id, currency, status
			 FROM wallets
			 WHERE id = $1
			 FOR UPDATE`,
			walletID,
		).Scan(
			&wallet.ID,
			&wallet.UserID,
			&wallet.Currency,
			&wallet.Status,
		)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("lock wallet: %w", err)
		}

		wallets[wallet.ID] = wallet
	}

	sender := wallets[input.SenderWalletID]
	receiver := wallets[input.ReceiverWalletID]

	if sender.UserID != input.UserID {
		return nil, ErrWalletNotFound
	}

	existing, err := findTransferByIdempotencyKey(
		ctx,
		tx,
		input.UserID,
		input.IdempotencyKey,
	)

	if err == nil {
		return validateTransferReplay(existing, input)
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("find existing transfer: %w", err)
	}

	if sender.Status != "active" || receiver.Status != "active" {
		return nil, ErrWalletUnavailable
	}

	if sender.Currency != receiver.Currency ||
		sender.Currency != input.Currency {
		return nil, ErrCurrencyMismatch
	}

	var balance int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT COALESCE(
			SUM(
				CASE
					WHEN le.entry_type = 'credit'
						THEN le.amount_minor
					WHEN le.entry_type = 'debit'
						THEN -le.amount_minor
					ELSE 0
				END
			),
			0
		)
		FROM ledger_accounts AS la
		LEFT JOIN ledger_entries AS le
			ON le.ledger_account_id = la.id
		WHERE la.wallet_id = $1`,
		input.SenderWalletID,
	).Scan(&balance)

	if err != nil {
		return nil, fmt.Errorf("read sender balance: %w", err)
	}

	if balance < input.AmountMinor {
		return nil, ErrInsufficientFunds
	}

	var transfer Transfer

	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO transfers (
			sender_wallet_id,
			receiver_wallet_id,
			amount_minor,
			currency,
			status,
			idempotency_key
		)
		VALUES ($1, $2, $3, $4, 'posted', $5)
		ON CONFLICT (idempotency_key)
			WHERE idempotency_key IS NOT NULL
		DO NOTHING
		RETURNING
			id,
			sender_wallet_id,
			receiver_wallet_id,
			amount_minor,
			currency,
			status,
			created_at`,
		input.SenderWalletID,
		input.ReceiverWalletID,
		input.AmountMinor,
		input.Currency,
		input.IdempotencyKey,
	).Scan(
		&transfer.ID,
		&transfer.SenderWalletID,
		&transfer.ReceiverWalletID,
		&transfer.AmountMinor,
		&transfer.Currency,
		&transfer.Status,
		&transfer.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		existing, lookupErr := findTransferByIdempotencyKey(
			ctx,
			tx,
			input.UserID,
			input.IdempotencyKey,
		)

		if lookupErr == nil {
			return validateTransferReplay(existing, input)
		}

		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return nil, fmt.Errorf(
				"find conflicting transfer: %w",
				lookupErr,
			)
		}
		return nil, ErrIdempotencyConflict
	}

	if err != nil {
		return nil, fmt.Errorf("insert transfer: %w", err)
	}

	// Find the sender's ledger account.
	var senderAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		 FROM ledger_accounts
		 WHERE wallet_id = $1
		   AND currency = $2
		   AND status = 'active'`,
		input.SenderWalletID,
		input.Currency,
	).Scan(&senderAccountID)

	if err != nil {
		return nil, fmt.Errorf("find sender ledger account: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO ledger_entries (
			transfer_id,
			ledger_account_id,
			entry_type,
			amount_minor
		)
		VALUES ($1, $2, 'debit', $3)`,
		transfer.ID,
		senderAccountID,
		input.AmountMinor,
	)

	if err != nil {
		return nil, fmt.Errorf("insert debit entry: %w", err)
	}

	// Find the receiver's ledger account.
	var receiverAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		 FROM ledger_accounts
		 WHERE wallet_id = $1
		   AND currency = $2
		   AND status = 'active'`,
		input.ReceiverWalletID,
		input.Currency,
	).Scan(&receiverAccountID)

	if err != nil {
		return nil, fmt.Errorf("find receiver ledger account: %w", err)
	}

	// Record the credit.
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO ledger_entries (
			transfer_id,
			ledger_account_id,
			entry_type,
			amount_minor
		)
		VALUES ($1, $2, 'credit', $3)`,
		transfer.ID,
		receiverAccountID,
		input.AmountMinor,
	)

	if err != nil {
		return nil, fmt.Errorf("insert credit entry: %w", err)
	}

	// The transfer and both ledger entries commit together.
	// Create a notification event inside the same transaction.
	eventPayload, err := json.Marshal(map[string]interface{}{
		"transfer_id":        transfer.ID,
		"sender_wallet_id":   transfer.SenderWalletID,
		"receiver_wallet_id": transfer.ReceiverWalletID,
		"amount_minor":       transfer.AmountMinor,
		"currency":           transfer.Currency,
		"status":             transfer.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal transfer event: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO outbox_events (event_type, payload)
     VALUES ($1, $2)`,
		"transfer.completed",
		eventPayload,
	)
	if err != nil {
		return nil, fmt.Errorf("insert transfer outbox event: %w", err)
	}

	// The transfer, ledger entries, and event commit together.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transfer: %w", err)
	}
	return &transfer, nil
}

func findTransferByIdempotencyKey(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
	key string,
) (*Transfer, error) {
	var result Transfer

	err := tx.QueryRowContext(
		ctx,
		`SELECT
			t.id,
			t.sender_wallet_id,
			t.receiver_wallet_id,
			t.amount_minor,
			t.currency,
			t.status,
			t.created_at
		FROM transfers AS t
		JOIN wallets AS sender
			ON sender.id = t.sender_wallet_id
		WHERE t.idempotency_key = $1
		  AND sender.user_id = $2`,
		key,
		userID,
	).Scan(
		&result.ID,
		&result.SenderWalletID,
		&result.ReceiverWalletID,
		&result.AmountMinor,
		&result.Currency,
		&result.Status,
		&result.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &result, nil
}

func validateTransferReplay(
	existing *Transfer,
	input CreateTransferInput,
) (*Transfer, error) {
	if existing.SenderWalletID != input.SenderWalletID ||
		existing.ReceiverWalletID != input.ReceiverWalletID ||
		existing.AmountMinor != input.AmountMinor ||
		existing.Currency != input.Currency {
		return nil, ErrIdempotencyConflict
	}

	return existing, nil
}
