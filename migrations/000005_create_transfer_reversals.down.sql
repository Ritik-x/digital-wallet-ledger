
DROP INDEX IF EXISTS idx_ledger_reversal_id;

ALTER TABLE ledger_entries
    DROP CONSTRAINT ledger_entry_one_operation;

ALTER TABLE ledger_entries
    DROP COLUMN reversal_id;

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entry_one_operation
    CHECK (
        (transfer_id IS NOT NULL AND deposit_id IS NULL)
        OR
        (transfer_id IS NULL AND deposit_id IS NOT NULL)
    );

DROP TABLE transfer_reversals;
