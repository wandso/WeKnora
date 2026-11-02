<template>
  <t-dialog v-model:visible="visible" :header="title" :footer="false" width="640px" destroy-on-close>
    <div class="dry-run">
      <p class="dry-run__hint">{{ t('agent.dryRun.hint') }}</p>

      <t-textarea v-model="query" :placeholder="t('agent.dryRun.queryPlaceholder')"
        :autosize="{ minRows: 2, maxRows: 4 }" :maxlength="2000" />

      <div class="dry-run__actions">
        <t-button theme="primary" :loading="running" :disabled="!query.trim()" @click="run">
          {{ running ? t('agent.dryRun.running') : t('agent.dryRun.run') }}
        </t-button>
        <span v-if="elapsed" class="dry-run__meta">{{ elapsed }} ms</span>
      </div>

      <t-alert v-if="error" theme="error" :message="error" class="dry-run__error" />

      <div v-if="result" class="dry-run__result">
        <div class="dry-run__answer">{{ result.answer }}</div>

        <div v-if="result.usage" class="dry-run__stats">
          <span>prompt {{ result.usage.prompt_tokens }}</span>
          <span>completion {{ result.usage.completion_tokens }}</span>
          <span>total {{ result.usage.total_tokens }}</span>
        </div>

        <details v-if="result.steps && result.steps.length" class="dry-run__steps">
          <summary>{{ t('agent.dryRun.steps', { count: result.steps.length }) }}</summary>
          <ol>
            <li v-for="(step, index) in result.steps" :key="index">
              <span class="dry-run__step-tool">{{ step.tool_name || step.type || 'step' }}</span>
              <span v-if="step.tool_output" class="dry-run__step-output">{{ step.tool_output }}</span>
            </li>
          </ol>
        </details>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { dryRunAgent } from '../../api/agentops';

const props = defineProps<{
  visible: boolean;
  agentId?: string;
  agentName?: string;
  draftConfig: Record<string, unknown>;
}>();

const emit = defineEmits<{ 'update:visible': [value: boolean] }>();

const { t } = useI18n();

const visible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
});

const title = computed(() => `${t('agent.dryRun.title')}${props.agentName ? ' · ' + props.agentName : ''}`);

const query = ref('');
const running = ref(false);
const error = ref('');
const elapsed = ref(0);
const result = ref<{ answer: string; usage?: any; steps?: any[] } | null>(null);

// A new draft invalidates the previous answer: keeping it on screen next to
// edited settings would read as if it had been produced by them.
watch(() => props.draftConfig, () => { result.value = null; error.value = ''; }, { deep: true });

async function run() {
  if (!props.agentId) return;
  running.value = true;
  error.value = '';
  result.value = null;
  try {
    const res = await dryRunAgent(props.agentId, props.draftConfig, query.value.trim());
    result.value = res?.data ?? null;
    elapsed.value = res?.data?.duration_ms ?? 0;
  } catch (err: any) {
    error.value = err?.message || t('agent.dryRun.failed');
    MessagePlugin.error(error.value);
  } finally {
    running.value = false;
  }
}
</script>

<style scoped>
.dry-run { display: flex; flex-direction: column; gap: 12px; }
.dry-run__hint { margin: 0; font-size: 12px; color: var(--td-text-color-secondary, #666); }
.dry-run__actions { display: flex; align-items: center; gap: 12px; }
.dry-run__meta { font-size: 12px; color: var(--td-text-color-secondary, #666); }
.dry-run__result { border: 1px solid var(--td-component-border, #e7e7e7); border-radius: 8px; padding: 12px; }
.dry-run__answer { white-space: pre-wrap; font-size: 13px; line-height: 1.7; max-height: 320px; overflow: auto; }
.dry-run__stats { display: flex; gap: 12px; margin-top: 8px; font-size: 12px; color: var(--td-text-color-secondary, #666); }
.dry-run__steps { margin-top: 8px; font-size: 12px; }
.dry-run__step-tool { font-weight: 500; margin-right: 6px; }
.dry-run__step-output { color: var(--td-text-color-secondary, #666); }
</style>
