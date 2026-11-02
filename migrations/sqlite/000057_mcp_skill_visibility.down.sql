DROP TRIGGER IF EXISTS trg_mcp_services_visibility_insert;
DROP TRIGGER IF EXISTS trg_mcp_services_visibility_update;
DROP TRIGGER IF EXISTS trg_tenant_skill_catalog_visibility_insert;
DROP TRIGGER IF EXISTS trg_tenant_skill_catalog_visibility_update;
DROP INDEX IF EXISTS idx_mcp_services_visibility;
DROP INDEX IF EXISTS idx_tenant_skill_catalog_visibility;

ALTER TABLE mcp_services DROP COLUMN visibility;
ALTER TABLE mcp_services DROP COLUMN created_by;
ALTER TABLE tenant_skill_catalog DROP COLUMN visibility;
ALTER TABLE tenant_skill_catalog DROP COLUMN created_by;
