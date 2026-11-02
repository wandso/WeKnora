DROP INDEX IF EXISTS idx_knowledge_bases_visibility;
DROP TRIGGER IF EXISTS trg_knowledge_bases_visibility_update;
DROP TRIGGER IF EXISTS trg_knowledge_bases_visibility_insert;
ALTER TABLE knowledge_bases DROP COLUMN visibility;
