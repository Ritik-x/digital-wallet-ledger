DROP TABLE IF EXISTS razorpay_webhook_events;

ALTER TABLE deposits
    DROP CONSTRAINT IF EXISTS deposits_razorpay_payment_requires_order;

DROP INDEX IF EXISTS idx_deposits_razorpay_payment_id;
DROP INDEX IF EXISTS idx_deposits_razorpay_order_id;

ALTER TABLE deposits
    DROP COLUMN IF EXISTS razorpay_payment_id,
    DROP COLUMN IF EXISTS razorpay_order_id;
