<template>
  <span v-if="enabled" class="answer-feedback" @click.stop>
    <t-tooltip :content="t('agent.feedback.helpful')" placement="top">
      <t-button size="small" variant="outline" shape="round" :disabled="busy"
        :class="{ 'answer-feedback__btn--active': rating === 'up' }"
        :aria-label="t('agent.feedback.helpful')" @click.stop="voteUp">
        <t-icon name="thumb-up" />
      </t-button>
    </t-tooltip>

    <span class="answer-feedback__down">
      <t-tooltip :content="t('agent.feedback.notHelpful')" placement="top">
        <t-button size="small" variant="outline" shape="round" :disabled="busy"
          :class="{ 'answer-feedback__btn--active': rating === 'down' }"
          :aria-label="t('agent.feedback.notHelpful')" @click.stop="togglePanel">
          <t-icon name="thumb-down" />
        </t-button>
      </t-tooltip>

      <!-- The reason panel is inline rather than a popup: it must survive the
           chat list's scroll container without being clipped by an overlay. -->
      <div v-if="panelOpen" class="answer-feedback__panel" role="dialog"
        :aria-label="t('agent.feedback.title')">
        <p class="answer-feedback__title">{{ t('agent.feedback.title') }}</p>
        <div class="answer-feedback__reasons">
          <button v-for="reason in reasons" :key="reason.value" type="button"
            class="answer-feedback__chip"
            :class="{ 'answer-feedback__chip--on': selected.includes(reason.value) }"
            @click.stop="toggleReason(reason.value)">
            {{ t(reason.label) }}
          </button>
        </div>
        <t-textarea v-model="comment" :placeholder="t('agent.feedback.commentPlaceholder')"
          :maxlength="500" :autosize="{ minRows: 2, maxRows: 4 }" />
        <div class="answer-feedback__actions">
          <t-button size="small" variant="text" @click.stop="closePanel">
            {{ t('agent.feedback.cancel') }}
          </t-button>
          <t-button size="small" theme="primary" :loading="busy" @click.stop="submitDown">
            {{ t('agent.feedback.submit') }}
          </t-button>
        </div>
      </div>
    </span>

    <span v-if="justSubmitted" class="answer-feedback__thanks" role="status">
      {{ t('agent.feedback.thanks') }}
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import {
  getMyFeedback,
  submitFeedback,
  type FeedbackRating,
  type FeedbackReason,
} from '../../../api/agentops';

const props = defineProps<{
  sessionId?: string;
  messageId?: string;
}>();

const { t } = useI18n();

// Both ids are required: the backend resolves the agent from the stored
// message, and a vote without them cannot be attributed.
const enabled = computed(() => !!(props.sessionId && props.messageId));

const rating = ref<FeedbackRating | ''>('');
const selected = ref<FeedbackReason[]>([]);
const comment = ref('');
const panelOpen = ref(false);
const busy = ref(false);
const justSubmitted = ref(false);

const reasons: Array<{ value: FeedbackReason; label: string }> = [
  { value: 'wrong_answer', label: 'agent.feedback.reasons.wrongAnswer' },
  { value: 'bad_citation', label: 'agent.feedback.reasons.badCitation' },
  { value: 'incomplete', label: 'agent.feedback.reasons.incomplete' },
  { value: 'too_slow', label: 'agent.feedback.reasons.tooSlow' },
  { value: 'bad_format', label: 'agent.feedback.reasons.badFormat' },
  { value: 'tool_failure', label: 'agent.feedback.reasons.toolFailure' },
  { value: 'unsafe', label: 'agent.feedback.reasons.unsafe' },
  { value: 'other', label: 'agent.feedback.reasons.other' },
];

onMounted(async () => {
  if (!enabled.value) return;
  try {
    const res = await getMyFeedback(props.messageId as string);
    const existing = res?.data;
    if (existing) {
      rating.value = existing.rating;
      selected.value = (existing.reason_tags || []) as FeedbackReason[];
      comment.value = existing.comment || '';
    }
  } catch {
    // A missing vote is the normal case; failing to read it must not break
    // the toolbar.
  }
});

function toggleReason(reason: FeedbackReason) {
  selected.value = selected.value.includes(reason)
    ? selected.value.filter((r) => r !== reason)
    : [...selected.value, reason];
}

function closePanel() {
  panelOpen.value = false;
}

function togglePanel() {
  panelOpen.value = !panelOpen.value;
}

async function send(nextRating: FeedbackRating, reasonTags: FeedbackReason[], text: string) {
  if (!props.sessionId || !props.messageId) return;
  busy.value = true;
  try {
    await submitFeedback({
      session_id: props.sessionId,
      message_id: props.messageId,
      rating: nextRating,
      reason_tags: reasonTags,
      comment: text,
    });
    rating.value = nextRating;
    panelOpen.value = false;
    justSubmitted.value = true;
    window.setTimeout(() => { justSubmitted.value = false; }, 2500);
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('agent.feedback.failed'));
  } finally {
    busy.value = false;
  }
}

function voteUp() {
  selected.value = [];
  comment.value = '';
  void send('up', [], '');
}

function submitDown() {
  void send('down', selected.value, comment.value);
}
</script>

<style scoped>
.answer-feedback {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.answer-feedback__down {
  position: relative;
  display: inline-flex;
}

.answer-feedback__btn--active {
  border-color: var(--td-brand-color, #0052d9);
  color: var(--td-brand-color, #0052d9);
}

.answer-feedback__panel {
  position: absolute;
  bottom: calc(100% + 8px);
  right: 0;
  z-index: 20;
  width: 268px;
  padding: 12px;
  border: 1px solid var(--td-component-border, #e7e7e7);
  border-radius: 8px;
  background: var(--td-bg-color-container, #fff);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  text-align: left;
}

.answer-feedback__title {
  margin: 0 0 8px;
  font-size: 13px;
  color: var(--td-text-color-primary, #000);
}

.answer-feedback__reasons {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}

.answer-feedback__chip {
  padding: 2px 8px;
  font-size: 12px;
  line-height: 20px;
  border: 1px solid var(--td-component-border, #e7e7e7);
  border-radius: 12px;
  background: transparent;
  color: var(--td-text-color-secondary, #666);
  cursor: pointer;
}

.answer-feedback__chip--on {
  border-color: var(--td-brand-color, #0052d9);
  color: var(--td-brand-color, #0052d9);
  background: var(--td-brand-color-light, #f2f3ff);
}

.answer-feedback__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}

.answer-feedback__thanks {
  font-size: 12px;
  color: var(--td-success-color, #2ba471);
}
</style>
