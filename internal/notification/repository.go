package notification

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type RecipientRepository struct {
	db *sql.DB
}

func NewRecipientRepository(db *sql.DB) *RecipientRepository {
	return &RecipientRepository{db: db}
}

func (r *RecipientRepository) GetWalletOwnerEmail(
	ctx context.Context,
	walletID int64,
) (string, error) {
	if walletID <= 0 {
		return "", fmt.Errorf("invalid wallet ID")
	}

	var email string

	err := r.db.QueryRowContext(
		ctx,
		`
		SELECT u.email
		FROM wallets w
		JOIN users u ON u.id = w.user_id
		WHERE w.id = $1
		`,
		walletID,
	).Scan(&email)

	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("wallet %d or its owner was not found", walletID)
		}
		return "", fmt.Errorf("lookup wallet owner email: %w", err)
	}

	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("wallet owner email is empty")
	}

	return email, nil
}
