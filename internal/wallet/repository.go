package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrWalletNotFound = errors.New("wallet not found")
	ErrUserNotFound   = errors.New("user not found")
)

type Repository interface {
	Create(ctx context.Context, input CreateWalletInput) (*Wallet, error)
	GetBalance(ctx context.Context, walletID int64) (*Balance, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	input CreateWalletInput,
) (*Wallet, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin wallet creation transaction: %w", err)
	}
	defer tx.Rollback()

	const insertWallet = `
		INSERT INTO wallets (user_id, currency)
		VALUES ($1, $2)
		RETURNING id, user_id, currency, created_at
	`

	var wallet Wallet

	err = tx.QueryRowContext(
		ctx,
		insertWallet,
		input.UserID,
		input.Currency,
	).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Currency,
		&wallet.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("create wallet: %w", err)
	}

	const insertLedgerAccount = `
		INSERT INTO ledger_accounts (
			account_type,
			wallet_id,
			currency,
			status
		)
		VALUES ('wallet', $1, $2, 'active')
	`

	_, err = tx.ExecContext(
		ctx,
		insertLedgerAccount,
		wallet.ID,
		wallet.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("create wallet ledger account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit wallet creation: %w", err)
	}

	return &wallet, nil
}

func (r *PostgresRepository) GetBalance(
	ctx context.Context,
	walletID int64,
) (*Balance, error) {
	const query = `
		SELECT
			w.id,
			w.currency,
			COALESCE(
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
			) AS balance_minor
  FROM wallets AS w
        JOIN ledger_accounts AS la
            ON la.wallet_id = w.id
        LEFT JOIN ledger_entries AS le
            ON le.ledger_account_id = la.id
        WHERE w.id = $1
        GROUP BY w.id, w.currency
	`

	var balance Balance

	err := r.db.QueryRowContext(
		ctx,
		query,
		walletID,
	).Scan(
		&balance.WalletID,
		&balance.Currency,
		&balance.BalanceMinor,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWalletNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get wallet balance: %w", err)
	}

	return &balance, nil
}
