-- Console translations of a skill definition's description (SQLite flavour of
-- 000141).
--
-- The canonical copy stays in tenant_skill_catalog.description, parsed out of
-- SKILL.md. This column only carries the per-language overrides the console
-- shows on a card, as a JSON object keyed by language code ("zh", "en").
-- Nothing reads it at run time, so a bad value can never change what an
-- installed skill does.
--
-- An empty string means "no translations yet"; the console then falls back to
-- description. The default is '' rather than NULL so every driver hands the
-- Go layer a string and no pointer handling is needed.

ALTER TABLE tenant_skill_catalog
    ADD COLUMN description_i18n TEXT NOT NULL DEFAULT '';
