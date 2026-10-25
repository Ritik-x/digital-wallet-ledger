package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrTransferNotFound            = errors.New("transfer not found")
	ErrTransferAlreadyReversed     = errors.New("transfer already reversed")
	ErrTransferNotReversible       = errors.New("transfer is not reversible")
	ErrReversalInsufficientFunds   = errors.New("receiver has insufficient funds")
	ErrReversalIdempotencyConflict = errors.New(
		"idempotency key already used for another reversal",
	)
)

type ReversalRepository interface {
	Reverse(
		ctx context.Context,
		input ReverseTransferInput,
	) (*TransferReversal, error)
}

type PostgresReversalRepository struct {
	db *sql.DB
}

func NewPostgresReversalRepository(
	db *sql.DB,
) *PostgresReversalRepository {
	return &PostgresReversalRepository{db: db}
}

type transferForReversal struct {
	ID               int64
	SenderWalletID   int64
	ReceiverWalletID int64
	AmountMinor      int64
	Currency         string
	Status           string
	SenderUserID     int64
}

func (r *PostgresReversalRepository) Reverse(
	ctx context.Context,
	input ReverseTransferInput,
) (*TransferReversal, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin reversal transaction: %w", err)
	}
	defer tx.Rollback()

	// First check whether this is a retry of a completed reversal.
	existing, err := findReversalByKey(
		ctx,
		tx,
		input.IdempotencyKey,
		input.UserID,
	)
	if err == nil {
		if existing.TransferID != input.TransferID {
			return nil, ErrReversalIdempotencyConflict
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("find existing reversal: %w", err)
	}

	// Find the transfer, but only if the authenticated user owns
	// the sender wallet. Do not expose another user's transfer.
	var transfer transferForReversal

	err = tx.QueryRowContext(
		ctx,
		`
		SELECT
			t.id,
			t.sender_wallet_id,
			t.receiver_wallet_id,
			t.amount_minor,
			t.currency,
			t.status,
			sender.user_id
		FROM transfers AS t
		JOIN wallets AS sender
			ON sender.id = t.sender_wallet_id
		WHERE t.id = $1
		  AND sender.user_id = $2
		`,
		input.TransferID,
		input.UserID,
	).Scan(
		&transfer.ID,
		&transfer.SenderWalletID,
		&transfer.ReceiverWalletID,
		&transfer.AmountMinor,
		&transfer.Currency,
		&transfer.Status,
		&transfer.SenderUserID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTransferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load transfer: %w", err)
	}

	// Lock wallets in a consistent order to reduce deadlocks.
	walletIDs := []int64{
		transfer.SenderWalletID,
		transfer.ReceiverWalletID,
	}
	sort.Slice(walletIDs, func(i, j int) bool {
		return walletIDs[i] < walletIDs[j]
	})

	for _, walletID := range walletIDs {
		var status string

		err := tx.QueryRowContext(
			ctx,
			`
			SELECT status
			FROM wallets
			WHERE id = $1
			FOR UPDATE
			`,
			walletID,
		).Scan(&status)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTransferNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("lock wallet: %w", err)
		}
		if status != "active" {
			return nil, ErrWalletUnavailable
		}
	}

	// Lock and re-read the transfer after locking both wallets.
	// This makes concurrent reversal attempts recheck the current state.
	err = tx.QueryRowContext(
		ctx,
		`
		SELECT
			id,
			sender_wallet_id,
			receiver_wallet_id,
			amount_minor,
			currency,
			status
		FROM transfers
		WHERE id = $1
		FOR UPDATE
		`,
		input.TransferID,
	).Scan(
		&transfer.ID,
		&transfer.SenderWalletID,
		&transfer.ReceiverWalletID,
		&transfer.AmountMinor,
		&transfer.Currency,
		&transfer.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("reload transfer: %w", err)
	}

	// A previous request might have reversed this transfer while
	// this request was waiting for the wallet locks.
	existing, err = findReversalByTransferID(
		ctx,
		tx,
		input.TransferID,
	)
	if err == nil {
		if existing.IdempotencyKey == input.IdempotencyKey {
			return existing, nil
		}
		return nil, ErrTransferAlreadyReversed
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check previous reversal: %w", err)
	}

	if transfer.Status != "posted" {
		return nil, ErrTransferNotReversible
	}

	// The receiver must still have enough funds to return.
	var receiverBalance int64

	err = tx.QueryRowContext(
		ctx,
		`
		SELECT COALESCE(
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
		WHERE la.wallet_id = $1
		  AND la.currency = $2
		`,
		transfer.ReceiverWalletID,
		transfer.Currency,
	).Scan(&receiverBalance)
	if err != nil {
		return nil, fmt.Errorf("read receiver balance: %w", err)
	}

	if receiverBalance < transfer.AmountMinor {
		return nil, ErrReversalInsufficientFunds
	}

	// Both ledger accounts must exist and be active.
	senderAccountID, err := getActiveWalletAccount(
		ctx,
		tx,
		transfer.SenderWalletID,
		transfer.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("find sender ledger account: %w", err)
	}

	receiverAccountID, err := getActiveWalletAccount(
		ctx,
		tx,
		transfer.ReceiverWalletID,
		transfer.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("find receiver ledger account: %w", err)
	}

	// Create the reversal record first.
	var reversal TransferReversal

	err = tx.QueryRowContext(
		ctx,
		`
		INSERT INTO transfer_reversals (
			transfer_id,
			idempotency_key
		)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		RETURNING id, transfer_id, idempotency_key, created_at
		`,
		transfer.ID,
		input.IdempotencyKey,
	).Scan(
		&reversal.ID,
		&reversal.TransferID,
		&reversal.IdempotencyKey,
		&reversal.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTransferAlreadyReversed
	}
	if err != nil {
		return nil, fmt.Errorf("create reversal record: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`
		INSERT INTO ledger_entries (
			transfer_id,
			deposit_id,
			reversal_id,
			ledger_account_id,
			entry_type,
			amount_minor
		)
		VALUES
			(NULL, NULL, $1, $2, 'credit', $3),
			(NULL, NULL, $1, $4, 'debit', $3)
		`,
		reversal.ID,
		senderAccountID,
		transfer.AmountMinor,
		receiverAccountID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert reversal ledger entries: %w", err)
	}

	// Mark the original transfer reversed only after the compensating
	// entries have been inserted. All changes commit together.
	result, err := tx.ExecContext(
		ctx,
		`
		UPDATE transfers
		SET status = 'reversed'
		WHERE id = $1
		  AND status = 'posted'
		`,
		transfer.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update transfer status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check transfer update: %w", err)
	}
	if affected != 1 {
		return nil, ErrTransferNotReversible
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit reversal: %w", err)
	}

	return &reversal, nil
}

func findReversalByKey(
	ctx context.Context,
	tx *sql.Tx,
	key string,
	userID int64,
) (*TransferReversal, error) {
	var result TransferReversal

	err := tx.QueryRowContext(
		ctx,
		`
		SELECT
			r.id,
			r.transfer_id,
			r.idempotency_key,
			r.created_at
		FROM transfer_reversals AS r
		JOIN transfers AS t ON t.id = r.transfer_id
		JOIN wallets AS sender ON sender.id = t.sender_wallet_id
		WHERE r.idempotency_key = $1
		  AND sender.user_id = $2
		`,
		key,
		userID,
	).Scan(
		&result.ID,
		&result.TransferID,
		&result.IdempotencyKey,
		&result.CreatedAt,
	)

	if err != nil {
		return nil, err
	}
	return &result, nil
}

func findReversalByTransferID(
	ctx context.Context,
	tx *sql.Tx,
	transferID int64,
) (*TransferReversal, error) {
	var result TransferReversal

	err := tx.QueryRowContext(
		ctx,
		`
		SELECT id, transfer_id, idempotency_key, created_at
		FROM transfer_reversals
		WHERE transfer_id = $1
		`,
		transferID,
	).Scan(
		&result.ID,
		&result.TransferID,
		&result.IdempotencyKey,
		&result.CreatedAt,
	)

	if err != nil {
		return nil, err
	}
	return &result, nil
}

func getActiveWalletAccount(
	ctx context.Context,
	tx *sql.Tx,
	walletID int64,
	currency string,
) (int64, error) {
	var accountID int64

	err := tx.QueryRowContext(
		ctx,
		`
		SELECT id
		FROM ledger_accounts
		WHERE wallet_id = $1
		  AND currency = $2
		  AND status = 'active'
		`,
		walletID,
		currency,
	).Scan(&accountID)

	if err != nil {
		return 0, err
	}
	return accountID, nil
}
