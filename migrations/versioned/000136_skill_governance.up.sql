-- Skill governance metadata parsed from SKILL.md at install time. Values are
-- classification hints for the toolbox: category describes the side effect a
-- skill can have, risk_level its severity. Existing rows keep empty strings,
-- which the API normalizes to unclassified / medium.
ALTER TABLE tenant_skills ADD COLUMN IF NOT EXISTS category VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE tenant_skills ADD COLUMN IF NOT EXISTS risk_level VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE tenant_skills ADD COLUMN IF NOT EXISTS keywords VARCHAR(512) NOT NULL DEFAULT '';
ALTER TABLE tenant_skills ADD COLUMN IF NOT EXISTS prompt_hint TEXT NOT NULL DEFAULT '';
