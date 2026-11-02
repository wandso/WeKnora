-- Content moderation: rules and hit log (SQLite flavour of 000137).
--
-- Record-only by design; see the PostgreSQL migration for the reasoning.

CREATE TABLE IF NOT EXISTS moderation_rules (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL DEFAULT 0,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL DEFAULT 'custom',
    pattern TEXT NOT NULL,
    is_regex INTEGER NOT NULL DEFAULT 0,
    action VARCHAR(16) NOT NULL DEFAULT 'record',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_moderation_rules_tenant
    ON moderation_rules(tenant_id, enabled);

CREATE TABLE IF NOT EXISTS moderation_hits (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    agent_id VARCHAR(36) NOT NULL DEFAULT '',
    session_id VARCHAR(36) NOT NULL DEFAULT '',
    message_id VARCHAR(36) NOT NULL DEFAULT '',
    direction VARCHAR(16) NOT NULL DEFAULT 'output',
    rule_id VARCHAR(36) NOT NULL DEFAULT '',
    rule_name VARCHAR(255) NOT NULL DEFAULT '',
    category VARCHAR(64) NOT NULL DEFAULT 'custom',
    snippet TEXT NOT NULL DEFAULT '',
    action VARCHAR(16) NOT NULL DEFAULT 'record',
    created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_moderation_hits_unique
    ON moderation_hits(tenant_id, message_id, rule_id);
CREATE INDEX IF NOT EXISTS idx_moderation_hits_agent
    ON moderation_hits(tenant_id, agent_id, created_at);
