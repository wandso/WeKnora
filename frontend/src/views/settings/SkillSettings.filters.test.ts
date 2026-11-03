import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { runInNewContext } from 'node:vm'
import test from 'node:test'
import { compileScript, parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import { createRenderer, nextTick, ref } from 'vue'
import { matchesResourceQuery } from '../../utils/resourceListSearch'
import * as skillTarget from '../../utils/skillTarget'
import * as skillUpgrade from '../../utils/skillUpgrade'
import type { SkillCatalogItem } from '../../api/skill'

const require = createRequire(import.meta.url)
const filename = fileURLToPath(new URL('./SkillSettings.vue', import.meta.url))
const { descriptor } = parse(readFileSync(filename, 'utf8'), { filename })
const script = compileScript(descriptor, { id: 'skill-filters-test' }).content
  .replace(/__expose\([^;]*\);/g, '')
  .replace('return __returned__', '__expose(__returned__); return __returned__')
const compiled = ts.transpileModule(script, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText

function item(id: string, statuses: string[] = []): SkillCatalogItem {
  return {
    id, name: id, description: '', created_at: '', updated_at: '',
    installations: statuses.map((status, index) => ({
      skill_id: `${id}-${index}`, sandbox_config_id: `sandbox-${index}`,
      status, enabled: false, updated_at: '',
    })),
  }
}

async function fixture(initial: SkillCatalogItem[], host = false) {
  let catalog = initial
  const exports: any = {}
  const calls: Array<{ id: string; target: string; text: string }> = []
  const mocks: Record<string, unknown> = {
    'vue': require('vue'),
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key, te: () => false, locale: ref('zh-CN') }) },
    'tdesign-vue-next': { MessagePlugin: { error: assert.fail, success: () => {} } },
    '@/stores/ui': { useUIStore: () => ({}) },
    '@/stores/deploymentCapabilities': {
      useDeploymentCapabilitiesStore: () => ({
        ensureLoaded: async () => {},
        isSupported: (key: string) => key.endsWith('.host') ? host : !host,
      }),
    },
    '@/components/settings/useConfirmDelete': { useConfirmDelete: () => () => {} },
    '@/composables/useSkillInstallerModel': {
      useSkillInstallerModel: () => ({ installerModelId: ref(''), savingInstallerModel: ref(false) }),
    },
    '@/composables/useConfigSkillInstallProgress': {
      useConfigSkillInstallProgress: () => ({ percentOf: () => null, sync() {}, stopAll() {} }),
    },
    '@/api/skill': {
      listSkillCatalog: async () => ({ data: catalog }),
      updateSkillCatalogDescription: async (id: string, target: string, text: string) => {
        calls.push({ id, target, text })
        return { success: true, data: { id, description: text, description_i18n: {} } }
      },
      translateSkillCatalogDescription: async () => ({
        success: true,
        data: {
          lang: 'zh', description: '机器译文', saved: false,
          catalog: { id: 'x', description: '', description_i18n: {} },
        },
      }),
    },
    '@/api/system': { listSandboxConfigs: async () => ({ data: [] }) },
    '@/utils/resourceListSearch': { matchesResourceQuery },
    '@/utils/skillTarget': skillTarget,
    '@/utils/skillUpgrade': skillUpgrade,
    '@/utils': {},
    '@/types/mention': { SKILL_ICON: 'tools' },
    '@/utils/assistantDataChange': { useAssistantDataChange() {} },
    'tdesign-icons-vue-next': {},
  }
  runInNewContext(compiled, {
    exports,
    window: { setInterval: () => 1, clearInterval() {} },
    require(name: string) {
      if (name.endsWith('.vue')) return { default: {} }
      assert.ok(name in mocks, `Unexpected import: ${name}`)
      return mocks[name]
    },
  })
  const component = exports.default
  component.render = () => null
  const renderer = createRenderer<any, any>({
    createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
    insert() {}, remove() {}, setElementText() {}, setText() {}, patchProp() {},
    parentNode: () => null, nextSibling: () => null,
  })
  const app = renderer.createApp(component)
  const vm: any = app.mount({})
  await new Promise<void>(resolve => setImmediate(resolve))
  await nextTick()
  return {
    vm, close: () => app.unmount(), calls,
    reload: async (rows: SkillCatalogItem[]) => { catalog = rows; await vm.loadCatalog(true) },
  }
}

const ids = (vm: any) => Array.from(vm.filteredCatalog, (row: any) => row.id)

test('skill search matches names and descriptions without changing order or row identity', async () => {
  const data = [item('财务 Report', ['ready']), item('未安装'), item('mixed', ['ready', 'failed'])]
  data[2]!.description = '财务 REPORT'
  const f = await fixture(data)
  try {
    f.vm.query = ' 财务 report '
    assert.deepEqual(ids(f.vm), ['财务 Report', 'mixed'])
    assert.equal(f.vm.filteredCatalog[1], f.vm.catalog[2])
    f.vm.openCatalogFiles(f.vm.filteredCatalog[1])
    assert.equal(f.vm.filesCatalogId, 'mixed')
    assert.equal(f.vm.catalog.length, 3)
    f.vm.query = 'missing'
    assert.deepEqual(ids(f.vm), [])
    f.vm.query = '  '
    assert.deepEqual(ids(f.vm), data.map(row => row.id))
  } finally { f.close() }
})

for (const host of [false, true]) {
  test(`skill search updates on catalog reload (${host ? 'Lite' : 'Standard'})`, async () => {
    const f = await fixture([item('report'), item('other')], host)
    try {
      f.vm.query = 'report'
      assert.deepEqual(ids(f.vm), ['report'])
      const updated = item('new')
      updated.description = 'Report tools'
      await f.reload([item('other'), updated])
      assert.deepEqual(ids(f.vm), ['new'])
      assert.equal(f.vm.query, 'report')
      await f.reload([])
      assert.deepEqual(ids(f.vm), [])
    } finally { f.close() }
  })
}

// The description language picks which variant the card prints; a language
// without a stored variant falls back to the text parsed out of SKILL.md.
test('skill descriptions follow the selected language and fall back to the source', async () => {
  const data = [item('pdf')]
  data[0]!.description = 'Extract text and tables.'
  data[0]!.description_i18n = { zh: '提取文本与表格。' }
  const f = await fixture(data)
  try {
    assert.equal(f.vm.descriptionFor(f.vm.catalog[0]), '提取文本与表格。')
    f.vm.descriptionLang = 'en'
    assert.equal(f.vm.descriptionFor(f.vm.catalog[0]), 'Extract text and tables.')
    // Translate is offered again only once the visible language has text of
    // its own; the source fallback must not count as a translation.
    assert.equal(f.vm.translateLabel(f.vm.catalog[0]), 'settings.skills.descriptionTranslate')
    f.vm.descriptionLang = 'zh'
    assert.equal(f.vm.translateLabel(f.vm.catalog[0]), 'settings.skills.descriptionRetranslate')
  } finally { f.close() }
})

test('only definitions this workspace can edit expose description editing', async () => {
  const data = [item('own'), item('foreign'), item('readonly')]
  data[0]!.can_edit = true
  data[1]!.can_edit = true
  data[1]!.is_foreign = true
  data[2]!.can_edit = false
  const f = await fixture(data)
  try {
    assert.equal(f.vm.canEditDescription(f.vm.catalog[0]), true)
    assert.equal(f.vm.canEditDescription(f.vm.catalog[1]), false)
    assert.equal(f.vm.canEditDescription(f.vm.catalog[2]), false)
  } finally { f.close() }
})

test('the description editor prefills, translates into the textarea and saves the language', async () => {
  const data = [item('pdf')]
  data[0]!.description = 'Extract text and tables.'
  data[0]!.can_edit = true
  const f = await fixture(data)
  try {
    f.vm.openDescriptionEditor(f.vm.catalog[0])
    assert.equal(f.vm.descriptionEditor.visible, true)
    // No zh variant yet: the editor starts from the source text.
    assert.equal(f.vm.descriptionEditor.text, 'Extract text and tables.')

    await f.vm.translateInEditor()
    assert.equal(f.vm.descriptionEditor.text, '机器译文')

    f.vm.descriptionEditor.text = '  提取文本  '
    await f.vm.saveDescription()
    assert.deepEqual(f.calls, [{ id: 'pdf', target: 'zh', text: '提取文本' }])
    assert.equal(f.vm.descriptionEditor.visible, false)
  } finally { f.close() }
})
