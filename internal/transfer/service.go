package transfer

import (
	"context"
	"errors"
	"fmt"
)
var (
	ErrInvalidWalletID = errors.New("wallet IDs must be positive")
	ErrSameWallet      = errors.New("sender and receiver must differ")
	ErrInvalidAmount   = errors.New("amount must be positive")
	ErrInvalidCurrency = errors.New("currency must contain three letters")
)


type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}
func (s *Service) CreateTransfer(
	ctx context.Context,
	input CreateTransferInput,
) (*Transfer, error) {
	if input.SenderWalletID <= 0 || input.ReceiverWalletID <= 0 {
		return nil, ErrInvalidWalletID
	}


	if input.SenderWalletID == input.ReceiverWalletID {
		return nil, ErrSameWallet
	}
		if input.AmountMinor <= 0 {
		return nil, ErrInvalidAmount
	}

	if len(input.Currency) != 3 {
		return nil, ErrInvalidCurrency
	}

	for _, ch := range input.Currency {
		if ch < 'A' || ch > 'Z' {
			return nil, ErrInvalidCurrency
		}
	}
transfer, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create transfer: %w", err)
	}

	return transfer, nil
}