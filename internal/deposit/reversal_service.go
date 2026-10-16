package deposit

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidReversalUserID = errors.New("user ID must be positive")
	ErrInvalidReversalDepositID = errors.New("deposit ID must be positive")
	ErrInvalidDepositReversalKey = errors.New("valid idempotency key is required")
)

type ReversalService struct {
	repo ReversalRepository
}

func NewReversalService(repo ReversalRepository) *ReversalService {
	return &ReversalService{
		repo: repo,
	}
}

func (s *ReversalService) Reverse(
	ctx context.Context,
	input ReverseDepositInput,
) (*DepositReversal, error) {
	if input.UserID <= 0 {
		return nil, ErrInvalidReversalUserID
	}

	if input.DepositID <= 0 {
		return nil, ErrInvalidReversalDepositID
	}

	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)

	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return nil, ErrInvalidDepositReversalKey
	}

	result, err := s.repo.Reverse(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("reverse deposit: %w", err)
	}

	return result, nil
}
