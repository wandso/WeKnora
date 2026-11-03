import assert from 'node:assert/strict'
import test from 'node:test'

import { fuzzyFilter, fuzzyScore, hasSearchQuery, tokenizeQuery } from './fuzzyFilter.ts'

interface Row {
  name: string
  description?: string
  category?: string
}

const rows: Row[] = [
  { name: 'search_knowledge', description: '检索知识库', category: '知识库' },
  { name: 'wiki_delete_page', description: '删除 Wiki 页面', category: 'Wiki' },
  { name: 'mcp-json-tool', description: '把 JSON 转成表格', category: 'MCP' },
  { name: '牛头山水库库容', description: '水位库容曲线查算', category: '水利' },
  { name: 'kb_search', description: '按关键词搜索知识条目', category: '知识库' },
]

const fields = (row: Row) => [
  { text: row.name, weight: 1 },
  { text: row.description ?? '', weight: 0.55 },
  { text: row.category ?? '', weight: 0.4 },
]

const names = (query: string) => fuzzyFilter(rows, query, fields).map((row) => row.name)

/** 关掉退化匹配：只认连续子串。 */
const exactNames = (query: string) =>
  fuzzyFilter(rows, query, fields, { degrade: false }).map((row) => row.name)

test('空查询原样返回所有条目并保持顺序', () => {
  assert.deepEqual(names(''), rows.map((row) => row.name))
  assert.deepEqual(names('   '), rows.map((row) => row.name))
  assert.equal(hasSearchQuery('  '), false)
  assert.equal(hasSearchQuery('a'), true)
})

test('大小写不敏感，且名称命中排在描述命中前面', () => {
  // 只有 kb_search 的名称里有 kb；search_knowledge 的名字里没有，靠子序列/描述命中。
  assert.equal(names('KB')[0], 'kb_search')
  assert.ok(names('kb').includes('search_knowledge') || names('kb').length >= 1)
})

test('子序列命中：不连续也能找出来', () => {
  assert.deepEqual(names('srhkn'), ['search_knowledge'])
  assert.deepEqual(names('wdp'), ['wiki_delete_page'])
})

test('忽略分隔符与大小写：mcpjson 命中 mcp-json-tool', () => {
  assert.deepEqual(names('mcpjson'), ['mcp-json-tool'])
  assert.deepEqual(names('MCP JSON'), ['mcp-json-tool'])
})

test('中文按字匹配', () => {
  assert.deepEqual(names('牛头'), ['牛头山水库库容'])
  assert.deepEqual(names('库容'), ['牛头山水库库容'])
  assert.deepEqual(names('删页'), ['wiki_delete_page'])
})

test('多个词之间是「且」的关系', () => {
  assert.deepEqual(names('wiki 页面'), ['wiki_delete_page'])
  assert.deepEqual(names('wiki 库容'), [])
})

test('单字不参与子序列匹配，避免整表命中', () => {
  assert.deepEqual(names('z'), [])
})

test('无命中返回空数组', () => {
  assert.deepEqual(names('不存在的工具'), [])
})

test('得分只用于排序：名称命中的分数高于描述命中', () => {
  const byName = fuzzyScore('wiki', fields(rows[1]))!
  const byDescription = fuzzyScore('wiki', fields({ name: 'x', description: 'wiki 相关' }))!
  assert.ok(byName > byDescription)
})

test('同分时保持原有顺序', () => {
  const same = [
    { name: '工具 A' },
    { name: '工具 B' },
    { name: '工具 C' },
  ]
  assert.deepEqual(
    fuzzyFilter(same, '工具', (row) => [{ text: row.name }]).map((row) => row.name),
    ['工具 A', '工具 B', '工具 C'],
  )
})

test('分词：连续空格与首尾空格不影响结果', () => {
  assert.deepEqual(tokenizeQuery('  wiki   page  '), ['wiki', 'page'])
})

test('关掉退化匹配后不再按顺序跳字符命中', () => {
  assert.deepEqual(names('srhkn'), ['search_knowledge'])
  assert.deepEqual(exactNames('srhkn'), [])
  assert.deepEqual(names('wdp'), ['wiki_delete_page'])
  assert.deepEqual(exactNames('wdp'), [])
})

test('关掉退化匹配后不再忽略分隔符', () => {
  assert.deepEqual(names('mcpjson'), ['mcp-json-tool'])
  assert.deepEqual(exactNames('mcpjson'), [])
  // 原样连续的子串照样命中，只看大小写不敏感。
  assert.deepEqual(exactNames('MCP-JSON'), ['mcp-json-tool'])
})

test('关掉退化匹配仍保持大小写不敏感', () => {
  assert.deepEqual(exactNames('KB_SEARCH'), ['kb_search'])
  assert.deepEqual(exactNames('Knowledge'), ['search_knowledge'])
})

test('关掉退化匹配时中文必须是连续片段', () => {
  assert.deepEqual(exactNames('牛头'), ['牛头山水库库容'])
  assert.deepEqual(names('删页'), ['wiki_delete_page'])
  assert.deepEqual(exactNames('删页'), [])
})

test('关掉退化匹配后多词仍是「且」关系，且可跨字段', () => {
  // wiki 命中名称、页面 命中描述，两个词各自连续即可。
  assert.deepEqual(exactNames('wiki 页面'), ['wiki_delete_page'])
  assert.deepEqual(exactNames('wiki 库容'), [])
})

test('开关不影响空查询与排序规则', () => {
  assert.deepEqual(exactNames(''), rows.map((row) => row.name))
  const same = [{ name: '工具 A' }, { name: '工具 B' }]
  assert.deepEqual(
    fuzzyFilter(same, '工具', (row) => [{ text: row.name }], { degrade: false }).map((row) => row.name),
    ['工具 A', '工具 B'],
  )
  const byName = fuzzyScore('wiki', fields(rows[1]), { degrade: false })!
  const byDescription = fuzzyScore(
    'wiki',
    fields({ name: 'x', description: 'wiki 相关' }),
    { degrade: false },
  )!
  assert.ok(byName > byDescription)
})
