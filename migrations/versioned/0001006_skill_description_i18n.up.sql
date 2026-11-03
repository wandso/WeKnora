-- Console translations of a skill definition's description.
--
-- tenant_skill_catalog.description holds the canonical text parsed out of
-- SKILL.md, which is what an installed image and the agent prompt use. This
-- column adds per-language overrides for the console only, as a JSON object
-- keyed by language code ("zh", "en").
--
-- Why a separate column instead of translating SKILL.md: editing a definition
-- must not touch its stored zip. An install row pins the archive its image was
-- built from, and rewriting the zip in place would make the file browser and
-- read_skill disagree with the image that is actually running.
--
-- Empty string means "no translations yet"; the card then falls back to
-- description. Not NULL so the Go layer never has to distinguish absent from
-- empty.

ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS description_i18n TEXT NOT NULL DEFAULT '';
