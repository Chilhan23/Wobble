-- 000005_create_telegram_updates_table.up.sql
CREATE TABLE telegram_updates (
    update_id   BIGINT      PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_telegram_updates_received_at ON telegram_updates (received_at);
