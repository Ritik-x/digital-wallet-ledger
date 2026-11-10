-- Associate each LedgerX deposit with its Razorpay order and payment.
ALTER TABLE deposits
    ADD COLUMN razorpay_order_id TEXT,
    ADD COLUMN razorpay_payment_id TEXT;

-- A Razorpay order or payment must never be associated with
-- multiple LedgerX deposits.
CREATE UNIQUE INDEX idx_deposits_razorpay_order_id
    ON deposits (razorpay_order_id)
    WHERE razorpay_order_id IS NOT NULL;

CREATE UNIQUE INDEX idx_deposits_razorpay_payment_id
    ON deposits (razorpay_payment_id)
    WHERE razorpay_payment_id IS NOT NULL;

-- A payment cannot exist without its associated Razorpay order.
ALTER TABLE deposits
    ADD CONSTRAINT deposits_razorpay_payment_requires_order
    CHECK (
        razorpay_payment_id IS NULL
        OR razorpay_order_id IS NOT NULL
    );

-- Record Razorpay webhook events for deduplication.
CREATE TABLE razorpay_webhook_events (
    event_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);
