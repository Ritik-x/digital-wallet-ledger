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

func (s *Service) ConfirmPayment(
	ctx context.Context,
	depositID int64,
	amountMinor int64,
	currency string,
) error {
	if depositID <= 0 {
		return errors.New("invalid deposit ID")
	}

	if amountMinor <= 0 {
		return errors.New("amount must be positive")
	}

	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return errors.New("currency must be a three-letter code")
	}

	for _, ch := range currency {
		if ch < 'A' || ch > 'Z' {
			return errors.New("currency must contain only letters")
		}
	}

	return s.repo.ConfirmPayment(
		ctx,
		depositID,
		amountMinor,
		currency,
	)
}
