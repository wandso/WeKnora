import { del, get, post, put } from '@/utils/request'

// A 小工具 card is a prompt preset: a formula/method, its declared inputs and
// the domain knowledge the model should apply when the card is attached to a
// turn. Mirrors internal/types/minitool.go.

/**
 * Who may read a card: private (creator + workspace admins), workspace (all
 * members) or public (every account on the deployment, read-only for
 * everybody outside the owning workspace).
 */
export type MiniToolVisibility = 'private' | 'workspace' | 'public'

export interface MiniToolVariable {
  key: string
  label: string
  unit?: string
  default?: string
  remark?: string
  required?: boolean
  min?: number | null
  max?: number | null
}

export interface MiniToolExample {
  title: string
  input: string
  output: string
}

export interface MiniTool {
  id: string
  tenant_id: number
  owner_user_id: string
  owner_name?: string
  name: string
  icon: string
  category: string
  description: string
  formula: string
  formula_note: string
  variables: MiniToolVariable[]
  knowledge: string
  examples: MiniToolExample[]
  visibility: MiniToolVisibility
  enabled: boolean
  usage_count: number
  can_edit: boolean
  can_delete: boolean
  // Set when the card belongs to another workspace and is visible only because
  // it is public. Read-only for this caller: edit/delete are always false.
  is_foreign?: boolean
  created_at: string
  updated_at: string
}

export interface MiniToolPayload {
  name: string
  icon?: string
  category?: string
  description?: string
  formula?: string
  formula_note?: string
  variables?: MiniToolVariable[]
  knowledge?: string
  examples?: MiniToolExample[]
  visibility?: MiniToolVisibility
  enabled?: boolean
}

export interface MiniToolListQuery {
  category?: string
  keyword?: string
  scope?: 'all' | 'mine' | 'workspace'
  include_disabled?: boolean
}

export function listMiniTools(query: MiniToolListQuery = {}) {
  return get<{ data: MiniTool[]; success: boolean }>('/api/v1/mini-tools', {
    params: {
      ...(query.category ? { category: query.category } : {}),
      ...(query.keyword ? { keyword: query.keyword } : {}),
      ...(query.scope ? { scope: query.scope } : {}),
      ...(query.include_disabled ? { include_disabled: 'true' } : {}),
    },
  })
}

export function listMiniToolCategories() {
  return get<{ data: string[]; success: boolean }>('/api/v1/mini-tools/categories')
}

export function getMiniTool(id: string) {
  return get<{ data: MiniTool; success: boolean }>(`/api/v1/mini-tools/${id}`)
}

export function createMiniTool(payload: MiniToolPayload) {
  return post<{ data: MiniTool; success: boolean }>('/api/v1/mini-tools', payload)
}

export function updateMiniTool(id: string, payload: Partial<MiniToolPayload>) {
  return put<{ data: MiniTool; success: boolean }>(`/api/v1/mini-tools/${id}`, payload)
}

export function deleteMiniTool(id: string) {
  return del<{ success: boolean }>(`/api/v1/mini-tools/${id}`)
}

export function duplicateMiniTool(id: string) {
  return post<{ data: MiniTool; success: boolean }>(`/api/v1/mini-tools/${id}/duplicate`)
}
