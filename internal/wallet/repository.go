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
	const query = `
		INSERT INTO wallets (user_id, currency)
		VALUES ($1, $2)
		RETURNING id, user_id, currency, created_at
	`

	var wallet Wallet

	err := r.db.QueryRowContext(
		ctx,
		query,
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

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23503" {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("create wallet: %w", err)
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
		FROM wallets w
		LEFT JOIN ledger_entries le
			ON le.wallet_id = w.id
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