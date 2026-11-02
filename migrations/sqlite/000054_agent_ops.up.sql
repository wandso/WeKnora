CREATE TABLE IF NOT EXISTS agent_versions (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    agent_id VARCHAR(36) NOT NULL,
    version_no INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    avatar VARCHAR(64) NOT NULL DEFAULT '',
    config TEXT NOT NULL DEFAULT '{}',
    changelog VARCHAR(512) NOT NULL DEFAULT '',
    created_by VARCHAR(36) NOT NULL DEFAULT '',
    source VARCHAR(16) NOT NULL DEFAULT 'update',
    created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_versions_no
    ON agent_versions(tenant_id, agent_id, version_no);
CREATE INDEX IF NOT EXISTS idx_agent_versions_agent
    ON agent_versions(tenant_id, agent_id, created_at);

CREATE TABLE IF NOT EXISTS message_feedback (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    message_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(128) NOT NULL,
    session_id VARCHAR(36) NOT NULL DEFAULT '',
    agent_id VARCHAR(36) NOT NULL DEFAULT '',
    rating VARCHAR(8) NOT NULL,
    reason_tags TEXT NOT NULL DEFAULT '[]',
    comment TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_message_feedback_vote
    ON message_feedback(tenant_id, message_id, user_id);
CREATE INDEX IF NOT EXISTS idx_message_feedback_agent
    ON message_feedback(tenant_id, agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_message_feedback_session
    ON message_feedback(session_id);


CREATE TABLE IF NOT EXISTS agent_templates (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL DEFAULT 0,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL DEFAULT 'general',
    icon VARCHAR(64) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    config TEXT NOT NULL DEFAULT '{}',
    agent_name VARCHAR(255) NOT NULL DEFAULT '',
    agent_description TEXT NOT NULL DEFAULT '',
    agent_avatar VARCHAR(64) NOT NULL DEFAULT '',
    source VARCHAR(16) NOT NULL DEFAULT 'tenant',
    created_by VARCHAR(36) NOT NULL DEFAULT '',
    installs INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_templates_tenant
    ON agent_templates(tenant_id, category);

CREATE TABLE IF NOT EXISTS agent_public_pages (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    agent_id VARCHAR(36) NOT NULL,
    embed_channel_id VARCHAR(36) NOT NULL,
    slug VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    og_image_url VARCHAR(512) NOT NULL DEFAULT '',
    access_mode VARCHAR(16) NOT NULL DEFAULT 'open',
    password_hash VARCHAR(128) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_public_pages_slug ON agent_public_pages(slug);
CREATE INDEX IF NOT EXISTS idx_agent_public_pages_tenant
    ON agent_public_pages(tenant_id, agent_id);
