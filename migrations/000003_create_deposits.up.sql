CREATE TABLE deposits (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    wallet_id BIGINT NOT NULL
        REFERENCES wallets(id),

    amount_minor BIGINT NOT NULL
        CHECK (amount_minor > 0),

    currency CHAR(3) NOT NULL,

    status TEXT NOT NULL DEFAULT 'posted'
        CHECK (status IN ('posted', 'reversed')),

    idempotency_key TEXT NOT NULL UNIQUE
        CHECK (length(trim(idempotency_key)) > 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (wallet_id, currency)
        REFERENCES wallets(id, currency)
);

ALTER TABLE ledger_entries
    ALTER COLUMN transfer_id DROP NOT NULL;

ALTER TABLE ledger_entries
    ADD COLUMN deposit_id BIGINT
        REFERENCES deposits(id);

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entry_one_operation
    CHECK (
        (transfer_id IS NOT NULL AND deposit_id IS NULL)
        OR
        (transfer_id IS NULL AND deposit_id IS NOT NULL)
    );

CREATE INDEX idx_deposits_wallet_created
    ON deposits(wallet_id, created_at DESC);

CREATE INDEX idx_ledger_deposit_id
    ON ledger_entries(deposit_id);
