
ALTER TABLE deposits
DROP CONSTRAINT IF EXISTS deposits_razorpay_order_created_requires_id;

ALTER TABLE deposits
DROP CONSTRAINT IF EXISTS deposits_razorpay_order_status_check;

ALTER TABLE deposits
DROP COLUMN IF EXISTS razorpay_order_status;
