package history

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidUserID   = errors.New("user ID must be positive")
	ErrInvalidWalletID = errors.New("wallet ID must be positive")
	ErrInvalidLimit    = errors.New("limit must be between 1 and 100")
	ErrInvalidOffset   = errors.New("offset cannot be negative")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(
	ctx context.Context,
	input ListInput,
) (*ListResult, error) {
	if input.UserID <= 0 {
		return nil, ErrInvalidUserID
	}

	if input.WalletID <= 0 {
		return nil, ErrInvalidWalletID
	}

	// Default page size.
	if input.Limit == 0 {
		input.Limit = 20
	}

	if input.Limit < 1 || input.Limit > 100 {
		return nil, ErrInvalidLimit
	}

	if input.Offset < 0 {
		return nil, ErrInvalidOffset
	}

	result, err := s.repo.List(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list wallet history: %w", err)
	}

	return result, nil
}
