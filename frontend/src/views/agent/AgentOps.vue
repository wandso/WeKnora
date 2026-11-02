<template>
  <div class="agent-ops">
    <div class="agent-ops__head">
      <div>
        <h1>{{ t('agent.ops.title') }}</h1>
        <p>{{ t('agent.ops.subtitle') }}</p>
      </div>
      <t-select
        v-model="agentId"
        :options="agentOptions"
        filterable
        :placeholder="t('agent.ops.selectAgent')"
        style="width: 260px"
      />
    </div>

    <t-tabs v-model="tab">
      <t-tab-panel value="overview" :label="t('agent.ops.tabs.overview')">
        <div class="agent-ops__kpis">
          <div class="kpi">
            <span class="kpi__label">{{ t('agent.ops.kpi.turns') }}</span>
            <strong>{{ report?.usage.turns ?? 0 }}</strong>
          </div>
          <div class="kpi">
            <span class="kpi__label">{{ t('agent.ops.kpi.totalTokens') }}</span>
            <strong>{{ report?.usage.total_tokens ?? 0 }}</strong>
          </div>
          <div class="kpi">
            <span class="kpi__label">{{ t('agent.ops.kpi.avgSteps') }}</span>
            <strong>{{ (report?.usage.avg_steps ?? 0).toFixed(1) }}</strong>
          </div>
          <div class="kpi">
            <span class="kpi__label">{{ t('agent.ops.kpi.avgLatency') }}</span>
            <strong>{{ Math.round(report?.usage.avg_latency_ms ?? 0) }} ms</strong>
          </div>
          <div class="kpi">
            <span class="kpi__label">{{ t('agent.ops.kpi.upRate') }}</span>
            <strong>{{ ((report?.feedback.up_rate ?? 0) * 100).toFixed(1) }}%</strong>
          </div>
          <div class="kpi">
            <span class="kpi__label">{{ t('agent.ops.kpi.downCount') }}</span>
            <strong>{{ report?.feedback.down ?? 0 }}</strong>
          </div>
        </div>

        <div v-if="reasonRows.length" class="agent-ops__reasons">
          <h3>{{ t('agent.ops.reasonDistribution') }}</h3>
          <div v-for="row in reasonRows" :key="row.key" class="reason-row">
            <span>{{ reasonLabel(row.key) }}</span>
            <t-progress :percentage="row.pct" :label="String(row.count)" />
          </div>
        </div>

        <div v-if="series.length" class="agent-ops__series">
          <h3>{{ t('agent.ops.dailyTurns') }}</h3>
          <div v-for="point in series" :key="point.bucket" class="series-row">
            <span>{{ point.bucket }}</span>
            <t-progress :percentage="seriesPct(point.turns)" :label="String(point.turns)" />
          </div>
        </div>

        <h3 class="agent-ops__section">{{ t('agent.ops.recentFeedback') }}</h3>
        <t-table row-key="id" :data="feedback" :columns="feedbackColumns" size="small" />
      </t-tab-panel>

      <t-tab-panel value="versions" :label="t('agent.ops.tabs.versions')">
        <div class="agent-ops__toolbar">
          <t-button
            size="small"
            variant="outline"
            :disabled="selectedVersions.length !== 2"
            @click="compareVersions"
          >
            {{ t('agent.ops.diff.action') }}
          </t-button>
          <t-button size="small" :loading="publishing" :disabled="!agentId" @click="publish">
            {{ t('agent.ops.publish') }}
          </t-button>
        </div>
        <t-table
          row-key="id"
          :data="versions"
          :columns="versionColumns"
          size="small"
          :selected-row-keys="selectedVersions"
          @select-change="onSelectVersions"
        >
          <template #source="{ row }">
            <t-tag size="small" variant="light">{{ sourceLabel(row.source) }}</t-tag>
          </template>
          <template #op="{ row }">
            <t-popconfirm :content="t('agent.ops.rollbackConfirm')" @confirm="rollback(row)">
              <t-button size="small" variant="text" theme="danger">
                {{ t('agent.ops.rollback') }}
              </t-button>
            </t-popconfirm>
          </template>
        </t-table>
      </t-tab-panel>

      <t-tab-panel value="templates" :label="t('agent.ops.tabs.templates')">
        <div class="agent-ops__toolbar">
          <t-button size="small" :disabled="!agentId" :loading="savingTemplate" @click="saveAsTemplate">
            {{ t('agent.ops.saveAsTemplate') }}
          </t-button>
        </div>
        <t-table row-key="id" :data="templates" :columns="templateColumns" size="small">
          <template #source="{ row }">
            <t-tag size="small" :theme="row.source === 'builtin' ? 'primary' : 'success'" variant="light">
              {{ row.source === 'builtin' ? t('agent.ops.sourceBuiltin') : t('agent.ops.sourceTenant') }}
            </t-tag>
          </template>
          <template #op="{ row }">
            <t-button size="small" variant="text" @click="install(row)">
              {{ t('agent.ops.install') }}
            </t-button>
          </template>
        </t-table>
      </t-tab-panel>

      <t-tab-panel value="moderation" :label="t('agent.ops.moderation.tab')">
        <p class="agent-ops__hint">{{ t('agent.ops.moderation.hint') }}</p>
        <div class="agent-ops__toolbar">
          <t-button
            size="small"
            variant="outline"
            :loading="scanning"
            :disabled="!agentId"
            @click="runScan"
          >
            {{ scanning ? t('agent.ops.moderation.scanning') : t('agent.ops.moderation.scan') }}
          </t-button>
          <t-button size="small" @click="openRuleDialog()">
            {{ t('agent.ops.moderation.addRule') }}
          </t-button>
        </div>

        <h3 class="agent-ops__section">{{ t('agent.ops.moderation.hits') }}</h3>
        <t-table row-key="id" :data="hits" :columns="hitColumns" size="small">
          <template #empty>{{ t('agent.ops.moderation.hitsEmpty') }}</template>
        </t-table>

        <h3 class="agent-ops__section agent-ops__section--spaced">
          {{ t('agent.ops.moderation.rules') }}
        </h3>
        <p class="agent-ops__hint">{{ t('agent.ops.moderation.builtinHint') }}</p>
        <t-table row-key="id" :data="rules" :columns="ruleColumns" size="small">
          <template #enabled="{ row }">
            <t-tag size="small" :theme="row.enabled ? 'success' : 'default'" variant="light">
              {{ row.enabled ? t('agent.ops.moderation.enabled') : t('agent.ops.moderation.disabled') }}
            </t-tag>
          </template>
          <template #op="{ row }">
            <t-button size="small" variant="text" @click="openRuleDialog(row)">
              {{ t('agent.ops.moderation.editRule') }}
            </t-button>
            <t-popconfirm
              :content="t('agent.ops.moderation.deleteRuleConfirm')"
              @confirm="removeRule(row)"
            >
              <t-button size="small" variant="text" theme="danger">
                {{ t('agent.ops.moderation.deleteRule') }}
              </t-button>
            </t-popconfirm>
          </template>
        </t-table>
      </t-tab-panel>
    </t-tabs>

    <t-dialog
      v-model:visible="diffVisible"
      :header="t('agent.ops.diff.title')"
      :footer="false"
      width="640px"
    >
      <div v-if="diff" class="diff">
        <p class="diff__summary">
          v{{ diff.from }} → v{{ diff.to }} ·
          {{ diffCount ? t('agent.ops.diff.summary', { count: diffCount }) : t('agent.ops.diff.same') }}
        </p>
        <ul v-if="diffCount" class="diff__list">
          <li v-if="diff.name_changed">{{ t('agent.ops.diff.name') }}</li>
          <li v-if="diff.description_changed">{{ t('agent.ops.diff.description') }}</li>
          <li v-if="diff.avatar_changed">{{ t('agent.ops.diff.avatar') }}</li>
          <li v-for="key in diff.changed_keys" :key="key"><code>{{ key }}</code></li>
        </ul>
      </div>
    </t-dialog>

    <t-dialog
      v-model:visible="ruleVisible"
      :header="ruleForm.id ? t('agent.ops.moderation.editRule') : t('agent.ops.moderation.addRule')"
      :on-confirm="saveRule"
      :confirm-btn="t('agent.ops.moderation.save')"
      :cancel-btn="t('agent.ops.moderation.cancel')"
      width="480px"
    >
      <t-form label-align="top">
        <t-form-item :label="t('agent.ops.moderation.ruleName')">
          <t-input
            v-model="ruleForm.name"
            :placeholder="t('agent.ops.moderation.ruleNamePlaceholder')"
          />
        </t-form-item>
        <t-form-item :label="t('agent.ops.moderation.rulePattern')">
          <t-input
            v-model="ruleForm.pattern"
            :placeholder="t('agent.ops.moderation.rulePatternPlaceholder')"
          />
        </t-form-item>
        <t-form-item>
          <t-checkbox v-model="ruleForm.is_regex">
            {{ t('agent.ops.moderation.ruleIsRegex') }}
          </t-checkbox>
          <t-checkbox v-model="ruleForm.enabled">
            {{ t('agent.ops.moderation.ruleEnabled') }}
          </t-checkbox>
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { listAgents } from '../../api/agent';
import {
  deleteModerationRule,
  diffAgentVersions,
  getAgentQuality,
  getAgentUsageTimeseries,
  listAgentFeedback,
  listAgentVersions,
  listModerationHits,
  listModerationRules,
  publishAgentVersion,
  rollbackAgentVersion,
  listAgentTemplates,
  installAgentTemplate,
  saveAgentAsTemplate,
  scanModeration,
  upsertModerationRule,
  type AgentQualityReport,
  type AgentVersion,
  type AgentVersionDiff,
  type AgentTemplate,
  type MessageFeedback,
  type ModerationHit,
  type ModerationRule,
} from '../../api/agentops';

const { t } = useI18n();

const agentId = ref('');
const tab = ref('overview');
const agents = ref<any[]>([]);
const report = ref<AgentQualityReport | null>(null);
const series = ref<Array<{ bucket: string; turns: number; total_tokens: number }>>([]);
const feedback = ref<MessageFeedback[]>([]);
const versions = ref<AgentVersion[]>([]);
const templates = ref<AgentTemplate[]>([]);
const publishing = ref(false);
const savingTemplate = ref(false);

// Version comparison. The backend only reports which top-level keys differ, so
// the UI is a checklist rather than a rendered diff — enough to answer "what
// changed between these two publishes" without a structural diff viewer.
const selectedVersions = ref<Array<string | number>>([]);
const diffVisible = ref(false);
const diff = ref<AgentVersionDiff | null>(null);

// Content moderation (observation phase). Nothing here can alter a
// conversation: scanning only appends audit rows.
const hits = ref<ModerationHit[]>([]);
const rules = ref<ModerationRule[]>([]);
const scanning = ref(false);
const ruleVisible = ref(false);
const ruleForm = ref({
  id: '',
  name: '',
  pattern: '',
  is_regex: false,
  enabled: true,
});

const agentOptions = computed(() =>
  agents.value.map((a) => ({ label: a.name, value: a.id })),
);

// Backend reason tags are snake_case; i18n keys are camelCase.
const REASON_KEYS: Record<string, string> = {
  wrong_answer: 'wrongAnswer',
  bad_citation: 'badCitation',
  incomplete: 'incomplete',
  too_slow: 'tooSlow',
  bad_format: 'badFormat',
  tool_failure: 'toolFailure',
  unsafe: 'unsafe',
  other: 'other',
};

const reasonLabel = (key: string) => {
  const i18nKey = REASON_KEYS[key];
  return i18nKey ? t(`agent.feedback.reasons.${i18nKey}`) : key;
};

const reasonRows = computed(() => {
  const reasons = report.value?.feedback.reasons || {};
  const total = Object.values(reasons).reduce((sum, n) => sum + n, 0) || 1;
  return Object.entries(reasons)
    .sort((a, b) => b[1] - a[1])
    .map(([key, count]) => ({ key, count, pct: Math.round((count / total) * 100) }));
});

const seriesPct = (turns: number) => {
  const max = Math.max(...series.value.map((p) => p.turns), 1);
  return Math.round((turns / max) * 100);
};

const sourceLabel = (source: string) => t(`agent.ops.versionSource.${source}`, source);

const diffCount = computed(() => {
  if (!diff.value) return 0;
  return (
    diff.value.changed_keys.length +
    (diff.value.name_changed ? 1 : 0) +
    (diff.value.description_changed ? 1 : 0) +
    (diff.value.avatar_changed ? 1 : 0)
  );
});

const feedbackColumns = computed(() => [
  { colKey: 'rating', title: t('agent.ops.columns.rating'), width: 80 },
  {
    colKey: 'reason_tags',
    title: t('agent.ops.columns.reason'),
    cell: (_h: any, { row }: any) =>
      (row.reason_tags || []).map((tag: string) => reasonLabel(tag)).join(', ') || '-',
  },
  { colKey: 'comment', title: t('agent.ops.columns.comment') },
  { colKey: 'created_at', title: t('agent.ops.columns.time'), width: 180 },
]);

const versionColumns = computed(() => [
  { colKey: 'row-select', type: 'multiple', width: 50 },
  { colKey: 'version_no', title: t('agent.ops.columns.version'), width: 80 },
  { colKey: 'changelog', title: t('agent.ops.columns.changelog') },
  { colKey: 'source', title: t('agent.ops.columns.source'), width: 100 },
  { colKey: 'created_at', title: t('agent.ops.columns.time'), width: 180 },
  { colKey: 'op', title: t('agent.ops.columns.action'), width: 80 },
]);

const templateColumns = computed(() => [
  { colKey: 'name', title: t('agent.ops.columns.name') },
  { colKey: 'category', title: t('agent.ops.columns.category'), width: 100 },
  { colKey: 'source', title: t('agent.ops.columns.source'), width: 90 },
  { colKey: 'installs', title: t('agent.ops.columns.installs'), width: 90 },
  { colKey: 'op', title: t('agent.ops.columns.action'), width: 80 },
]);

const hitColumns = computed(() => [
  { colKey: 'rule_name', title: t('agent.ops.moderation.columns.rule'), width: 160 },
  { colKey: 'category', title: t('agent.ops.moderation.columns.category'), width: 90 },
  {
    colKey: 'direction',
    title: t('agent.ops.moderation.columns.direction'),
    width: 80,
    cell: (_h: any, { row }: any) => t(`agent.ops.moderation.direction.${row.direction}`, row.direction),
  },
  { colKey: 'snippet', title: t('agent.ops.moderation.columns.snippet'), ellipsis: true },
  { colKey: 'created_at', title: t('agent.ops.moderation.columns.time'), width: 180 },
]);

const ruleColumns = computed(() => [
  { colKey: 'name', title: t('agent.ops.moderation.columns.rule'), width: 160 },
  { colKey: 'pattern', title: t('agent.ops.moderation.columns.pattern'), ellipsis: true },
  { colKey: 'enabled', title: t('agent.ops.moderation.columns.enabled'), width: 90 },
  { colKey: 'op', title: t('agent.ops.moderation.columns.action'), width: 140 },
]);

function onSelectVersions(keys: Array<string | number>) {
  // Keep the two most recent picks so a third click does not silently compare
  // the wrong pair.
  selectedVersions.value = keys.length > 2 ? keys.slice(-2) : keys;
}

async function loadAll() {
  if (!agentId.value) return;
  selectedVersions.value = [];
  try {
    const [q, f, v] = await Promise.all([
      getAgentQuality(agentId.value),
      listAgentFeedback(agentId.value, '', 20, 0),
      listAgentVersions(agentId.value),
    ]);
    report.value = q?.data ?? null;
    feedback.value = f?.data ?? [];
    versions.value = v?.data ?? [];
    series.value = (await getAgentUsageTimeseries(agentId.value))?.data ?? [];
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.loadFailed'));
  }
}

async function loadModeration() {
  if (!agentId.value) return;
  try {
    const [h, r] = await Promise.all([
      listModerationHits(agentId.value),
      listModerationRules(),
    ]);
    hits.value = h?.data ?? [];
    rules.value = r?.data ?? [];
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.loadFailed'));
  }
}

async function runScan() {
  if (!agentId.value) return;
  scanning.value = true;
  try {
    const res = await scanModeration(agentId.value);
    const result = res?.data;
    if (result) {
      MessagePlugin.success(
        t('agent.ops.moderation.scanDone', {
          scanned: result.scanned,
          matched: result.matched,
          rules: result.rules,
        })
      );
    }
    await loadModeration();
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.moderation.scanFailed'));
  } finally {
    scanning.value = false;
  }
}

function openRuleDialog(row?: ModerationRule) {
  ruleForm.value = row
    ? {
        id: row.id,
        name: row.name,
        pattern: row.pattern,
        is_regex: row.is_regex,
        enabled: row.enabled,
      }
    : { id: '', name: '', pattern: '', is_regex: false, enabled: true };
  ruleVisible.value = true;
}

// Returning false keeps the dialog open when the request fails, so the user
// does not lose what they typed.
async function saveRule(): Promise<boolean> {
  try {
    await upsertModerationRule({ ...ruleForm.value });
    MessagePlugin.success(t('agent.ops.moderation.ruleSaved'));
    ruleVisible.value = false;
    rules.value = (await listModerationRules())?.data ?? rules.value;
    return true;
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.moderation.ruleSaveFailed'));
    return false;
  }
}

async function removeRule(row: ModerationRule) {
  try {
    await deleteModerationRule(row.id);
    MessagePlugin.success(t('agent.ops.moderation.ruleDeleted'));
    rules.value = (await listModerationRules())?.data ?? [];
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.moderation.ruleDeleteFailed'));
  }
}

async function compareVersions() {
  if (selectedVersions.value.length !== 2) {
    MessagePlugin.warning(t('agent.ops.diff.pickTwo'));
    return;
  }
  const picks = selectedVersions.value
    .map((key) => versions.value.find((v) => String(v.id) === String(key)))
    .filter((v): v is AgentVersion => Boolean(v))
    .map((v) => v.version_no)
    .sort((a, b) => a - b);
  if (picks.length !== 2) {
    MessagePlugin.warning(t('agent.ops.diff.pickTwo'));
    return;
  }
  try {
    const res = await diffAgentVersions(agentId.value, picks[0], picks[1]);
    diff.value = res?.data ?? null;
    diffVisible.value = true;
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.diff.failed'));
  }
}

async function publish() {
  publishing.value = true;
  try {
    await publishAgentVersion(agentId.value, '');
    MessagePlugin.success(t('agent.ops.published'));
    await loadAll();
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.publishFailed'));
  } finally {
    publishing.value = false;
  }
}

async function rollback(row: AgentVersion) {
  try {
    await rollbackAgentVersion(agentId.value, row.version_no);
    MessagePlugin.success(t('agent.ops.rolledBack', { version: row.version_no }));
    await loadAll();
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.rollbackFailed'));
  }
}

async function saveAsTemplate() {
  savingTemplate.value = true;
  try {
    await saveAgentAsTemplate(agentId.value, {});
    MessagePlugin.success(t('agent.ops.savedAsTemplate'));
    templates.value = (await listAgentTemplates())?.data ?? [];
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.loadFailed'));
  } finally {
    savingTemplate.value = false;
  }
}

async function install(row: AgentTemplate) {
  try {
    await installAgentTemplate(row.id);
    MessagePlugin.success(t('agent.ops.installed'));
    agents.value = (await listAgents())?.data ?? agents.value;
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.ops.installFailed'));
  }
}

watch(agentId, (id) => {
  void loadAll();
  if (tab.value === 'moderation' && id) void loadModeration();
});

// Moderation data is only fetched when the tab is opened: the hit log is the
// largest table on the page and most visits never look at it.
watch(tab, (value) => {
  if (value === 'moderation') void loadModeration();
});

onMounted(async () => {
  try {
    const res: any = await listAgents();
    agents.value = res?.data ?? [];
    if (agents.value.length) agentId.value = agents.value[0].id;
    templates.value = (await listAgentTemplates())?.data ?? [];
  } catch {
    // The page is still usable once an agent is picked manually.
  }
});
</script>

<style scoped>
.agent-ops { padding: 24px; max-width: 1100px; margin: 0 auto; }
.agent-ops__head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; margin-bottom: 16px; }
.agent-ops__head h1 { margin: 0 0 4px; font-size: 20px; font-weight: 500; }
.agent-ops__head p { margin: 0; font-size: 13px; color: var(--td-text-color-secondary, #666); }
.agent-ops__kpis { display: grid; grid-template-columns: repeat(6, 1fr); gap: 12px; margin: 16px 0; }
.kpi { border: 1px solid var(--td-component-border, #e7e7e7); border-radius: 8px; padding: 12px; display: flex; flex-direction: column; gap: 6px; }
.kpi__label { font-size: 12px; color: var(--td-text-color-secondary, #666); }
.kpi strong { font-size: 20px; font-weight: 500; }
.agent-ops__reasons { margin: 8px 0 20px; }
.agent-ops__reasons h3, .agent-ops__section { font-size: 14px; font-weight: 500; margin: 0 0 8px; }
.reason-row { display: grid; grid-template-columns: 100px 1fr; gap: 12px; align-items: center; margin-bottom: 6px; font-size: 12px; }
.agent-ops__series { margin-bottom: 20px; }
.series-row { display: grid; grid-template-columns: 100px 1fr; gap: 12px; align-items: center; margin-bottom: 6px; font-size: 12px; }
.agent-ops__toolbar { display: flex; justify-content: flex-end; gap: 8px; margin: 12px 0; }
.agent-ops__hint { margin: 0 0 12px; font-size: 12px; line-height: 1.7; color: var(--td-text-color-secondary, #666); }
.agent-ops__section--spaced { margin-top: 24px; }
.diff__summary { margin: 0 0 12px; font-size: 13px; color: var(--td-text-color-secondary, #666); }
.diff__list { margin: 0; padding-left: 18px; font-size: 13px; line-height: 1.9; }
.diff__list code { font-size: 12px; background: var(--td-bg-color-secondarycontainer, #f3f3f3); padding: 1px 6px; border-radius: 4px; }
@media (max-width: 900px) { .agent-ops__kpis { grid-template-columns: repeat(2, 1fr); } }
</style>
