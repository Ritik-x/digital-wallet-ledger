CREATE TABLE ledger_accounts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_type TEXT NOT NULL
        CHECK (account_type IN ('wallet', 'system')),

    wallet_id BIGINT UNIQUE REFERENCES wallets(id),

    code TEXT,

    currency CHAR(3) NOT NULL,

    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'frozen', 'closed')),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (code, currency),

    CHECK (
        (account_type = 'wallet'
            AND wallet_id IS NOT NULL
            AND code IS NULL)
        OR
        (account_type = 'system'
            AND wallet_id IS NULL
            AND code IS NOT NULL)
    )
);
INSERT INTO ledger_accounts (
    account_type,
    wallet_id,
    currency
)
SELECT
    'wallet',
    id,
    currency
FROM wallets;
ALTER TABLE ledger_entries
ADD COLUMN ledger_account_id BIGINT;

UPDATE ledger_entries AS le
SET ledger_account_id = la.id
FROM ledger_accounts AS la
WHERE la.wallet_id = le.wallet_id;

ALTER TABLE ledger_entries
ALTER COLUMN ledger_account_id SET NOT NULL;

ALTER TABLE ledger_entries
ADD CONSTRAINT fk_ledger_entries_account
FOREIGN KEY (ledger_account_id)
REFERENCES ledger_accounts(id);

CREATE INDEX idx_ledger_account_id
ON ledger_entries(ledger_account_id);

DROP INDEX idx_ledger_wallet_id;

ALTER TABLE ledger_entries
DROP COLUMN wallet_id;

-- 7. Create the counterparty account for INR deposits.
INSERT INTO ledger_accounts (
    account_type,
    code,
    currency
)
VALUES (
    'system',
    'external_clearing',
    'INR'
);