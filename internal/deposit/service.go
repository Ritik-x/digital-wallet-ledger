package deposit

import (
	"context"
	"digital-wallet/internal/razorpay"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidWalletID           = errors.New("wallet ID must be positive")
	ErrInvalidAmount             = errors.New("deposit amount must be positive")
	ErrInvalidCurrency           = errors.New("currency must contain three letters")
	ErrInvalidIdempotency        = errors.New("idempotency key is required")
	ErrRazorpayClientUnavailable = errors.New("razorpay client is unavailable")
	ErrRazorpayOrderInProgress   = errors.New("razorpay order creation already in progress")
	ErrRazorpayOrderUncertain    = errors.New("razorpay order creation outcome is uncertain")
	ErrRazorpayOrderNotCreated   = errors.New("razorpay order could not be created")
)

type Service struct {
	repo           Repository
	razorpayClient *razorpay.Client
}
type CreateRazorpayOrderResult struct {
	Deposit  *Deposit `json:"deposit"`
	OrderID  string   `json:"razorpay_order_id"`
	KeyID    string   `json:"razorpay_key_id"`
	Amount   int64    `json:"amount"`
	Currency string   `json:"currency"`
}

func NewService(repo Repository, razorpayClient *razorpay.Client) *Service {
	return &Service{repo: repo, razorpayClient: razorpayClient}
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
func (s *Service) CreateRazorpayOrder(
	ctx context.Context,
	input CreateDepositInput,
) (*CreateRazorpayOrderResult, error) {

	if s.razorpayClient == nil {
		return nil, ErrRazorpayClientUnavailable
	}

	// Step 1: Validate and create or retrieve the pending deposit.
	deposit, err := s.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	// Step 2: If an order already exists, reuse it.
	if deposit.RazorpayOrderStatus == "created" &&
		deposit.RazorpayOrderID != "" {

		return &CreateRazorpayOrderResult{
			Deposit:  deposit,
			OrderID:  deposit.RazorpayOrderID,
			KeyID:    s.razorpayClient.KeyID(),
			Amount:   deposit.AmountMinor,
			Currency: deposit.Currency,
		}, nil
	}

	// Step 3: Never blindly retry uncertain or active order creation.
	switch deposit.RazorpayOrderStatus {
	case "in_progress":
		return nil, ErrRazorpayOrderInProgress

	case "unknown":
		return nil, ErrRazorpayOrderUncertain

	case "created":
		return nil, ErrRazorpayOrderNotCreated
	}

	// Step 4: Atomically claim order creation.
	claimed, err := s.repo.StartRazorpayOrder(ctx, deposit.ID)
	if err != nil {
		return nil, fmt.Errorf("claim razorpay order: %w", err)
	}

	if !claimed {
		return nil, ErrRazorpayOrderInProgress
	}

	// Step 5: Ask Razorpay to create the order.
	order, err := s.razorpayClient.CreateOrder(
		ctx,
		razorpay.CreateOrderRequest{
			Amount:   deposit.AmountMinor,
			Currency: deposit.Currency,
			Receipt:  fmt.Sprintf("dep_%d", deposit.ID),
			Notes: map[string]string{
				"deposit_id": fmt.Sprintf("%d", deposit.ID),
			},
		},
	)

	if err != nil {
		markErr := s.repo.MarkRazorpayOrderUnknown(ctx, deposit.ID)
		if markErr != nil {
			return nil, fmt.Errorf(
				"razorpay order outcome uncertain: %w; marking unknown also failed: %v",
				err,
				markErr,
			)
		}

		return nil, fmt.Errorf(
			"%w: %v",
			ErrRazorpayOrderUncertain,
			err,
		)
	}

	// Step 6: Persist the Razorpay order ID.
	if err := s.repo.SaveRazorpayOrder(
		ctx,
		deposit.ID,
		order.ID,
	); err != nil {
		markErr := s.repo.MarkRazorpayOrderUnknown(ctx, deposit.ID)
		if markErr != nil {
			return nil, fmt.Errorf(
				"save razorpay order failed: %w; marking unknown also failed: %v",
				err,
				markErr,
			)
		}

		return nil, fmt.Errorf(
			"save razorpay order: %w",
			err,
		)
	}

	// Update the in-memory object to reflect the persisted state.
	deposit.RazorpayOrderID = order.ID
	deposit.RazorpayOrderStatus = "created"

	return &CreateRazorpayOrderResult{
		Deposit:  deposit,
		OrderID:  order.ID,
		KeyID:    s.razorpayClient.KeyID(),
		Amount:   order.Amount,
		Currency: order.Currency,
	}, nil
}
func (s *Service) ConfirmPayment(
	ctx context.Context,
	depositID int64,
	razorpayOrderID string,
	razorpayPaymentID string,
	amountMinor int64,
	currency string,
) error {
	if depositID <= 0 {
		return errors.New("invalid deposit ID")
	}

	razorpayOrderID = strings.TrimSpace(razorpayOrderID)
	razorpayPaymentID = strings.TrimSpace(razorpayPaymentID)

	if razorpayOrderID == "" {
		return errors.New("Razorpay order ID is required")
	}

	if razorpayPaymentID == "" {
		return errors.New("Razorpay payment ID is required")
	}

	if amountMinor <= 0 {
		return errors.New("amount must be positive")
	}

	currency = strings.ToUpper(strings.TrimSpace(currency))

	if len(currency) != 3 {
		return errors.New("invalid currency")
	}

	for _, ch := range currency {
		if ch < 'A' || ch > 'Z' {
			return errors.New("invalid currency")
		}
	}

	return s.repo.ConfirmPayment(
		ctx,
		depositID,
		razorpayOrderID,
		razorpayPaymentID,
		amountMinor,
		currency,
	)
}
