package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrWalletUnavailable = errors.New("wallet is not active")
	ErrCurrencyMismatch  = errors.New("currency mismatch")
	ErrInsufficientFunds = errors.New("insufficient funds")
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
			`SELECT id, currency, status
			 FROM wallets
			 WHERE id = $1
			 FOR UPDATE`,
			walletID,
		).Scan(
			&wallet.ID,
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
			status
		)
		VALUES ($1, $2, $3, $4, 'posted')
		RETURNING id, sender_wallet_id, receiver_wallet_id,
		          amount_minor, currency, status, created_at`,
		input.SenderWalletID,
		input.ReceiverWalletID,
		input.AmountMinor,
		input.Currency,
	).Scan(
		&transfer.ID,
		&transfer.SenderWalletID,
		&transfer.ReceiverWalletID,
		&transfer.AmountMinor,
		&transfer.Currency,
		&transfer.Status,
		&transfer.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("insert transfer: %w", err)
	}

	// Record the debit.

	var senderAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
     FROM ledger_accounts
     WHERE wallet_id = $1`,
		input.SenderWalletID,
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

	// Record the credit.

	var receiverAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
     FROM ledger_accounts
     WHERE wallet_id = $1`,
		input.ReceiverWalletID,
	).Scan(&receiverAccountID)

	if err != nil {
		return nil, fmt.Errorf("find receiver ledger account: %w", err)
	}

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

	// Nothing becomes committed until all writes succeed.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transfer: %w", err)
	}

	return &transfer, nil
}
