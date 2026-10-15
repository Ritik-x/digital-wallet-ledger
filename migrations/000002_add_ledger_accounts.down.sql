DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM ledger_entries le
        JOIN ledger_accounts la
            ON la.id = le.ledger_account_id
        WHERE la.account_type = 'system'
    ) THEN
        RAISE EXCEPTION
            'Cannot roll back: system ledger entries exist';
    END IF;
END $$;

-- Restore the original wallet reference.
ALTER TABLE ledger_entries
ADD COLUMN wallet_id BIGINT;

UPDATE ledger_entries AS le
SET wallet_id = la.wallet_id
FROM ledger_accounts AS la
WHERE la.id = le.ledger_account_id;

ALTER TABLE ledger_entries
ALTER COLUMN wallet_id SET NOT NULL;

ALTER TABLE ledger_entries
ADD CONSTRAINT fk_ledger_entries_wallet
FOREIGN KEY (wallet_id)
REFERENCES wallets(id);

CREATE INDEX idx_ledger_wallet_id
ON ledger_entries(wallet_id);

-- Remove the new ledger-account reference.
DROP INDEX idx_ledger_account_id;

ALTER TABLE ledger_entries
DROP CONSTRAINT fk_ledger_entries_account;

ALTER TABLE ledger_entries
DROP COLUMN ledger_account_id;

DROP TABLE ledger_accounts;
