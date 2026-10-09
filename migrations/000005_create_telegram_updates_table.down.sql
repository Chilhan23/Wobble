-- 000005_create_telegram_updates_table.down.sql
DROP INDEX IF EXISTS idx_telegram_updates_received_at;
DROP TABLE IF EXISTS telegram_updates;
