-- 000003_create_tickets_table.down.sql
DROP TRIGGER IF EXISTS trg_tickets_updated_at ON tickets;
DROP INDEX IF EXISTS idx_tickets_autoclose;
DROP INDEX IF EXISTS idx_tickets_tenant_status;
DROP INDEX IF EXISTS uq_tickets_active_per_user;
DROP INDEX IF EXISTS uq_tickets_thread_id;
DROP TABLE IF EXISTS tickets;
