
CREATE TABLE transfer_reversals (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    transfer_id BIGINT NOT NULL UNIQUE
        REFERENCES transfers(id),

    idempotency_key TEXT NOT NULL UNIQUE
        CHECK (length(trim(idempotency_key)) > 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Link reversal ledger entries to their reversal operation.
ALTER TABLE ledger_entries
    ADD COLUMN reversal_id BIGINT
        REFERENCES transfer_reversals(id);

-- Every ledger entry must belong to exactly one operation:

ALTER TABLE ledger_entries
    DROP CONSTRAINT ledger_entry_one_operation;

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entry_one_operation
    CHECK (
        (
            transfer_id IS NOT NULL
            AND deposit_id IS NULL
            AND reversal_id IS NULL
        )
        OR
        (
            transfer_id IS NULL
            AND deposit_id IS NOT NULL
            AND reversal_id IS NULL
        )
        OR
        (
            transfer_id IS NULL
            AND deposit_id IS NULL
            AND reversal_id IS NOT NULL
        )
    );

CREATE INDEX idx_ledger_reversal_id
    ON ledger_entries(reversal_id);
