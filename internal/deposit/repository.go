package deposit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletUnavailable   = errors.New("wallet is not active")
	ErrCurrencyMismatch    = errors.New("currency mismatch")
	ErrIdempotencyConflict = errors.New(
		"idempotency key already used for another request",
	)
	// ErrDepositNotFound  = errors.New("deposit not found")
	ErrDepositNotPending = errors.New("deposit is not pending")
	ErrPaymentMismatch   = errors.New("payment details do not match deposit")
)

type Repository interface {
	Create(
		ctx context.Context,
		input CreateDepositInput,
	) (*Deposit, error)

	StartRazorpayOrder(
		ctx context.Context,
		depositID int64,
	) (bool, error)

	SaveRazorpayOrder(
		ctx context.Context,
		depositID int64,
		orderID string,
	) error

	MarkRazorpayOrderUnknown(
		ctx context.Context,
		depositID int64,
	) error

	ConfirmPayment(
		ctx context.Context,
		depositID int64,
		razorpayOrderID string,
		razorpayPaymentID string,
		amountMinor int64,
		currency string,
	) error
}
type PostgresRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	input CreateDepositInput,
) (*Deposit, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin deposit transaction: %w", err)
	}
	defer tx.Rollback()

	// Check whether this idempotency key was already processed.
	existing, err := findDepositByKey(
		ctx,
		tx,
		input.IdempotencyKey,
	)
	if err == nil {
		if existing.WalletID != input.WalletID ||
			existing.AmountMinor != input.AmountMinor ||
			strings.TrimSpace(existing.Currency) != input.Currency {
			return nil, ErrIdempotencyConflict
		}

		return existing, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing deposit: %w", err)
	}

	// Lock the wallet and verify that it is active.
	var walletCurrency string
	var walletStatus string

	err = tx.QueryRowContext(
		ctx,
		`SELECT currency, status
		 FROM wallets
		 WHERE id = $1
		 FOR UPDATE`,
		input.WalletID,
	).Scan(&walletCurrency, &walletStatus)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWalletNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock deposit wallet: %w", err)
	}

	if walletStatus != "active" {
		return nil, ErrWalletUnavailable
	}

	if strings.TrimSpace(walletCurrency) != input.Currency {
		return nil, ErrCurrencyMismatch
	}

	// Create a pending deposit.
	// No ledger entries are created at this stage.
	var created Deposit

	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO deposits (
			wallet_id,
			amount_minor,
			currency,
			status,
			idempotency_key
		)
		VALUES ($1, $2, $3, 'pending', $4)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, wallet_id, amount_minor, currency,
		          status, idempotency_key, created_at`,
		input.WalletID,
		input.AmountMinor,
		input.Currency,
		input.IdempotencyKey,
	).Scan(
		&created.ID,
		&created.WalletID,
		&created.AmountMinor,
		&created.Currency,
		&created.Status,
		&created.IdempotencyKey,
		&created.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		// Another request may have inserted the same key concurrently.
		existing, lookupErr := findDepositByKey(
			ctx,
			tx,
			input.IdempotencyKey,
		)
		if lookupErr != nil {
			return nil, fmt.Errorf(
				"retrieve concurrent deposit: %w",
				lookupErr,
			)
		}

		if existing.WalletID != input.WalletID ||
			existing.AmountMinor != input.AmountMinor ||
			strings.TrimSpace(existing.Currency) != input.Currency {
			return nil, ErrIdempotencyConflict
		}

		return existing, nil
	}

	if err != nil {
		return nil, fmt.Errorf("insert deposit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit deposit: %w", err)
	}

	return &created, nil
}
func (r *PostgresRepository) StartRazorpayOrder(
	ctx context.Context,
	depositID int64,
) (bool, error) {

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE deposits
         SET razorpay_order_status = 'in_progress'
         WHERE id = $1
           AND status = 'pending'
           AND razorpay_order_status = 'not_started'`,
		depositID,
	)

	if err != nil {
		return false, fmt.Errorf(
			"claim razorpay order creation: %w",
			err,
		)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf(
			"check razorpay order claim: %w",
			err,
		)
	}

	return rowsAffected == 1, nil
}
func (r *PostgresRepository) SaveRazorpayOrder(
	ctx context.Context,
	depositID int64,
	orderID string,
) error {

	orderID = strings.TrimSpace(orderID)

	if depositID <= 0 {
		return fmt.Errorf("invalid deposit ID")
	}

	if orderID == "" {
		return fmt.Errorf("razorpay order ID is required")
	}

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE deposits
         SET razorpay_order_id = $1,
             razorpay_order_status = 'created'
         WHERE id = $2
           AND status = 'pending'
           AND razorpay_order_status = 'in_progress'
           AND razorpay_order_id IS NULL`,
		orderID,
		depositID,
	)

	if err != nil {
		return fmt.Errorf("save razorpay order: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check razorpay order update: %w", err)
	}

	if rowsAffected != 1 {
		return fmt.Errorf(
			"deposit is not eligible for razorpay order update",
		)
	}

	return nil
}

func (r *PostgresRepository) MarkRazorpayOrderUnknown(
	ctx context.Context,
	depositID int64,
) error {

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE deposits
         SET razorpay_order_status = 'unknown'
         WHERE id = $1
           AND status = 'pending'
           AND razorpay_order_status = 'in_progress'
           AND razorpay_order_id IS NULL`,
		depositID,
	)

	if err != nil {
		return fmt.Errorf(
			"mark razorpay order as unknown: %w",
			err,
		)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(
			"check unknown razorpay order update: %w",
			err,
		)
	}

	if rowsAffected != 1 {
		return fmt.Errorf(
			"deposit is not eligible for unknown razorpay order status",
		)
	}

	return nil
}
func findDepositByKey(
	ctx context.Context,
	tx *sql.Tx,
	key string,
) (*Deposit, error) {
	var d Deposit

	err := tx.QueryRowContext(
		ctx,
		`SELECT id, wallet_id, amount_minor, currency,
       status, idempotency_key, created_at,
       razorpay_order_id, razorpay_order_status
FROM deposits
WHERE idempotency_key = $1`,
		key,
	).Scan(
		&d.ID,
		&d.WalletID,
		&d.AmountMinor,
		&d.Currency,
		&d.Status,
		&d.IdempotencyKey,
		&d.CreatedAt,
		&d.RazorpayOrderID,
		&d.RazorpayOrderStatus,
	)

	if err != nil {
		return nil, err
	}

	return &d, nil
}

func (r *PostgresRepository) ConfirmPayment(
	ctx context.Context,
	depositID int64,
	razorpayOrderID string,
	razorpayPaymentID string,
	amountMinor int64,
	currency string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin payment confirmation: %w", err)
	}
	defer tx.Rollback()

	// Lock the deposit so concurrent webhooks cannot post it twice.
	var walletID int64
	var depositAmount int64
	var depositCurrency string
	var status string
	var storedOrderID sql.NullString
	var storedPaymentID sql.NullString

	err = tx.QueryRowContext(
		ctx,
		`SELECT wallet_id,
            amount_minor,
            currency,
            status,
            razorpay_order_id,
            razorpay_payment_id
     FROM deposits
     WHERE id = $1
     FOR UPDATE`,
		depositID,
	).Scan(
		&walletID,
		&depositAmount,
		&depositCurrency,
		&status,
		&storedOrderID,
		&storedPaymentID,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrDepositNotFound
	}
	if err != nil {
		return fmt.Errorf("lock deposit: %w", err)
	}
	// Verify that this payment belongs to the Razorpay order
	// created for this LedgerX deposit.
	if !storedOrderID.Valid ||
		storedOrderID.String != razorpayOrderID ||
		strings.TrimSpace(razorpayPaymentID) == "" {
		return ErrPaymentMismatch
	}
	// Never trust the webhook's amount or currency alone.
	if depositAmount != amountMinor ||
		strings.TrimSpace(depositCurrency) != strings.TrimSpace(currency) {
		return ErrPaymentMismatch
	}

	// A duplicate delivery must never create duplicate ledger entries.
	if status == "posted" {
		if storedPaymentID.Valid &&
			storedPaymentID.String == razorpayPaymentID {
			return nil
		}

		return ErrPaymentMismatch
	}
	if status != "pending" {
		return ErrDepositNotPending
	}

	// Lock and validate the wallet.
	var walletStatus string
	var walletCurrency string

	err = tx.QueryRowContext(
		ctx,
		`SELECT status, currency
		 FROM wallets
		 WHERE id = $1
		 FOR UPDATE`,
		walletID,
	).Scan(&walletStatus, &walletCurrency)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrWalletNotFound
	}
	if err != nil {
		return fmt.Errorf("lock deposit wallet: %w", err)
	}
	if walletStatus != "active" {
		return ErrWalletUnavailable
	}
	if strings.TrimSpace(walletCurrency) != strings.TrimSpace(depositCurrency) {
		return ErrCurrencyMismatch
	}

	var clearingAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		 FROM ledger_accounts
		 WHERE account_type = 'system'
		   AND code = 'external_clearing'
		   AND currency = $1
		   AND status = 'active'`,
		depositCurrency,
	).Scan(&clearingAccountID)

	if err != nil {
		return fmt.Errorf("find clearing account: %w", err)
	}

	// Find the active ledger account belonging to the wallet.
	var walletAccountID int64

	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		 FROM ledger_accounts
		 WHERE wallet_id = $1
		   AND currency = $2
		   AND status = 'active'`,
		walletID,
		depositCurrency,
	).Scan(&walletAccountID)

	if err != nil {
		return fmt.Errorf("find wallet ledger account: %w", err)
	}

	// Debit the external clearing account.
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO ledger_entries (
			deposit_id, ledger_account_id, entry_type, amount_minor
		)
		VALUES ($1, $2, 'debit', $3)`,
		depositID,
		clearingAccountID,
		depositAmount,
	)
	if err != nil {
		return fmt.Errorf("insert clearing debit: %w", err)
	}

	// Credit the customer's wallet account.
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO ledger_entries (
			deposit_id, ledger_account_id, entry_type, amount_minor
		)
		VALUES ($1, $2, 'credit', $3)`,
		depositID,
		walletAccountID,
		depositAmount,
	)
	if err != nil {
		return fmt.Errorf("insert wallet credit: %w", err)
	}

	// Mark the deposit posted in the same transaction.
	result, err := tx.ExecContext(
		ctx,
		`UPDATE deposits
 SET status = 'posted',
     razorpay_payment_id = $2
 WHERE id = $1
   AND status = 'pending'
   AND razorpay_order_id = $3`,
		depositID,
		razorpayPaymentID,
		razorpayOrderID,
	)
	if err != nil {
		return fmt.Errorf("mark deposit posted: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deposit update: %w", err)
	}
	if rowsAffected != 1 {
		return ErrDepositNotPending
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit payment confirmation: %w", err)
	}

	return nil
}
