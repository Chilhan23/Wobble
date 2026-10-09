-- 000004_create_messages_table.down.sql
DROP INDEX IF EXISTS idx_messages_outbox;
DROP INDEX IF EXISTS uq_messages_client_msg;
DROP INDEX IF EXISTS idx_messages_ticket_id_id;
DROP TABLE IF EXISTS messages;
