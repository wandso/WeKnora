/**
 * 关键词模糊过滤。
 *
 * 给智能体编辑弹窗里 MCP 服务 / 技能 / 小工具三个候选列表用 —— 这几份列表动辄几十上百项，
 * 只能靠滚动找目标项太慢。规则（不引第三方依赖，中英文一视同仁）：
 *
 *   - 查询按空白拆词，每个词都要命中任一字段，全部命中才算这条匹配；
 *   - 先试连续子串（大小写不敏感），命中不了再退化成「按顺序出现」的子序列，
 *     所以 `kb` 能命中 `search-knowledge-base`，`牛头` 能命中 `牛头山水库`；
 *   - 每个字段有权重，名称命中（1）天然排在描述命中（0.55）前面；
 *   - 分数只用于排序，同分保持原有顺序，避免过滤时列表跳动。
 *
 * 子序列这一层算「退化匹配」，噪音也主要来自它（`kb` 会把 `kube-...` 之类一起带出来），
 * 所以留了 `degrade: false` 的开关：关掉后只认连续子串，所见即所得。
 */

export interface FuzzyField {
  text: string
  /** 越大越重要，默认 1。 */
  weight?: number
}

export interface FuzzyOptions {
  /**
   * 是否允许退化匹配，默认 `true`。
   *
   * - `true`：连续子串命中不了时，再按「字符按顺序出现」找一遍（也忽略 `-_/ ` 等分隔符，
   *   所以 `mcpjson` 能命中 `mcp-json-tool`）；
   * - `false`：只匹配原文中连续出现的子串（大小写不敏感），不做任何跳字符。
   */
  degrade?: boolean
}

/** 词边界：命中落在这些字符之后，说明是某个词的词首，排名更靠前。 */
const WORD_BOUNDARY = /[\s\-_/.,:·、，。：；()（）[\]【】]/

const SEPARATORS = /[\s\-_/.,:·、，。：；()（）[\]【】]/g

export function tokenizeQuery(query: string): string[] {
  return query.trim().toLowerCase().split(/\s+/).filter(Boolean)
}

export function hasSearchQuery(query: string): boolean {
  return tokenizeQuery(query).length > 0
}

/** 去掉分隔符的副本，让 `mcpjson` 也能命中 `MCP JSON` / `mcp_json`。 */
function compact(text: string): string {
  return text.replace(SEPARATORS, '')
}

function scoreField(token: string, rawText: string, degrade: boolean): number | null {
  if (!rawText) return null
  const text = rawText.toLowerCase()
  const direct = text.indexOf(token)
  if (direct >= 0) {
    const atWordStart = direct === 0 || WORD_BOUNDARY.test(text[direct - 1] ?? '')
    return 1000 + (atWordStart ? 120 : 0) - Math.min(direct, 100)
  }
  if (!degrade) return null
  // 单字做子序列匹配等于没匹配（它本身就是子串），直接放弃。
  if (token.length < 2) return null
  const hay = compact(text)
  let from = 0
  let start = -1
  let gaps = 0
  for (const ch of token) {
    const at = hay.indexOf(ch, from)
    if (at < 0) return null
    if (start < 0) start = at
    else gaps += at - from
    from = at + 1
  }
  // 子序列命中永远排在子串命中之后，跳过越多分越低。
  return Math.max(60, 520 - gaps * 8 - start * 3)
}

/** 命中返回得分（越高越靠前），未命中返回 null。空查询返回 0。 */
export function fuzzyScore(
  query: string,
  fields: readonly FuzzyField[],
  options: FuzzyOptions = {},
): number | null {
  const degrade = options.degrade ?? true
  const tokens = tokenizeQuery(query)
  if (!tokens.length) return 0
  let total = 0
  for (const token of tokens) {
    let best: number | null = null
    for (const field of fields) {
      const score = scoreField(token, field.text, degrade)
      if (score === null) continue
      const weighted = score * (field.weight ?? 1)
      if (best === null || weighted > best) best = weighted
    }
    if (best === null) return null
    total += best
  }
  return total
}

/**
 * 过滤并按相关度排序。空查询原样返回（保持调用方给的顺序），
 * 这样搜索框清空后列表回到默认分组顺序。
 */
export function fuzzyFilter<T>(
  items: readonly T[],
  query: string,
  fieldsOf: (item: T) => readonly FuzzyField[],
  options: FuzzyOptions = {},
): T[] {
  if (!hasSearchQuery(query)) return [...items]
  const scored: Array<{ item: T; score: number; order: number }> = []
  items.forEach((item, order) => {
    const score = fuzzyScore(query, fieldsOf(item), options)
    if (score !== null) scored.push({ item, score, order })
  })
  scored.sort((a, b) => b.score - a.score || a.order - b.order)
  return scored.map((entry) => entry.item)
}
