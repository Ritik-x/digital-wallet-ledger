package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidUserID   = errors.New("user ID must be positive")
	ErrInvalidWalletID = errors.New("wallet ID must be positive")
	ErrInvalidCurrency = errors.New("currency must contain three letters")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateWallet(
	ctx context.Context,
	input CreateWalletInput,
) (*Wallet, error) {
	if input.UserID <= 0 {
		return nil, ErrInvalidUserID
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

	wallet, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}

	return wallet, nil
}

func (s *Service) GetBalance(
	ctx context.Context,
	walletID int64,
) (*Balance, error) {
	if walletID <= 0 {
		return nil, ErrInvalidWalletID
	}

	balance, err := s.repo.GetBalance(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("get wallet balance: %w", err)
	}

	return balance, nil
}
