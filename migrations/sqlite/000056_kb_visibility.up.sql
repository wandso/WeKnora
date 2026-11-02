-- Knowledge base visibility (SQLite flavour of 000139).
--
-- private / workspace / public, defaulting to workspace so existing rows keep
-- the audience they already had. "public" is read-only for everyone outside the
-- owning workspace; see the PostgreSQL migration for the reasoning.

ALTER TABLE knowledge_bases
    ADD COLUMN visibility VARCHAR(16) NOT NULL DEFAULT 'workspace';

-- SQLite has no ADD CONSTRAINT, so the vocabulary is enforced with a trigger.
-- A typo would otherwise fall back to the default in application code and widen
-- an audience the author did not intend, which is exactly the failure mode that
-- must not be silent.
CREATE TRIGGER IF NOT EXISTS trg_knowledge_bases_visibility_insert
BEFORE INSERT ON knowledge_bases
WHEN NEW.visibility NOT IN ('private', 'workspace', 'public')
BEGIN
    SELECT RAISE(ABORT, 'knowledge_bases.visibility must be private, workspace or public');
END;

CREATE TRIGGER IF NOT EXISTS trg_knowledge_bases_visibility_update
BEFORE UPDATE OF visibility ON knowledge_bases
WHEN NEW.visibility NOT IN ('private', 'workspace', 'public')
BEGIN
    SELECT RAISE(ABORT, 'knowledge_bases.visibility must be private, workspace or public');
END;

-- Listing public KBs is a cross-tenant scan filtered by visibility.
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility
    ON knowledge_bases(visibility, tenant_id);
