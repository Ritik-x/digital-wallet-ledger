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
			SELECT
				t.id,
				CASE
					WHEN t.sender_wallet_id = w.id
						THEN 'transfer_sent'
					ELSE 'transfer_received'
				END AS transaction_type,
				t.amount_minor,
				t.currency,
				t.status,
				CASE
					WHEN t.sender_wallet_id = w.id
						THEN t.receiver_wallet_id
					ELSE t.sender_wallet_id
				END AS counterparty_wallet_id,
				t.created_at
			FROM transfers AS t
			JOIN owned_wallet AS w
				ON t.sender_wallet_id = w.id
				OR t.receiver_wallet_id = w.id

			UNION ALL

			SELECT
				d.id,
				'deposit' AS transaction_type,
				d.amount_minor,
				d.currency,
				d.status,
				NULL::BIGINT AS counterparty_wallet_id,
				d.created_at
			FROM deposits AS d
			JOIN owned_wallet AS w
				ON d.wallet_id = w.id
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
		ORDER BY created_at DESC, id DESC, transaction_type DESC
		LIMIT $3 OFFSET $4
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
