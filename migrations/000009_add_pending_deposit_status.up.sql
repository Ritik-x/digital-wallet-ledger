ALTER TABLE deposits
    ALTER COLUMN status DROP DEFAULT;

ALTER TABLE deposits
    DROP CONSTRAINT deposits_status_check;

ALTER TABLE deposits
    ADD CONSTRAINT deposits_status_check
    CHECK (status IN ('pending', 'posted', 'reversed'));

ALTER TABLE deposits
    ALTER COLUMN status SET DEFAULT 'pending';
    