package deposit

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidWalletID    = errors.New("wallet ID must be positive")
	ErrInvalidAmount      = errors.New("deposit amount must be positive")
	ErrInvalidCurrency    = errors.New("currency must contain three letters")
	ErrInvalidIdempotency = errors.New("idempotency key is required")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(
	ctx context.Context,
	input CreateDepositInput,
) (*Deposit, error) {
	if input.WalletID <= 0 {
		return nil, ErrInvalidWalletID
	}

	if input.AmountMinor <= 0 {
		return nil, ErrInvalidAmount
	}

	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(input.Currency) != 3 {
		return nil, ErrInvalidCurrency
	}

	for _, ch := range input.Currency {
		if ch < 'A' || ch > 'Z' {
			return nil, ErrInvalidCurrency
		}
	}

	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return nil, ErrInvalidIdempotency
	}

	return s.repo.Create(ctx, input)
}
