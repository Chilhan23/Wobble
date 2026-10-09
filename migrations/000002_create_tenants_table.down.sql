-- 000002_create_tenants_table.down.sql
DROP TRIGGER IF EXISTS trg_tenants_updated_at ON tenants;
DROP TABLE IF EXISTS tenants;
