<template>
  <div class="mini-tools">
    <div class="section-header">
      <h2>{{ $t('miniTools.title') }}</h2>
      <p class="section-description">{{ $t('miniTools.description') }}</p>
    </div>

    <div v-if="loading" class="loading-container">
      <t-loading :text="$t('common.loading')" />
    </div>

    <template v-else>
      <Teleport to="#toolbox-toolbar" defer>
        <t-input v-if="tools.length > 0" v-model="query" class="list-search"
          :placeholder="$t('miniTools.searchPlaceholder')" :aria-label="$t('miniTools.searchPlaceholder')" clearable>
          <template #prefix-icon><t-icon name="search" size="16px" /></template>
        </t-input>
      </Teleport>

      <div class="mini-tool-filters">
        <button type="button" class="filter-chip" :class="{ 'is-active': !activeCategory }"
          @click="activeCategory = ''">
          {{ $t('miniTools.allCategories') }}
        </button>
        <button v-for="category in categories" :key="category" type="button" class="filter-chip"
          :class="{ 'is-active': activeCategory === category }" @click="activeCategory = category">
          {{ category }}
        </button>
      </div>

      <div v-if="tools.length === 0" class="empty-state">
        <t-empty :description="$t('miniTools.empty')" />
        <t-button v-if="canCreate" theme="primary" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ $t('miniTools.create') }}
        </t-button>
      </div>

      <div v-else-if="filteredTools.length === 0" class="empty-state">
        <t-empty :description="$t('common.noResult')" />
        <t-button variant="outline" @click="resetFilters">{{ $t('common.clear') }}</t-button>
      </div>

      <div v-else class="tools-grid">
        <article v-for="tool in filteredTools" :key="tool.id" class="tool-card" tabindex="0" role="button"
          @click="openDetail(tool)" @keydown.enter="openDetail(tool)">
          <div class="tool-card__header">
            <span class="tool-card__icon" aria-hidden="true">{{ tool.icon || '🧮' }}</span>
            <div class="tool-card__title-block">
              <h3 class="tool-card__title" :title="tool.name">{{ tool.name }}</h3>
              <div class="tool-card__meta">
                <span class="tool-card__category">{{ tool.category }}</span>
                <span v-if="tool.visibility !== 'private'" class="tool-card__visibility"
                  :class="{ 'tool-card__visibility--public': tool.visibility === 'public' }">
                  {{ visibilityLabel(tool) }}
                </span>
                <span v-if="tool.is_foreign" class="tool-card__visibility tool-card__visibility--foreign">
                  {{ $t('miniTools.foreignBadge') }}
                </span>
                <span v-if="!tool.enabled" class="tool-card__disabled">{{ $t('miniTools.disabled') }}</span>
              </div>
            </div>
            <!-- Every card the caller can do something with gets an overflow
                 menu. The shipped catalogue cards are deliberately not
                 editable in place, so can_edit alone used to leave them with
                 no affordance at all — and therefore no way to reach the
                 visibility switch. can_share covers exactly that gap. -->
            <div v-if="tool.can_edit || tool.can_share || canCreate" class="tool-card__actions" @click.stop>
              <button v-if="tool.can_edit" type="button" class="icon-btn" :title="$t('common.edit')"
                :aria-label="$t('common.edit')" @click="openEdit(tool)">
                <t-icon name="edit" size="14px" />
              </button>
              <t-popup :visible="openMenuToolId === tool.id"
                @visible-change="(visible: boolean) => setOpenMenu(tool.id, visible)" overlayClassName="card-more-popup"
                trigger="click" destroy-on-close placement="bottom-right">
                <button type="button" class="icon-btn" :aria-label="$t('common.expand')"
                  :aria-expanded="openMenuToolId === tool.id" aria-haspopup="true" @click.stop>
                  <t-icon name="ellipsis" size="14px" />
                </button>
                <template #content>
                  <div class="popup-menu" @click.stop>
                    <template v-if="tool.can_share">
                      <div class="popup-menu-section-title">{{ $t('miniTools.visibility') }}</div>
                      <div v-for="opt in VISIBILITY_OPTIONS" :key="opt.value" class="popup-menu-item visibility-option"
                        :class="{ 'is-current': (tool.visibility || 'workspace') === opt.value }"
                        @click.stop="changeVisibility(tool, opt.value)">
                        <t-icon class="menu-icon"
                          :name="(tool.visibility || 'workspace') === opt.value ? 'check' : opt.icon" />
                        <span>{{ $t(opt.labelKey) }}</span>
                      </div>
                      <div class="popup-menu-divider"></div>
                    </template>
                    <div v-if="canCreate" class="popup-menu-item" @click.stop="duplicate(tool)">
                      <t-icon class="menu-icon" name="file-copy" />
                      <span>{{ $t('miniTools.duplicate') }}</span>
                    </div>
                    <div v-if="tool.can_delete" class="popup-menu-item delete" @click.stop="removeTool(tool)">
                      <t-icon class="menu-icon" name="delete" />
                      <span>{{ $t('common.delete') }}</span>
                    </div>
                  </div>
                </template>
              </t-popup>
            </div>
          </div>
          <p class="tool-card__desc">{{ tool.description || $t('miniTools.noDescription') }}</p>
          <div class="tool-card__footer">
            <span><t-icon name="view-list" size="14px" /> {{ $t('miniTools.variableCount', { count: tool.variables?.length || 0 }) }}</span>
            <span><t-icon name="chart-bubble" size="14px" /> {{ $t('miniTools.usedTimes', { count: tool.usage_count }) }}</span>
            <span v-if="tool.owner_name" class="tool-card__owner">{{ tool.owner_name }}</span>
          </div>
        </article>

        <button v-if="canCreate" type="button" class="tool-card tool-card--add" @click="openCreate">
          <t-icon name="add" size="20px" />
          <span>{{ $t('miniTools.create') }}</span>
        </button>
      </div>
    </template>

    <!-- Detail: the whole card, so a reader can check the formula and inputs
         before attaching it to an agent. -->
    <t-dialog v-model:visible="detailVisible" :header="detail?.name" width="680px" :footer="false"
      destroy-on-close>
      <div v-if="detail" class="detail">
        <div class="detail__badges">
          <span class="detail__badge">{{ detail.category }}</span>
          <span class="detail__badge detail__badge--muted">
            {{ visibilityLabel(detail) }}
          </span>
          <span v-if="detail.is_foreign" class="detail__badge detail__badge--muted">
            {{ $t('miniTools.foreignBadge') }}
          </span>
          <span v-if="!detail.enabled" class="detail__badge detail__badge--warn">{{ $t('miniTools.disabled') }}</span>
        </div>
        <p v-if="detail.description" class="detail__desc">{{ detail.description }}</p>

        <h4 class="detail__title">{{ $t('miniTools.formula') }}</h4>
        <pre class="detail__pre">{{ detail.formula || '—' }}</pre>
        <p v-if="detail.formula_note" class="detail__note">{{ detail.formula_note }}</p>

        <template v-if="detail.variables?.length">
          <h4 class="detail__title">{{ $t('miniTools.variables') }}</h4>
          <table class="detail__table">
            <thead>
              <tr>
                <th>{{ $t('miniTools.variableKey') }}</th>
                <th>{{ $t('miniTools.variableLabel') }}</th>
                <th>{{ $t('miniTools.variableUnit') }}</th>
                <th>{{ $t('miniTools.variableDefault') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="variable in detail.variables" :key="variable.key">
                <td><code>{{ variable.key }}</code></td>
                <td>{{ variable.label }}</td>
                <td>{{ variable.unit || '—' }}</td>
                <td>{{ variable.default || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </template>

        <template v-if="detail.knowledge">
          <h4 class="detail__title">{{ $t('miniTools.knowledge') }}</h4>
          <pre class="detail__pre detail__pre--wrap">{{ detail.knowledge }}</pre>
        </template>

        <template v-if="detail.examples?.length">
          <h4 class="detail__title">{{ $t('miniTools.examples') }}</h4>
          <div v-for="(example, index) in detail.examples" :key="index" class="detail__example">
            <strong>{{ example.title || `${$t('miniTools.example')} ${index + 1}` }}</strong>
            <div>{{ $t('miniTools.exampleInput') }}: {{ example.input }}</div>
            <div>{{ $t('miniTools.exampleOutput') }}: {{ example.output }}</div>
          </div>
        </template>

        <div class="detail__actions">
          <t-button v-if="detail.can_edit" variant="outline" @click="openEdit(detail)">
            {{ $t('common.edit') }}
          </t-button>
          <t-button v-if="detail.can_edit" variant="text" @click="duplicate(detail)">
            {{ $t('miniTools.duplicate') }}
          </t-button>
        </div>
        <p class="detail__hint">{{ $t('miniTools.attachHint') }}</p>
      </div>
    </t-dialog>

    <!-- Editor -->
    <t-dialog v-model:visible="editorVisible" :header="editingId ? $t('miniTools.editTitle') : $t('miniTools.createTitle')"
      width="760px" :confirm-btn="{ content: $t('common.save'), loading: saving }" :cancel-btn="{ content: $t('common.cancel') }"
      :close-on-overlay-click="false" destroy-on-close @confirm="save">
      <div class="editor">
        <div class="editor__row">
          <label class="editor__field editor__field--icon">
            <span>{{ $t('miniTools.icon') }}</span>
            <t-input v-model="form.icon" maxlength="8" :placeholder="'🧮'" />
          </label>
          <label class="editor__field">
            <span>{{ $t('miniTools.name') }}</span>
            <t-input v-model="form.name" maxlength="120" :placeholder="$t('miniTools.namePlaceholder')" />
          </label>
          <label class="editor__field">
            <span>{{ $t('miniTools.category') }}</span>
            <t-input v-model="form.category" maxlength="64" :placeholder="$t('miniTools.categoryPlaceholder')" />
          </label>
        </div>

        <label class="editor__field">
          <span>{{ $t('miniTools.descriptionLabel') }}</span>
          <t-textarea v-model="form.description" :autosize="{ minRows: 2, maxRows: 4 }" maxlength="1000" />
        </label>

        <label class="editor__field">
          <span>{{ $t('miniTools.formula') }}</span>
          <t-textarea v-model="form.formula" :autosize="{ minRows: 4, maxRows: 10 }" maxlength="8000"
            :placeholder="$t('miniTools.formulaPlaceholder')" />
        </label>
        <label class="editor__field">
          <span>{{ $t('miniTools.formulaNote') }}</span>
          <t-input v-model="form.formula_note" :placeholder="$t('miniTools.formulaNotePlaceholder')" />
        </label>

        <section class="editor__section">
          <header class="editor__section-head">
            <h4>{{ $t('miniTools.variables') }}</h4>
            <t-button size="small" variant="text" @click="addVariable">
              <template #icon><t-icon name="add" /></template>
              {{ $t('miniTools.addVariable') }}
            </t-button>
          </header>
          <p class="editor__section-hint">{{ $t('miniTools.variablesHint') }}</p>
          <div v-for="(variable, index) in form.variables" :key="index" class="variable-row">
            <t-input v-model="variable.key" class="variable-row__key" :placeholder="$t('miniTools.variableKey')" />
            <t-input v-model="variable.label" :placeholder="$t('miniTools.variableLabel')" />
            <t-input v-model="variable.unit" class="variable-row__unit" :placeholder="$t('miniTools.variableUnit')" />
            <t-input v-model="variable.default" class="variable-row__default" :placeholder="$t('miniTools.variableDefault')" />
            <t-checkbox v-model="variable.required">{{ $t('miniTools.required') }}</t-checkbox>
            <button type="button" class="icon-btn icon-btn--danger" :aria-label="$t('common.delete')"
              @click="form.variables?.splice(index, 1)">
              <t-icon name="delete" size="14px" />
            </button>
          </div>
        </section>

        <label class="editor__field">
          <span>{{ $t('miniTools.knowledge') }}</span>
          <t-textarea v-model="form.knowledge" :autosize="{ minRows: 3, maxRows: 8 }" maxlength="20000"
            :placeholder="$t('miniTools.knowledgePlaceholder')" />
        </label>

        <section class="editor__section">
          <header class="editor__section-head">
            <h4>{{ $t('miniTools.examples') }}</h4>
            <t-button size="small" variant="text" @click="addExample">
              <template #icon><t-icon name="add" /></template>
              {{ $t('miniTools.addExample') }}
            </t-button>
          </header>
          <div v-for="(example, index) in form.examples" :key="index" class="example-row">
            <t-input v-model="example.title" :placeholder="$t('miniTools.exampleTitle')" />
            <t-input v-model="example.input" :placeholder="$t('miniTools.exampleInput')" />
            <t-input v-model="example.output" :placeholder="$t('miniTools.exampleOutput')" />
            <button type="button" class="icon-btn icon-btn--danger" :aria-label="$t('common.delete')"
              @click="form.examples?.splice(index, 1)">
              <t-icon name="delete" size="14px" />
            </button>
          </div>
        </section>

        <div class="editor__row editor__row--options">
          <label class="editor__inline">
            <span>{{ $t('miniTools.visibility') }}</span>
            <t-radio-group v-model="form.visibility">
              <t-radio value="private">{{ $t('miniTools.visibilityPrivate') }}</t-radio>
              <t-radio value="workspace">{{ $t('miniTools.visibilityWorkspace') }}</t-radio>
              <t-radio value="public">{{ $t('miniTools.visibilityPublic') }}</t-radio>
            </t-radio-group>
          </label>
          <label class="editor__inline">
            <span>{{ $t('miniTools.enabled') }}</span>
            <t-switch v-model="form.enabled" />
          </label>
        </div>
        <!-- Publishing a card to the whole deployment is not reversible by the
             reader, so the consequence is spelled out at the moment of choice
             rather than in a tooltip. -->
        <p v-if="form.visibility === 'public'" class="editor__hint">
          {{ $t('miniTools.visibilityPublicHint') }}
        </p>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import {
  createMiniTool,
  deleteMiniTool,
  duplicateMiniTool,
  listMiniToolCategories,
  listMiniTools,
  setMiniToolVisibility,
  updateMiniTool,
  type MiniTool,
  type MiniToolExample,
  type MiniToolPayload,
  type MiniToolVariable,
  type MiniToolVisibility,
} from '@/api/minitool'

const { t } = useI18n()
const emit = defineEmits<{ count: [number] }>()
const authStore = useAuthStore()
// Reads are open to every member; creating a card needs Contributor, which is
// the same bar the backend route guard enforces.
const canCreate = computed(() => authStore.hasRole('contributor'))

// Label for the audience a card is published to. Kept in one place because the
// same wording appears on the card badge, the detail sheet and the editor.
function visibilityLabel(tool: Pick<MiniTool, 'visibility'>): string {
  if (tool.visibility === 'public') return t('miniTools.visibilityPublic')
  if (tool.visibility === 'workspace') return t('miniTools.visibilityWorkspace')
  return t('miniTools.visibilityPrivate')
}

// Same three levels as the knowledge base menu, so the two catalogs read the
// same way. Order runs from most private to most visible.
const VISIBILITY_OPTIONS: Array<{ value: MiniToolVisibility; labelKey: string; icon: string }> = [
  { value: 'private', labelKey: 'miniTools.visibilityPrivate', icon: 'lock-on' },
  { value: 'workspace', labelKey: 'miniTools.visibilityWorkspace', icon: 'usergroup' },
  { value: 'public', labelKey: 'miniTools.visibilityPublic', icon: 'earth' },
]

// Only one card menu is open at a time; tracking the id keeps the popup
// controlled, which is what lets the click on an option close it.
const openMenuToolId = ref('')

function setOpenMenu(id: string, visible: boolean) {
  openMenuToolId.value = visible ? id : (openMenuToolId.value === id ? '' : openMenuToolId.value)
}

const loading = ref(true)
const saving = ref(false)
const tools = ref<MiniTool[]>([])
const categories = ref<string[]>([])
const query = ref('')
const activeCategory = ref('')

const detailVisible = ref(false)
const detail = ref<MiniTool | null>(null)

const editorVisible = ref(false)
const editingId = ref('')
const form = reactive<MiniToolPayload>(emptyForm())

function emptyForm(): MiniToolPayload {
  return {
    name: '',
    icon: '🧮',
    category: '',
    description: '',
    formula: '',
    formula_note: '',
    variables: [],
    knowledge: '',
    examples: [],
    visibility: 'private',
    enabled: true,
  }
}

const filteredTools = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return tools.value.filter((tool) => {
    if (activeCategory.value && tool.category !== activeCategory.value) return false
    if (!keyword) return true
    return [tool.name, tool.description, tool.category, tool.formula]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(keyword))
  })
})

async function load() {
  loading.value = true
  try {
    const [list, cats] = await Promise.all([
      listMiniTools({ include_disabled: true }),
      listMiniToolCategories(),
    ])
    tools.value = list.data ?? []
    categories.value = cats.data ?? []
    emit('count', tools.value.length)
  } catch (error) {
    MessagePlugin.error(t('miniTools.loadFailed'))
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  query.value = ''
  activeCategory.value = ''
}

function openCreate() {
  editingId.value = ''
  Object.assign(form, emptyForm())
  editorVisible.value = true
}

function openEdit(tool: MiniTool) {
  editingId.value = tool.id
  Object.assign(form, {
    name: tool.name,
    icon: tool.icon,
    category: tool.category,
    description: tool.description,
    formula: tool.formula,
    formula_note: tool.formula_note,
    variables: (tool.variables || []).map((variable) => ({ ...variable })),
    knowledge: tool.knowledge,
    examples: (tool.examples || []).map((example) => ({ ...example })),
    visibility: tool.visibility,
    enabled: tool.enabled,
  })
  detailVisible.value = false
  editorVisible.value = true
}

function openDetail(tool: MiniTool) {
  detail.value = tool
  detailVisible.value = true
}

function addVariable() {
  const variable: MiniToolVariable = { key: '', label: '', unit: '', default: '', remark: '', required: true }
  form.variables = [...(form.variables || []), variable]
}

function addExample() {
  const example: MiniToolExample = { title: '', input: '', output: '' }
  form.examples = [...(form.examples || []), example]
}

async function save() {
  if (!form.name.trim()) {
    MessagePlugin.warning(t('miniTools.nameRequired'))
    return
  }
  if (!form.formula?.trim() && !form.knowledge?.trim()) {
    MessagePlugin.warning(t('miniTools.formulaRequired'))
    return
  }
  const keys = (form.variables || []).map((variable) => variable.key.trim())
  if (keys.some((key) => !key)) {
    MessagePlugin.warning(t('miniTools.variableKeyRequired'))
    return
  }
  if (new Set(keys).size !== keys.length) {
    MessagePlugin.warning(t('miniTools.variableKeyDuplicate'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await updateMiniTool(editingId.value, form)
      MessagePlugin.success(t('miniTools.updated'))
    } else {
      await createMiniTool(form)
      MessagePlugin.success(t('miniTools.created'))
    }
    editorVisible.value = false
    await load()
  } catch (error) {
    MessagePlugin.error(t('miniTools.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function removeTool(tool: MiniTool) {
  try {
    await deleteMiniTool(tool.id)
    MessagePlugin.success(t('miniTools.deleted'))
    detailVisible.value = false
    await load()
  } catch (error) {
    MessagePlugin.error(t('miniTools.deleteFailed'))
  }
}

async function duplicate(tool: MiniTool) {
  try {
    await duplicateMiniTool(tool.id)
    MessagePlugin.success(t('miniTools.duplicated'))
    detailVisible.value = false
    await load()
  } catch (error) {
    MessagePlugin.error(t('miniTools.saveFailed'))
  }
}

async function changeVisibility(tool: MiniTool, visibility: MiniToolVisibility) {
  openMenuToolId.value = ''
  if ((tool.visibility || 'workspace') === visibility) return
  try {
    await setMiniToolVisibility(tool.id, visibility)
    MessagePlugin.success(t('miniTools.visibilityUpdated'))
    // The list carries the server-computed permission flags, so re-read it
    // instead of patching the row in place.
    await load()
  } catch (error) {
    MessagePlugin.error(t('miniTools.saveFailed'))
  }
}

watch(detailVisible, (visible) => {
  if (!visible) detail.value = null
})

onMounted(load)

// The toolbox header button calls openAdd() on the active panel, and the tab
// badge reads the count, so both names must match the contract used by
// SkillSettings and McpSettings.
defineExpose({ openAdd: openCreate })
</script>

<style scoped lang="less">
@import (reference) '@/components/css/settings-section.less';

.mini-tools {
  width: 100%;
}

.section-header {
  .settings-section-header();
}

.loading-container {
  padding: 40px 0;
  text-align: center;
}

.empty-state {
  padding: 60px 0;
  text-align: center;

  :deep(.t-empty__description) {
    font-size: var(--app-text-base);
    color: var(--td-text-color-placeholder);
    margin-bottom: 16px;
  }
}

.mini-tool-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 4px 0 14px;
}

.filter-chip {
  padding: 4px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 999px;
  background: transparent;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm);
  cursor: pointer;
  transition: all 0.16s ease;

  &:hover {
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
  }

  &.is-active {
    border-color: var(--td-brand-color);
    background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
    color: var(--td-brand-color);
  }
}

.tools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr));
  gap: 10px;
  align-items: stretch;
}

.tool-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  padding: 14px;
  text-align: left;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;

  &:hover,
  &:focus-visible {
    border-color: color-mix(in srgb, var(--td-brand-color) 40%, var(--td-component-stroke));
    box-shadow: 0 3px 12px color-mix(in srgb, var(--td-text-color-primary) 6%, transparent);
    outline: none;
  }

  &--add {
    align-items: center;
    justify-content: center;
    gap: 6px;
    min-height: 110px;
    border-style: dashed;
    background: transparent;
    color: var(--td-text-color-secondary);
  }
}

.tool-card__header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.tool-card__icon {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  font-size: 20px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--td-brand-color) 8%, transparent);
}

.tool-card__title-block {
  min-width: 0;
  flex: 1;
}

.tool-card__title {
  margin: 0;
  font-size: var(--app-text-base);
  font-weight: 600;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tool-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}

.tool-card__category,
.tool-card__visibility,
.tool-card__disabled {
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
}

.tool-card__visibility {
  color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
}

/* A card published to the whole deployment outranks a workspace preset, so it
   gets a filled badge rather than the same tint every other chip uses. */
.tool-card__visibility--public {
  color: var(--td-text-color-anti);
  background: var(--td-brand-color);
}

/* A card owned by another workspace: readable, never editable. Muted so it
   reads as provenance instead of status. */
.tool-card__visibility--foreign {
  color: var(--td-text-color-secondary);
  background: transparent;
  border: 1px dashed var(--td-component-stroke);
}

.tool-card__disabled {
  color: var(--td-error-color);
  background: color-mix(in srgb, var(--td-error-color) 10%, transparent);
}

.tool-card__actions {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.16s ease;
}

.tool-card:hover .tool-card__actions,
.tool-card:focus-within .tool-card__actions {
  opacity: 1;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
  }

  &--danger:hover {
    color: var(--td-error-color);
  }
}

.tool-card__desc {
  margin: 0;
  min-height: 2.6em;
  font-size: var(--app-text-sm);
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.tool-card__footer {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px dashed var(--td-component-stroke);
  font-size: 12px;
  color: var(--td-text-color-placeholder);

  span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
}

.tool-card__owner {
  margin-left: auto;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 62vh;
  overflow-y: auto;
  padding-right: 4px;
}

.detail__badges {
  display: flex;
  gap: 8px;
}

.detail__badge {
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);

  &--muted {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-secondarycontainer);
  }

  &--warn {
    color: var(--td-warning-color);
    background: color-mix(in srgb, var(--td-warning-color) 12%, transparent);
  }
}

.detail__desc {
  margin: 0;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm);
}

.detail__title {
  margin: 8px 0 0;
  font-size: var(--app-text-sm);
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.detail__pre {
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  font-family: var(--app-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 13px;
  line-height: 1.6;
  white-space: pre;
  overflow-x: auto;

  &--wrap {
    white-space: pre-wrap;
  }
}

.detail__note {
  margin: 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.detail__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;

  th,
  td {
    padding: 6px 8px;
    border-bottom: 1px solid var(--td-component-stroke);
    text-align: left;
    color: var(--td-text-color-secondary);
  }

  th {
    color: var(--td-text-color-placeholder);
    font-weight: 500;
  }
}

.detail__example {
  padding: 8px 10px;
  border-left: 2px solid var(--td-brand-color);
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 0 8px 8px 0;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail__actions {
  display: flex;
  gap: 8px;
  margin-top: 6px;
}

.detail__hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 66vh;
  overflow-y: auto;
  padding-right: 4px;
}

.editor__row {
  display: flex;
  gap: 12px;

  &--options {
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
  }
}

.editor__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
  min-width: 0;
  font-size: var(--app-text-sm);
  color: var(--td-text-color-secondary);

  &--icon {
    flex: 0 0 90px;
  }
}

.editor__inline {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: var(--app-text-sm);
  color: var(--td-text-color-secondary);
}

.editor__hint {
  margin: 6px 0 0;
  padding: 8px 10px;
  border-radius: var(--app-radius-sm);
  font-size: var(--app-text-sm);
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  background: color-mix(in srgb, var(--td-warning-color) 10%, transparent);
}

.editor__section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: var(--app-radius-md);
}

.editor__section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;

  h4 {
    margin: 0;
    font-size: var(--app-text-sm);
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.editor__section-hint {
  margin: -4px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.variable-row,
.example-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.variable-row__key {
  flex: 0 0 120px;
}

.variable-row__unit,
.variable-row__default {
  flex: 0 0 90px;
}
</style>
