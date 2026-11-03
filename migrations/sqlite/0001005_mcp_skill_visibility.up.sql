-- MCP service and skill catalogue visibility (SQLite flavour of 000140).
--
-- See the PostgreSQL migration for what "public" means on each table: MCP
-- grants runtime usability against the owning workspace's credentials (which
-- are never handed to the caller), a skill catalogue row grants discovery and
-- installation into the caller's own sandbox.

ALTER TABLE mcp_services
    ADD COLUMN visibility VARCHAR(16) NOT NULL DEFAULT 'workspace';

ALTER TABLE mcp_services
    ADD COLUMN created_by VARCHAR(36) NOT NULL DEFAULT '';

ALTER TABLE tenant_skill_catalog
    ADD COLUMN visibility VARCHAR(16) NOT NULL DEFAULT 'workspace';

ALTER TABLE tenant_skill_catalog
    ADD COLUMN created_by VARCHAR(36) NOT NULL DEFAULT '';

-- SQLite has no ADD CONSTRAINT, so the vocabulary is enforced with triggers.
CREATE TRIGGER IF NOT EXISTS trg_mcp_services_visibility_insert
BEFORE INSERT ON mcp_services
WHEN NEW.visibility NOT IN ('private', 'workspace', 'public')
BEGIN
    SELECT RAISE(ABORT, 'mcp_services.visibility must be private, workspace or public');
END;

CREATE TRIGGER IF NOT EXISTS trg_mcp_services_visibility_update
BEFORE UPDATE OF visibility ON mcp_services
WHEN NEW.visibility NOT IN ('private', 'workspace', 'public')
BEGIN
    SELECT RAISE(ABORT, 'mcp_services.visibility must be private, workspace or public');
END;

CREATE TRIGGER IF NOT EXISTS trg_tenant_skill_catalog_visibility_insert
BEFORE INSERT ON tenant_skill_catalog
WHEN NEW.visibility NOT IN ('private', 'workspace', 'public')
BEGIN
    SELECT RAISE(ABORT, 'tenant_skill_catalog.visibility must be private, workspace or public');
END;

CREATE TRIGGER IF NOT EXISTS trg_tenant_skill_catalog_visibility_update
BEFORE UPDATE OF visibility ON tenant_skill_catalog
WHEN NEW.visibility NOT IN ('private', 'workspace', 'public')
BEGIN
    SELECT RAISE(ABORT, 'tenant_skill_catalog.visibility must be private, workspace or public');
END;

CREATE INDEX IF NOT EXISTS idx_mcp_services_visibility
    ON mcp_services(visibility, tenant_id);

CREATE INDEX IF NOT EXISTS idx_tenant_skill_catalog_visibility
    ON tenant_skill_catalog(visibility, tenant_id);
