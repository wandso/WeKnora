DROP INDEX IF EXISTS idx_knowledge_bases_visibility;
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_visibility_check;
ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS visibility;
