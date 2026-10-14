CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE TABLE wallets (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    currency CHAR(3) NOT NULL DEFAULT 'INR',
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'frozen', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, currency)
);

CREATE TABLE transfers (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sender_wallet_id BIGINT NOT NULL REFERENCES wallets(id),
    receiver_wallet_id BIGINT NOT NULL REFERENCES wallets(id),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL DEFAULT 'posted'
        CHECK (status IN ('posted', 'reversed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (sender_wallet_id <> receiver_wallet_id),
    FOREIGN KEY (sender_wallet_id, currency)
        REFERENCES wallets(id, currency),
    FOREIGN KEY (receiver_wallet_id, currency)
        REFERENCES wallets(id, currency)
);

CREATE TABLE ledger_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transfer_id BIGINT NOT NULL REFERENCES transfers(id),
    wallet_id BIGINT NOT NULL REFERENCES wallets(id),
    entry_type TEXT NOT NULL
        CHECK (entry_type IN ('debit', 'credit')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_wallets_user_id
    ON wallets(user_id);

CREATE INDEX idx_transfers_sender_created
    ON transfers(sender_wallet_id, created_at DESC);

CREATE INDEX idx_transfers_receiver_created
    ON transfers(receiver_wallet_id, created_at DESC);

CREATE INDEX idx_ledger_wallet_id
    ON ledger_entries(wallet_id);

CREATE INDEX idx_ledger_transfer_id
    ON ledger_entries(transfer_id);