
ALTER TABLE transfers
ADD COLUMN idempotency_key TEXT;

CREATE UNIQUE INDEX uq_transfers_idempotency_key
ON transfers (idempotency_key)
WHERE idempotency_key IS NOT NULL;
