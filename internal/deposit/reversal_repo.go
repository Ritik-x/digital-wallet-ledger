package deposit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrDepositNotFound                    = errors.New("deposit not found")
	ErrDepositAlreadyReversed             = errors.New("deposit already reversed")
	ErrDepositNotReversible               = errors.New("deposit is not reversible")
	ErrDepositReversalInsufficientFunds    = errors.New("wallet has insufficient funds for reversal")
	ErrDepositReversalIdempotencyConflict = errors.New("idempotency key already used for another reversal")
	ErrDepositWalletUnavailable           = errors.New("wallet or ledger account is unavailable")
)

type ReversalRepository interface {
	Reverse(ctx context.Context, input ReverseDepositInput) (*DepositReversal, error)
}

type PostgresReversalRepository struct {
	db *sql.DB
}

func NewPostgresReversalRepository(db *sql.DB) *PostgresReversalRepository {
	return &PostgresReversalRepository{db: db}
}

func (r *PostgresReversalRepository) Reverse(
	ctx context.Context,
	input ReverseDepositInput,
) (*DepositReversal, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin deposit reversal transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Check whether this request was already completed.
	existing, ownerID, err := findDepositReversalByKey(
		ctx, tx, input.IdempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("find deposit reversal by key: %w", err)
	}

	if existing != nil {
		if ownerID != input.UserID || existing.DepositID != input.DepositID {
			return nil, ErrDepositReversalIdempotencyConflict
		}

		return existing, nil
	}

	// 2. Find the deposit and verify its owner.
	var walletID int64

	err = tx.QueryRowContext(ctx, `
		SELECT d.wallet_id
		FROM deposits d
		JOIN wallets w ON w.id = d.wallet_id
		WHERE d.id = $1
		  AND w.user_id = $2
	`, input.DepositID, input.UserID).Scan(&walletID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDepositNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find owned deposit: %w", err)
	}

	// 3. Lock the wallet.
	// Other wallet operations should acquire this same lock
	// before changing the balance.
	var walletStatus string

	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM wallets
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, walletID, input.UserID).Scan(&walletStatus)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDepositNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock wallet: %w", err)
	}

	if walletStatus != "active" {
		return nil, ErrDepositWalletUnavailable
	}

	// 4. Lock the deposit and read its current state.
	var (
		depositWalletID int64
		amountMinor     int64
		currency        string
		status          string
	)

	err = tx.QueryRowContext(ctx, `
		SELECT wallet_id, amount_minor, currency, status
		FROM deposits
		WHERE id = $1
		FOR UPDATE
	`, input.DepositID).Scan(
		&depositWalletID,
		&amountMinor,
		&currency,
		&status,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDepositNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock deposit: %w", err)
	}

	if depositWalletID != walletID {
		return nil, ErrDepositNotFound
	}

	// 5. Handle a reversal that may have been created
	// while this request was waiting for the wallet lock.
	existing, err = findDepositReversalByDepositID(
		ctx, tx, input.DepositID,
	)
	if err != nil {
		return nil, fmt.Errorf("find existing deposit reversal: %w", err)
	}

	if existing != nil {
		if existing.IdempotencyKey == input.IdempotencyKey {
			return existing, nil
		}
		return nil, ErrDepositAlreadyReversed
	}

	if status != "posted" {
		return nil, ErrDepositNotReversible
	}

	// 6. Find the active wallet ledger account.
	walletAccountID, err := getDepositWalletAccountID(
		ctx, tx, walletID, currency,
	)
	if err != nil {
		return nil, err
	}

	// 7. Check the wallet's current ledger balance.
	var balanceMinor int64

	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(
			SUM(
				CASE
					WHEN entry_type = 'credit' THEN amount_minor
					ELSE -amount_minor
				END
			),
			0
		)
		FROM ledger_entries
		WHERE ledger_account_id = $1
	`, walletAccountID).Scan(&balanceMinor)

	if err != nil {
		return nil, fmt.Errorf("calculate wallet balance: %w", err)
	}

	if balanceMinor < amountMinor {
		return nil, ErrDepositReversalInsufficientFunds
	}

	// 8. Find the active system clearing account.
	var systemAccountID int64

	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM ledger_accounts
		WHERE account_type = 'system'
		  AND code = 'external_clearing'
		  AND currency = $1
		  AND status = 'active'
	`, currency).Scan(&systemAccountID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDepositWalletUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("find system clearing account: %w", err)
	}

	// 9. Create the reversal record.
	result := &DepositReversal{}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO deposit_reversals (
			deposit_id,
			idempotency_key
		)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		RETURNING id, deposit_id, idempotency_key, created_at
	`, input.DepositID, input.IdempotencyKey).Scan(
		&result.ID,
		&result.DepositID,
		&result.IdempotencyKey,
		&result.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		// A unique constraint was hit. Return a safe conflict
		// rather than risking duplicate ledger entries.
		return nil, ErrDepositAlreadyReversed
	}
	if err != nil {
		return nil, fmt.Errorf("insert deposit reversal: %w", err)
	}

	// 10. Debit the wallet to cancel the original credit.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			ledger_account_id,
			entry_type,
			amount_minor,
			deposit_reversal_id
		)
		VALUES ($1, 'debit', $2, $3)
	`, walletAccountID, amountMinor, result.ID)

	if err != nil {
		return nil, fmt.Errorf("insert wallet reversal debit: %w", err)
	}

	// 11. Credit system clearing to cancel the original debit.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			ledger_account_id,
			entry_type,
			amount_minor,
			deposit_reversal_id
		)
		VALUES ($1, 'credit', $2, $3)
	`, systemAccountID, amountMinor, result.ID)

	if err != nil {
		return nil, fmt.Errorf("insert clearing reversal credit: %w", err)
	}

	// 12. Mark the original deposit as reversed.
	updateResult, err := tx.ExecContext(ctx, `
		UPDATE deposits
		SET status = 'reversed'
		WHERE id = $1 AND status = 'posted'
	`, input.DepositID)

	if err != nil {
		return nil, fmt.Errorf("update deposit status: %w", err)
	}

	rowsAffected, err := updateResult.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check deposit status update: %w", err)
	}
	if rowsAffected != 1 {
		return nil, ErrDepositNotReversible
	}

	// 13. Commit the reversal record, both entries,
	// and the status update as one atomic operation.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit deposit reversal: %w", err)
	}

	return result, nil
}

func findDepositReversalByKey(
	ctx context.Context,
	tx *sql.Tx,
	key string,
) (*DepositReversal, int64, error) {
	result := &DepositReversal{}
	var ownerID int64

	err := tx.QueryRowContext(ctx, `
		SELECT
			dr.id,
			dr.deposit_id,
			dr.idempotency_key,
			dr.created_at,
			w.user_id
		FROM deposit_reversals dr
		JOIN deposits d ON d.id = dr.deposit_id
		JOIN wallets w ON w.id = d.wallet_id
		WHERE dr.idempotency_key = $1
	`, key).Scan(
		&result.ID,
		&result.DepositID,
		&result.IdempotencyKey,
		&result.CreatedAt,
		&ownerID,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	return result, ownerID, nil
}

func findDepositReversalByDepositID(
	ctx context.Context,
	tx *sql.Tx,
	depositID int64,
) (*DepositReversal, error) {
	result := &DepositReversal{}

	err := tx.QueryRowContext(ctx, `
		SELECT id, deposit_id, idempotency_key, created_at
		FROM deposit_reversals
		WHERE deposit_id = $1
	`, depositID).Scan(
		&result.ID,
		&result.DepositID,
		&result.IdempotencyKey,
		&result.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return result, nil
}

func getDepositWalletAccountID(
	ctx context.Context,
	tx *sql.Tx,
	walletID int64,
	currency string,
) (int64, error) {
	var accountID int64

	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM ledger_accounts
		WHERE account_type = 'wallet'
		  AND wallet_id = $1
		  AND currency = $2
		  AND status = 'active'
	`, walletID, currency).Scan(&accountID)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrDepositWalletUnavailable
	}
	if err != nil {
		return 0, fmt.Errorf("find wallet ledger account: %w", err)
	}

	return accountID, nil
}
