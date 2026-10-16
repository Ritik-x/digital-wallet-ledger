package transfer

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidReversalUserID     = errors.New("user ID must be positive")
	ErrInvalidReversalTransferID = errors.New("transfer ID must be positive")
	ErrInvalidReversalKey        = errors.New("valid idempotency key is required")
)

type ReversalService struct {
	repo ReversalRepository
}

func NewReversalService(repo ReversalRepository) *ReversalService {
	return &ReversalService{repo: repo}
}

func (s *ReversalService) Reverse(
	ctx context.Context,
	input ReverseTransferInput,
) (*TransferReversal, error) {
	if input.UserID <= 0 {
		return nil, ErrInvalidReversalUserID
	}

	if input.TransferID <= 0 {
		return nil, ErrInvalidReversalTransferID
	}

	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return nil, ErrInvalidReversalKey
	}

	result, err := s.repo.Reverse(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("reverse transfer: %w", err)
	}

	return result, nil
}
