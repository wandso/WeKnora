import { del, get, post, postUpload, put } from "../../utils/request";
import type { ConfigSkillFileContent, ConfigSkillFileEntry } from "../system";

// Skill信息
export interface SkillInfo {
  name: string;
  description: string;
  // Governance metadata parsed from SKILL.md. category: safe | read_local |
  // read_remote | write | unclassified; risk_level: low | medium | high |
  // critical. Always present, so the UI need not guess a default.
  category?: string;
  risk_level?: string;
  keywords?: string[];
  prompt_hint?: string;
}

export interface SkillCatalogInstall {
  skill_id: string;
  sandbox_config_id: string;
  sandbox_config_name?: string;
  sandbox_type?: string;
  status: string;
  enabled: boolean;
  error?: string;
  version?: string;
  bundle_sha256?: string;
  // Present while a newer install is in flight or has failed and the sandbox
  // still runs the previous version.
  served?: { version?: string };
  updated_at: string;
}

export interface SkillCatalogItem {
  id: string;
  name: string;
  version?: string;
  description?: string;
  // Console translations of description keyed by language code. A missing
  // entry means "not translated yet"; the card falls back to description.
  description_i18n?: Record<string, string>;
  bundle_sha256?: string;
  created_at: string;
  updated_at: string;
  installations: SkillCatalogInstall[];
  // Governance metadata parsed from SKILL.md. The backend normalizes both
  // values, so they are always one of the known vocabulary words.
  category?: string;
  risk_level?: string;
  keywords?: string[];
  prompt_hint?: string;
  // Who may discover and install this definition. Public definitions owned by
  // another workspace arrive with is_foreign=true and every write flag false;
  // they can be installed into this workspace's own sandbox but not edited.
  visibility?: 'private' | 'workspace' | 'public';
  is_foreign?: boolean;
  can_edit?: boolean;
  can_delete?: boolean;
  can_share?: boolean;
}

export interface SkillCatalogRegisterResult {
  id: string;
  name: string;
  version?: string;
  description?: string;
}

/** Locates a shared agent's source workspace, same parameters as a chat request. */
export interface AgentScope {
  agentId?: string;
  /** Set only for a shared agent; left empty for this workspace's own agents. */
  sourceTenantId?: string | number;
}

// Lists the skills executable on the current sandbox config. Without a
// sandboxConfigId, or when skills_available is false, the caller should hide or
// disable the skills UI.
//
// Pass the agent for a shared one: its skills are installed on a sandbox config
// in the OWNER's workspace, so looking them up in the caller's returns nothing.
// The backend then reads the config off that agent and ignores sandboxConfigId.
export function listSkills(sandboxConfigId?: string, agent?: AgentScope) {
  const params: Record<string, string> = {};
  if (sandboxConfigId) params.sandbox_config_id = sandboxConfigId;
  if (agent?.agentId && agent?.sourceTenantId) {
    params.agent_id = agent.agentId;
    params.agent_source_tenant_id = String(agent.sourceTenantId);
  }
  return get<{ data: SkillInfo[]; skills_available?: boolean }>('/api/v1/skills', { params });
}

export function listSkillCatalog() {
  return get<{ data: SkillCatalogItem[] }>('/api/v1/skills/catalog');
}

export function registerSkillCatalogFromSource(source: string) {
  return post<{ data: SkillCatalogRegisterResult }>('/api/v1/skills/catalog', { source }, {
    timeout: 2 * 60 * 1000,
  });
}

export function registerSkillCatalogFromFile(
  file: File,
  onProgress?: (percent: number) => void,
) {
  const form = new FormData();
  form.append('file', file);
  // A multi-skill zip registers every SKILL.md directory, so the response is a
  // list of definitions; a single-skill zip answers with one object.
  return postUpload('/api/v1/skills/catalog', form, (e: any) => {
    if (e.total) onProgress?.(Math.round((e.loaded * 100) / e.total));
  }, { timeout: 5 * 60 * 1000 }) as Promise<{
    data: SkillCatalogRegisterResult | SkillCatalogRegisterResult[];
  }>;
}

export function installSkillCatalog(catalogId: string, sandboxConfigIds: string[]) {
  return post<{ data: { installs: Record<string, string>; errors?: Record<string, string> } }>(
    `/api/v1/skills/catalog/${catalogId}/install`,
    { sandbox_config_ids: sandboxConfigIds },
  );
}

export function deleteSkillCatalog(catalogId: string) {
  return del(`/api/v1/skills/catalog/${catalogId}`);
}

/**
 * Change only the visibility of a definition. Separate from registration
 * because it is a workspace policy decision, not a content change, and it must
 * keep working for definitions whose bundle this caller cannot re-upload.
 */
export function setSkillCatalogVisibility(
  catalogId: string,
  visibility: 'private' | 'workspace' | 'public',
) {
  return put<{ data: { id: string; visibility: string } }>(
    `/api/v1/skills/catalog/${catalogId}/visibility`,
    { visibility },
  );
}

export function listCatalogSkillFiles(catalogId: string) {
  return get<{ data: ConfigSkillFileEntry[] }>(`/api/v1/skills/catalog/${catalogId}/files`);
}

/** Console description languages. "source" addresses the SKILL.md text. */
export type SkillDescriptionLang = 'zh' | 'en';
export type SkillDescriptionTarget = SkillDescriptionLang | 'source';

export interface SkillCatalogDescription {
  id: string;
  description: string;
  description_i18n: Record<string, string>;
}

/**
 * Rewrites the description of one language. "source" writes the text parsed
 * out of SKILL.md; a language code writes a console-only translation. Neither
 * touches the stored bundle, so installed sandboxes keep their SKILL.md.
 */
export function updateSkillCatalogDescription(
  catalogId: string,
  target: SkillDescriptionTarget,
  description: string,
) {
  return put<{ success: boolean; data: SkillCatalogDescription }>(
    `/api/v1/skills/catalog/${catalogId}/description`,
    { lang: target, description },
  );
}

/**
 * Machine-translates a description with the workspace's chat model. text
 * defaults on the server to the definition's own description; save=false only
 * returns the translation so the editor can preview it.
 */
export function translateSkillCatalogDescription(
  catalogId: string,
  lang: SkillDescriptionLang,
  options: { text?: string; save?: boolean } = {},
) {
  return post<{
    success: boolean;
    data: {
      lang: SkillDescriptionLang;
      description: string;
      saved: boolean;
      catalog: SkillCatalogDescription;
    };
  }>(`/api/v1/skills/catalog/${catalogId}/translate`, {
    lang,
    text: options.text ?? '',
    save: options.save ?? false,
  }, { timeout: 2 * 60 * 1000 });
}

export function getCatalogSkillFile(catalogId: string, path: string) {
  return get<{ data: ConfigSkillFileContent }>(`/api/v1/skills/catalog/${catalogId}/files/content`, {
    params: { path },
  });
}
