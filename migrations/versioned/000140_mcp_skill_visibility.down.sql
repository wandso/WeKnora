-- Reverse of 000140. Dropping the column drops its indexes and constraints.
ALTER TABLE mcp_services DROP COLUMN IF EXISTS visibility;
ALTER TABLE mcp_services DROP COLUMN IF EXISTS created_by;
ALTER TABLE tenant_skill_catalog DROP COLUMN IF EXISTS visibility;
ALTER TABLE tenant_skill_catalog DROP COLUMN IF EXISTS created_by;
