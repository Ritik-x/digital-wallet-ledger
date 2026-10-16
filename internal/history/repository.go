package history

import (
	"context"
	"database/sql"
	"fmt"
)

type Repository interface {
	List(
		ctx context.Context,
		input ListInput,
	) (*ListResult, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(
	ctx context.Context,
	input ListInput,
) (*ListResult, error) {
	// Fetch one extra record to determine whether another page exists.
	fetchLimit := input.Limit + 1

	rows, err := r.db.QueryContext(
		ctx,
		`
	WITH owned_wallet AS (
    SELECT id, currency
    FROM wallets
    WHERE id = $1
      AND user_id = $2
),
wallet_transactions AS (

    -- 1. Original transfers involving this wallet.
    SELECT
        t.id,
        CASE
            WHEN t.sender_wallet_id = ow.id
                THEN 'transfer_sent'
            ELSE 'transfer_received'
        END AS transaction_type,
        t.amount_minor,
        t.currency,
        t.status,
        CASE
            WHEN t.sender_wallet_id = ow.id
                THEN t.receiver_wallet_id
            ELSE t.sender_wallet_id
        END AS counterparty_wallet_id,
        t.created_at
    FROM transfers t
    JOIN owned_wallet ow
      ON t.sender_wallet_id = ow.id
      OR t.receiver_wallet_id = ow.id

    UNION ALL

    -- 2. Original deposits.
    SELECT
        d.id,
        'deposit' AS transaction_type,
        d.amount_minor,
        d.currency,
        d.status,
        NULL::BIGINT AS counterparty_wallet_id,
        d.created_at
    FROM deposits d
    JOIN owned_wallet ow
      ON d.wallet_id = ow.id

    UNION ALL

    -- 3. Transfer reversals.
    -- The original sender receives money back;
    -- the original receiver has money deducted.
    SELECT
        tr.id,
        CASE
            WHEN t.sender_wallet_id = ow.id
                THEN 'transfer_reversal_received'
            ELSE 'transfer_reversal_sent'
        END AS transaction_type,
        t.amount_minor,
        t.currency,
        'posted' AS status,
        CASE
            WHEN t.sender_wallet_id = ow.id
                THEN t.receiver_wallet_id
            ELSE t.sender_wallet_id
        END AS counterparty_wallet_id,
        tr.created_at
    FROM transfer_reversals tr
    JOIN transfers t
      ON t.id = tr.transfer_id
    JOIN owned_wallet ow
      ON t.sender_wallet_id = ow.id
      OR t.receiver_wallet_id = ow.id

    UNION ALL

    -- 4. Deposit reversals.
    SELECT
        dr.id,
        'deposit_reversal' AS transaction_type,
        d.amount_minor,
        d.currency,
        'posted' AS status,
        NULL::BIGINT AS counterparty_wallet_id,
        dr.created_at
    FROM deposit_reversals dr
    JOIN deposits d
      ON d.id = dr.deposit_id
    JOIN owned_wallet ow
      ON d.wallet_id = ow.id
)
SELECT
    id,
    transaction_type,
    amount_minor,
    currency,
    status,
    counterparty_wallet_id,
    created_at
FROM wallet_transactions
ORDER BY
    created_at DESC,
    id DESC,
    transaction_type DESC
LIMIT $3 OFFSET $4;
		`,
		input.WalletID,
		input.UserID,
		fetchLimit,
		input.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query wallet transactions: %w", err)
	}
	defer rows.Close()

	result := &ListResult{
		WalletID:     input.WalletID,
		Transactions: make([]Transaction, 0, input.Limit),
		Limit:        input.Limit,
		Offset:       input.Offset,
	}

	for rows.Next() {
		var item Transaction

		err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.AmountMinor,
			&item.Currency,
			&item.Status,
			&item.CounterpartyWalletID,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan wallet transaction: %w", err)
		}

		result.Transactions = append(result.Transactions, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate wallet transactions: %w", err)
	}

	if len(result.Transactions) > input.Limit {
		result.HasMore = true
		result.Transactions = result.Transactions[:input.Limit]
	}

	return result, nil
}
