-- Knowledge base visibility: private / workspace / public.
--
-- Until now a knowledge base was only reachable from inside its own workspace
-- (plus explicit organization shares). Cross-workspace reuse therefore needed
-- an organization, an invite and a per-KB share row — too much ceremony for the
-- common case of publishing a reference corpus to everyone on the deployment.
--
-- Three levels, ordered by audience:
--   private   the creator, plus workspace Owner/Admin
--   workspace every member of the owning workspace (the historical behaviour,
--             hence the default so existing rows keep their current audience)
--   public    every signed-in account, read-only
--
-- "public" is deliberately read-only: a published corpus is a single copy, and
-- letting any account edit it would turn a shared reference into a wiki nobody
-- owns. Non-owners get Viewer at the access resolver, so every write route
-- refuses them without needing its own check.
--
-- Organization shares remain a separate axis: a KB can be public and also be
-- shared into an org with editor rights, and the resolver takes the stronger of
-- the two. The column is not indexed for listing because "public" is a small
-- minority of rows and the list query unions it with the caller's own rows.

ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS visibility VARCHAR(16) NOT NULL DEFAULT 'workspace';

-- Constrain the vocabulary at the storage layer as well: a typo would otherwise
-- silently fall back to the default in application code and widen an audience
-- the author did not intend.
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_visibility_check;
ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_visibility_check
    CHECK (visibility IN ('private', 'workspace', 'public'));

-- Listing public KBs is a cross-tenant scan filtered by visibility, so the
-- index leads with the column that has the smallest match set.
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility
    ON knowledge_bases(visibility, tenant_id)
    WHERE deleted_at IS NULL;
