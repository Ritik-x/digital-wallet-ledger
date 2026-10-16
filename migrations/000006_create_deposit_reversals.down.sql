DROP INDEX IF EXISTS idx_ledger_deposit_reversal_id;

ALTER TABLE ledger_entries
DROP CONSTRAINT ledger_entry_one_operation;

ALTER TABLE ledger_entries
DROP COLUMN deposit_reversal_id;

ALTER TABLE ledger_entries
ADD CONSTRAINT ledger_entry_one_operation CHECK (
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

DROP TABLE deposit_reversals;