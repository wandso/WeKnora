CREATE TABLE IF NOT EXISTS mini_tools (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
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
    usage_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_mini_tools_tenant ON mini_tools(tenant_id);
CREATE INDEX IF NOT EXISTS idx_mini_tools_owner ON mini_tools(tenant_id, owner_user_id);
CREATE INDEX IF NOT EXISTS idx_mini_tools_category ON mini_tools(tenant_id, category);
CREATE INDEX IF NOT EXISTS idx_mini_tools_deleted_at ON mini_tools(deleted_at);
