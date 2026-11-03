-- Content moderation: rules and hit log.
--
-- Deliberately record-only for now. The plan agreed in
-- docs/agent-ops-analysis is to accumulate a week of real hits before any
-- blocking is switched on, because a false positive that truncates a paying
-- user's answer is worse than a missed detection during the observation phase.
-- `moderation_rules.action` therefore defaults to 'record'; the column exists
-- so enabling blocking later is a data change, not a migration.
--
-- Rules are tenant-owned keyword/regex entries. The product does not ship a
-- built-in profanity or political word list: those lists age badly, differ by
-- jurisdiction, and a wrong entry silently censors legitimate business content.
-- Only structural detectors (phone numbers, ID cards, bank cards) are built in,
-- and they live in code because they are patterns, not vocabulary.

CREATE TABLE IF NOT EXISTS moderation_rules (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL DEFAULT 'custom',
    pattern TEXT NOT NULL,
    is_regex BOOLEAN NOT NULL DEFAULT FALSE,
    action VARCHAR(16) NOT NULL DEFAULT 'record',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_moderation_rules_tenant
    ON moderation_rules(tenant_id, enabled);

CREATE TABLE IF NOT EXISTS moderation_hits (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    agent_id VARCHAR(36) NOT NULL DEFAULT '',
    session_id VARCHAR(36) NOT NULL DEFAULT '',
    message_id VARCHAR(36) NOT NULL DEFAULT '',
    direction VARCHAR(16) NOT NULL DEFAULT 'output',
    rule_id VARCHAR(36) NOT NULL DEFAULT '',
    rule_name VARCHAR(255) NOT NULL DEFAULT '',
    category VARCHAR(64) NOT NULL DEFAULT 'custom',
    snippet TEXT NOT NULL DEFAULT '',
    action VARCHAR(16) NOT NULL DEFAULT 'record',
    created_at TIMESTAMPTZ NOT NULL
);
-- Re-scanning a conversation must not duplicate rows: the same message cannot
-- hit the same rule twice.
CREATE UNIQUE INDEX IF NOT EXISTS idx_moderation_hits_unique
    ON moderation_hits(tenant_id, message_id, rule_id);
CREATE INDEX IF NOT EXISTS idx_moderation_hits_agent
    ON moderation_hits(tenant_id, agent_id, created_at DESC);
