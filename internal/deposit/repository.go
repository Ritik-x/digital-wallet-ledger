package deposit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletUnavailable   = errors.New("wallet is not active")
	ErrCurrencyMismatch    = errors.New("currency mismatch")
	ErrIdempotencyConflict = errors.New(
		"idempotency key already used for another request",
	)
)

type Repository interface {
	Create(ctx context.Context, input CreateDepositInput) (*Deposit, error)
}
type PostgresRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}
func (r *PostgresRepository) Create(
	ctx context.Context,
	input CreateDepositInput,
) (*Deposit, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin deposit transaction: %w", err)
	}
	defer tx.Rollback()
	// First, check whether this request has already been processed.
	existing, err := findDepositByKey(
		ctx,
		tx,
		input.IdempotencyKey,
	)
	if err == nil {
		if existing.WalletID != input.WalletID ||
			existing.AmountMinor != input.AmountMinor ||
			strings.TrimSpace(existing.Currency) != input.Currency {
			return nil, ErrIdempotencyConflict
		}

		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing deposit: %w", err)
	}

	var walletCurrency string
	var walletStatus string

	err = tx.QueryRowContext(
		ctx,
		`SELECT currency, status
         FROM wallets
         WHERE id = $1
         FOR UPDATE`,
		input.WalletID,
	).Scan(&walletCurrency, &walletStatus)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWalletNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock deposit wallet: %w", err)
	}
	if walletStatus != "active" {
		return nil, ErrWalletUnavailable
	}

	if strings.TrimSpace(walletCurrency) != input.Currency {
		return nil, ErrCurrencyMismatch
	}
	// Find the system's clearing account for this currency.
	var clearingAccountID int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT id
         FROM ledger_accounts
         WHERE account_type = 'system'
           AND code = 'external_clearing'
           AND currency = $1
           AND status = 'active'`,
		input.Currency,
	).Scan(&clearingAccountID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf(
			"no active clearing account for currency %s",
			input.Currency,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("find clearing account: %w", err)
	}

	// Find the ledger account belonging to the wallet.
	var walletAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
         FROM ledger_accounts
         WHERE wallet_id = $1
           AND currency = $2
           AND status = 'active'`,
		input.WalletID,
		input.Currency,
	).Scan(&walletAccountID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("wallet ledger account not found")
	}
	if err != nil {
		return nil, fmt.Errorf("find wallet ledger account: %w", err)
	}

	// Insert the deposit record.
	var created Deposit

	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO deposits (
            wallet_id,
            amount_minor,
            currency,
            status,
            idempotency_key
         )
         VALUES ($1, $2, $3, 'posted', $4)
         ON CONFLICT (idempotency_key) DO NOTHING
         RETURNING id, wallet_id, amount_minor, currency,
                   status, idempotency_key, created_at`,
		input.WalletID,
		input.AmountMinor,
		input.Currency,
		input.IdempotencyKey,
	).Scan(
		&created.ID,
		&created.WalletID,
		&created.AmountMinor,
		&created.Currency,
		&created.Status,
		&created.IdempotencyKey,
		&created.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		// Another request may have inserted this key concurrently.
		existing, lookupErr := findDepositByKey(
			ctx,
			tx,
			input.IdempotencyKey,
		)
		if lookupErr != nil {
			return nil, fmt.Errorf(
				"retrieve concurrent deposit: %w",
				lookupErr,
			)
		}

		if existing.WalletID != input.WalletID ||
			existing.AmountMinor != input.AmountMinor ||
			strings.TrimSpace(existing.Currency) != input.Currency {
			return nil, ErrIdempotencyConflict
		}

		return existing, nil
	}

	if err != nil {
		return nil, fmt.Errorf("insert deposit: %w", err)
	}

	// Debit the system clearing account.
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO ledger_entries (
            deposit_id,
            ledger_account_id,
            entry_type,
            amount_minor
         )
         VALUES ($1, $2, 'debit', $3)`,
		created.ID,
		clearingAccountID,
		input.AmountMinor,
	)
	if err != nil {
		return nil, fmt.Errorf("insert clearing debit: %w", err)
	}

	// Credit the customer's wallet account.
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO ledger_entries (
            deposit_id,
            ledger_account_id,
            entry_type,
            amount_minor
         )
         VALUES ($1, $2, 'credit', $3)`,
		created.ID,
		walletAccountID,
		input.AmountMinor,
	)
	if err != nil {
		return nil, fmt.Errorf("insert wallet credit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit deposit: %w", err)
	}

	return &created, nil
}

func findDepositByKey(
	ctx context.Context,
	tx *sql.Tx,
	key string,
) (*Deposit, error) {
	var d Deposit

	err := tx.QueryRowContext(
		ctx,
		`SELECT id, wallet_id, amount_minor, currency,
                status, idempotency_key, created_at
         FROM deposits
         WHERE idempotency_key = $1`,
		key,
	).Scan(
		&d.ID,
		&d.WalletID,
		&d.AmountMinor,
		&d.Currency,
		&d.Status,
		&d.IdempotencyKey,
		&d.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &d, nil
}
