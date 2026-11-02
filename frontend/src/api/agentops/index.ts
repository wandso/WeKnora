import { get, post, put, del } from "../../utils/request";

// Agent operations centre: version history, answer feedback, usage/quality,
// templates and public pages. Shapes mirror internal/types/agentops.go.

export type FeedbackRating = "up" | "down";

export type FeedbackReason =
  | "wrong_answer"
  | "bad_citation"
  | "incomplete"
  | "too_slow"
  | "unsafe"
  | "bad_format"
  | "tool_failure"
  | "other";

export interface MessageFeedback {
  id: string;
  tenant_id: number;
  message_id: string;
  user_id: string;
  session_id: string;
  agent_id: string;
  rating: FeedbackRating;
  reason_tags: FeedbackReason[] | null;
  comment: string;
  created_at: string;
  updated_at: string;
}

export interface SubmitFeedbackPayload {
  session_id: string;
  message_id: string;
  rating: FeedbackRating;
  reason_tags?: FeedbackReason[];
  comment?: string;
}

/** Submit or replace the caller's vote on one assistant message. */
export const submitFeedback = (payload: SubmitFeedbackPayload) =>
  post<{ success: boolean; data: MessageFeedback }>("/api/v1/answer-feedback", payload);

/** The caller's existing vote, or null when they have not voted. */
export const getMyFeedback = (messageId: string) =>
  get<{ success: boolean; data: MessageFeedback | null }>("/api/v1/answer-feedback", {
    params: { message_id: messageId },
  });

export interface AgentVersion {
  id: string;
  agent_id: string;
  version_no: number;
  name: string;
  description: string;
  avatar: string;
  config: Record<string, unknown>;
  changelog: string;
  created_by: string;
  source: "create" | "update" | "publish" | "rollback";
  created_at: string;
}

export const listAgentVersions = (agentId: string, limit = 20, offset = 0) =>
  get<{ success: boolean; data: AgentVersion[]; total: number }>(
    `/api/v1/agents/${agentId}/versions`,
    { params: { limit, offset } }
  );

export const publishAgentVersion = (agentId: string, changelog = "") =>
  post<{ success: boolean; data: AgentVersion }>(
    `/api/v1/agents/${agentId}/versions`,
    { changelog }
  );

export const rollbackAgentVersion = (agentId: string, versionNo: number) =>
  post<{ success: boolean; data: unknown }>(
    `/api/v1/agents/${agentId}/versions/${versionNo}/rollback`
  );

export interface AgentVersionDiff {
  from: number;
  to: number;
  changed_keys: string[];
  name_changed: boolean;
  description_changed: boolean;
  avatar_changed: boolean;
}

export const diffAgentVersions = (agentId: string, from: number, to: number) =>
  get<{ success: boolean; data: AgentVersionDiff }>(
    `/api/v1/agents/${agentId}/version-diff`,
    { params: { from, to } }
  );

export interface AgentUsageSummary {
  turns: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  avg_steps: number;
  avg_latency_ms: number;
  unique_sessions: number;
}

export interface AgentUsagePoint {
  bucket: string;
  turns: number;
  total_tokens: number;
}

export interface FeedbackSummary {
  up: number;
  down: number;
  total: number;
  up_rate: number;
  reasons: Record<string, number> | null;
}

export interface AgentQualityReport {
  agent_id: string;
  since: string;
  usage: AgentUsageSummary;
  feedback: FeedbackSummary;
}

export const getAgentUsage = (agentId: string, days = 30) =>
  get<{ success: boolean; data: AgentUsageSummary }>(`/api/v1/agents/${agentId}/usage`, {
    params: { days },
  });

export const getAgentUsageTimeseries = (agentId: string, days = 30) =>
  get<{ success: boolean; data: AgentUsagePoint[] }>(
    `/api/v1/agents/${agentId}/usage/timeseries`,
    { params: { days } }
  );

export const getAgentQuality = (agentId: string, days = 30) =>
  get<{ success: boolean; data: AgentQualityReport }>(`/api/v1/agents/${agentId}/quality`, {
    params: { days },
  });

export const listAgentFeedback = (
  agentId: string,
  rating: FeedbackRating | "",
  limit = 20,
  offset = 0
) =>
  get<{ success: boolean; data: MessageFeedback[]; total: number }>(
    `/api/v1/agents/${agentId}/feedback`,
    { params: { rating: rating || undefined, limit, offset } }
  );

export interface AgentTemplate {
  id: string;
  tenant_id: number;
  name: string;
  category: string;
  icon: string;
  description: string;
  agent_name: string;
  agent_description: string;
  agent_avatar: string;
  source: "builtin" | "tenant";
  installs: number;
  created_at: string;
}

export const listAgentTemplates = (category = "") =>
  get<{ success: boolean; data: AgentTemplate[] }>("/api/v1/agent-templates", {
    params: category ? { category } : undefined,
  });

export const installAgentTemplate = (templateId: string) =>
  post<{ success: boolean; data: unknown }>(
    `/api/v1/agent-templates/${templateId}/install`
  );

export const saveAgentAsTemplate = (
  agentId: string,
  payload: { name?: string; category?: string; icon?: string; description?: string }
) => post<{ success: boolean; data: AgentTemplate }>(`/api/v1/agents/${agentId}/templates`, payload);

export const deleteAgentTemplate = (templateId: string) =>
  del<{ success: boolean }>(`/api/v1/agent-templates/${templateId}`);

export interface AgentPublicPage {
  id: string;
  agent_id: string;
  embed_channel_id: string;
  slug: string;
  title: string;
  description: string;
  og_image_url: string;
  access_mode: "open" | "password";
  enabled: boolean;
}

export const getAgentPublicPage = (agentId: string) =>
  get<{ success: boolean; data: AgentPublicPage | null }>(
    `/api/v1/agents/${agentId}/public-page`
  );

export const upsertAgentPublicPage = (
  agentId: string,
  payload: {
    embed_channel_id: string;
    slug: string;
    title?: string;
    description?: string;
    og_image_url?: string;
    access_mode?: "open" | "password";
    password?: string;
    enabled?: boolean;
  }
) => put<{ success: boolean; data: AgentPublicPage }>(`/api/v1/agents/${agentId}/public-page`, payload);

export const deleteAgentPublicPage = (agentId: string) =>
  del<{ success: boolean }>(`/api/v1/agents/${agentId}/public-page`);

/** Anonymous resolver used by the public landing page. */
export const resolvePublicAgent = (slug: string, password = "") =>
  get<{ success: boolean; data: unknown }>(`/api/v1/public/agents/${slug}`, {
    params: password ? { password } : undefined,
  });

export interface AgentDryRunResult {
  answer: string;
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number };
  steps?: Array<Record<string, unknown>>;
  duration_ms: number;
}

/** Run an agent against an unsaved draft config. Writes nothing to history. */
export const dryRunAgent = (agentId: string, config: Record<string, unknown>, query: string) =>
  post<{ success: boolean; data: AgentDryRunResult }>(`/api/v1/agents/${agentId}/dry-run`, {
    config,
    query,
  });

// ---------------------------------------------------------------------------
// Content moderation (observation phase)
// ---------------------------------------------------------------------------
// The backend records matches and never alters a conversation. These wrappers
// exist so the operations page can show what would have been caught before any
// blocking policy is switched on.

export type ModerationDirection = "input" | "output";
export type ModerationAction = "record" | "block";

export interface ModerationRule {
  id: string;
  tenant_id: number;
  name: string;
  category: string;
  pattern: string;
  is_regex: boolean;
  action: ModerationAction;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ModerationHit {
  id: string;
  tenant_id: number;
  agent_id: string;
  session_id: string;
  message_id: string;
  direction: ModerationDirection;
  rule_id: string;
  rule_name: string;
  category: string;
  snippet: string;
  action: ModerationAction;
  created_at: string;
}

export interface ModerationScanResult {
  scanned: number;
  matched: number;
  rules: number;
}

export const listModerationRules = () =>
  get<{ success: boolean; data: ModerationRule[] }>("/api/v1/moderation-rules");

export const upsertModerationRule = (payload: {
  id?: string;
  name: string;
  category?: string;
  pattern: string;
  is_regex?: boolean;
  action?: ModerationAction;
  enabled?: boolean;
}) => post<{ success: boolean; data: ModerationRule }>("/api/v1/moderation-rules", payload);

export const deleteModerationRule = (id: string) =>
  del<{ success: boolean }>(`/api/v1/moderation-rules/${id}`);

export const listModerationHits = (agentId: string, limit = 20, offset = 0) =>
  get<{ success: boolean; data: ModerationHit[]; total: number }>(
    `/api/v1/agents/${agentId}/moderation-hits`,
    { params: { limit, offset } }
  );

/** Scan the agent's recent turns. Idempotent: re-running records nothing new. */
export const scanModeration = (agentId: string, limit = 200) =>
  post<{ success: boolean; data: ModerationScanResult }>(
    `/api/v1/agents/${agentId}/moderation-scan`,
    { limit }
  );
