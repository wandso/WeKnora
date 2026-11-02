-- 小工具（mini tools）: workspace-scoped prompt presets carrying a formula,
-- declared inputs and domain knowledge. An agent references cards by id, so a
-- card can be edited without rewriting the agents that use it.
CREATE TABLE IF NOT EXISTS mini_tools (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    owner_user_id VARCHAR(36) NOT NULL DEFAULT '',
    name VARCHAR(120) NOT NULL,
    icon VARCHAR(64) NOT NULL DEFAULT '',
    category VARCHAR(64) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    formula TEXT NOT NULL DEFAULT '',
    formula_note TEXT NOT NULL DEFAULT '',
    variables TEXT NOT NULL DEFAULT '[]',
    knowledge TEXT NOT NULL DEFAULT '',
    examples TEXT NOT NULL DEFAULT '[]',
    visibility VARCHAR(16) NOT NULL DEFAULT 'private',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    usage_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mini_tools_tenant ON mini_tools(tenant_id);
CREATE INDEX IF NOT EXISTS idx_mini_tools_owner ON mini_tools(tenant_id, owner_user_id);
CREATE INDEX IF NOT EXISTS idx_mini_tools_category ON mini_tools(tenant_id, category);
CREATE INDEX IF NOT EXISTS idx_mini_tools_deleted_at ON mini_tools(deleted_at);
