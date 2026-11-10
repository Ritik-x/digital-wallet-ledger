
-- Track the lifecycle of Razorpay order creation independently
-- from the financial status of a LedgerX deposit.

ALTER TABLE deposits
ADD COLUMN razorpay_order_status TEXT NOT NULL DEFAULT 'not_started';

-- Only allow known order-creation states.
ALTER TABLE deposits
ADD CONSTRAINT deposits_razorpay_order_status_check
CHECK (
    razorpay_order_status IN (
        'not_started',
        'in_progress',
        'created',
        'unknown'
    )
);

-- A successfully created Razorpay order must have an order ID.
ALTER TABLE deposits
ADD CONSTRAINT deposits_razorpay_order_created_requires_id
CHECK (
    razorpay_order_status <> 'created'
    OR razorpay_order_id IS NOT NULL
);
