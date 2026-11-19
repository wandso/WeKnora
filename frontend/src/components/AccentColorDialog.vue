<template>
  <t-dialog
    :visible="visible"
    :header="t('theme.customColorTitle')"
    width="440px"
    placement="center"
    :confirm-btn="{ content: t('common.confirm'), theme: 'primary' }"
    :cancel-btn="{ content: t('common.cancel') }"
    @confirm="confirm"
    @cancel="close"
  >
    <div class="accent-dialog">
      <p class="accent-dialog__desc">{{ t('theme.customColorDescription') }}</p>

      <div
        class="accent-dialog__preview"
        :style="{ background: safeDraft, color: previewTextColor }"
      >
        <span class="accent-dialog__preview-label">{{ t('theme.customColorPreview') }}</span>
        <span class="accent-dialog__preview-hex">{{ safeDraft.toUpperCase() }}</span>
      </div>

      <div class="accent-dialog__field">
        <label class="accent-dialog__label">{{ t('theme.customColorPick') }}</label>
        <t-color-picker
          v-model="draft"
          format="HEX"
          :enable-alpha="false"
          :swatch-colors="presets"
        />
      </div>

      <div class="accent-dialog__field">
        <label class="accent-dialog__label">{{ t('theme.customColorPresets') }}</label>
        <div class="accent-dialog__presets">
          <button
            v-for="color in presets"
            :key="color"
            type="button"
            class="accent-dialog__preset"
            :class="{ 'is-current': color === safeDraft }"
            :style="{ background: color }"
            :title="color.toUpperCase()"
            :aria-label="color.toUpperCase()"
            @click="draft = color"
          />
        </div>
      </div>

      <p v-if="!readable" class="accent-dialog__warning">
        <t-icon name="info-circle" size="14px" />
        {{ t('theme.customColorLowContrast') }}
      </p>

      <div class="accent-dialog__footer">
        <t-button variant="text" size="small" @click="reset">
          {{ t('theme.customColorReset') }}
        </t-button>
        <span class="accent-dialog__footer-hint">{{ t('theme.customColorHint') }}</span>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useTheme } from '@/composables/useTheme'
import {
  DEFAULT_ACCENT_COLOR,
  isAccentReadable,
  normalizeHex,
} from '@/composables/accentColor'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [value: boolean] }>()

const { t } = useI18n()
const { accentColor, setAccentColor } = useTheme()

const presets = [
  '#07c05f', // WeKnora green — the built-in default
  '#2563eb',
  '#7c3aed',
  '#db2777',
  '#ea580c',
  '#0891b2',
  '#0d9488',
  '#475569',
]

const draft = ref<string>(DEFAULT_ACCENT_COLOR)

// A colour picker may hand back uppercase hex or a mid-edit value; keep the
// preview and the persisted value strictly #rrggbb.
const safeDraft = computed(() => normalizeHex(draft.value) ?? DEFAULT_ACCENT_COLOR)
const readable = computed(() => isAccentReadable(safeDraft.value))
const previewTextColor = computed(() => {
  // Simple perceived-brightness split, enough for a one-line preview label.
  const hex = safeDraft.value.slice(1)
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16))
  return (r * 299 + g * 587 + b * 114) / 1000 > 150 ? '#181818' : '#ffffff'
})

watch(
  () => props.visible,
  (open) => {
    if (open) draft.value = accentColor.value ?? DEFAULT_ACCENT_COLOR
  },
)

function close() {
  emit('update:visible', false)
}

function confirm() {
  if (setAccentColor(safeDraft.value)) close()
}

function reset() {
  setAccentColor(null)
  close()
}
</script>

<style lang="less" scoped>
.accent-dialog {
  padding-top: 4px;
}

.accent-dialog__desc {
  margin: 0 0 16px;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm);
  line-height: 1.6;
}

.accent-dialog__preview {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 18px;
  padding: 14px 16px;
  border-radius: var(--app-radius-md);
  font-size: var(--app-text-sm);
  font-weight: 600;
}

.accent-dialog__preview-hex {
  font-family: var(--td-font-family-mono);
  letter-spacing: 0.04em;
}

.accent-dialog__field {
  margin-bottom: 16px;
}

.accent-dialog__label {
  display: block;
  margin-bottom: 8px;
  color: var(--td-text-color-primary);
  font-size: var(--app-text-sm);
  font-weight: 600;
}

.accent-dialog__presets {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.accent-dialog__preset {
  width: 24px;
  height: 24px;
  padding: 0;
  border: 2px solid var(--td-bg-color-container);
  border-radius: var(--app-radius-pill);
  box-shadow: 0 0 0 1px var(--td-component-stroke);
  cursor: pointer;

  &.is-current {
    box-shadow: 0 0 0 2px var(--td-text-color-primary);
  }
}

.accent-dialog__warning {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: -4px 0 14px;
  color: var(--td-warning-color-6);
  font-size: var(--app-text-xs);
}

.accent-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 4px;
}

.accent-dialog__footer-hint {
  color: var(--td-text-color-placeholder);
  font-size: var(--app-text-xs);
}
</style>
