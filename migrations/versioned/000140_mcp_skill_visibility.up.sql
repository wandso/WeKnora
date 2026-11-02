-- MCP service and skill catalogue visibility: private / workspace / public.
--
-- Same three-level vocabulary as knowledge_bases (000139), but the semantics of
-- "public" differ per table and are worth spelling out here, because the same
-- word covers three different kinds of sharing.
--
-- mcp_services
--   private    the creator, plus workspace Owner/Admin
--   workspace  every member of the owning workspace (historical behaviour,
--              hence the default so existing rows keep their audience)
--   public     usable from every workspace, against the owning workspace's
--              credentials
--
--   MCP is not a read-only resource like a knowledge base: a card holds a URL
--   and an API key, and "using" it means calling an external server on someone
--   else's quota. The deployment owner asked for exactly that (share one
--   key across workspaces), so public grants runtime usability. To keep the key
--   from being copied out, the response projection still strips URL, headers
--   and auth config for any caller outside the owning workspace — the secret is
--   used, never handed over. Only the owning workspace can edit or delete.
--
-- tenant_skill_catalog
--   private    the creator, plus workspace Owner/Admin
--   workspace  every member of the owning workspace
--   public     discoverable and installable from every workspace
--
--   A skill definition is a zip plus a SKILL.md, never a running process. The
--   executable copy lives in each workspace's own sandbox image, so installing
--   a public skill still builds that workspace's own snapshot from the
--   definition. There is no credential to leak here (env values are encrypted
--   and only ever reported as "is_set"), which is why public is a plain
--   install permission rather than a runtime one.
--
-- created_by exists because neither table had a creator column: without it
-- "private" cannot be expressed. Empty string means "unknown", which for
-- private falls back to workspace Owner/Admin only.
--
-- is_builtin on mcp_services stays a separate axis. A builtin row is shipped
-- with the product, is visible to every workspace, cannot be edited, and always
-- has its connection details stripped. It is effectively public, but a
-- different kind: nobody owns it. Rows created by users start at workspace.

ALTER TABLE mcp_services
    ADD COLUMN IF NOT EXISTS visibility VARCHAR(16) NOT NULL DEFAULT 'workspace';

ALTER TABLE mcp_services
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

ALTER TABLE mcp_services
    DROP CONSTRAINT IF EXISTS mcp_services_visibility_check;
ALTER TABLE mcp_services
    ADD CONSTRAINT mcp_services_visibility_check
    CHECK (visibility IN ('private', 'workspace', 'public'));

CREATE INDEX IF NOT EXISTS idx_mcp_services_visibility
    ON mcp_services(visibility, tenant_id)
    WHERE deleted_at IS NULL;

ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS visibility VARCHAR(16) NOT NULL DEFAULT 'workspace';

ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

ALTER TABLE tenant_skill_catalog
    DROP CONSTRAINT IF EXISTS tenant_skill_catalog_visibility_check;
ALTER TABLE tenant_skill_catalog
    ADD CONSTRAINT tenant_skill_catalog_visibility_check
    CHECK (visibility IN ('private', 'workspace', 'public'));

CREATE INDEX IF NOT EXISTS idx_tenant_skill_catalog_visibility
    ON tenant_skill_catalog(visibility, tenant_id)
    WHERE deleted_at IS NULL;
