<template>
  <t-dialog
    :visible="visible"
    :header="t('theme.backdropTitle')"
    width="520px"
    placement="center"
    :confirm-btn="null"
    :cancel-btn="{ content: t('theme.backdropClose') }"
    @cancel="close"
  >
    <div class="backdrop-dialog">
      <p class="backdrop-dialog__desc">{{ t('theme.backdropDescription') }}</p>

      <div class="backdrop-dialog__section">
        <label class="backdrop-dialog__label">{{ t('theme.backdropPresets') }}</label>
        <div class="backdrop-dialog__grid">
          <button
            v-for="item in choices"
            :key="item.id"
            type="button"
            class="backdrop-card"
            :class="{ 'is-active': activeId === item.id }"
            :aria-pressed="activeId === item.id"
            @click="selectPreset(item.id)"
          >
            <span
              class="backdrop-card__preview"
              :style="{ background: item.preview }"
            >
              <span class="backdrop-card__mock-side" />
              <span class="backdrop-card__mock-body">
                <span class="backdrop-card__mock-line" />
                <span class="backdrop-card__mock-line" />
              </span>
            </span>
            <span class="backdrop-card__name">{{ item.label }}</span>
            <t-icon v-if="activeId === item.id" name="check" class="backdrop-card__check" size="14px" />
          </button>
        </div>
      </div>

      <div class="backdrop-dialog__section">
        <label class="backdrop-dialog__label">{{ t('theme.backdropCustom') }}</label>
        <div class="backdrop-dialog__upload">
          <button type="button" class="backdrop-dialog__upload-btn" @click="openPicker">
            <t-icon name="upload" size="16px" />
            <span>{{ t('theme.backdropUpload') }}</span>
          </button>
          <span v-if="activeId === 'custom'" class="backdrop-dialog__custom-active">
            <t-icon name="check-circle" size="14px" />
            {{ t('theme.backdropCustomActive') }}
          </span>
          <span class="backdrop-dialog__upload-hint">{{ t('theme.backdropUploadHint') }}</span>
        </div>
        <input
          ref="fileInput"
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          class="backdrop-dialog__file"
          @change="onFile"
        />
        <p v-if="uploadError" class="backdrop-dialog__error">{{ uploadError }}</p>
        <p v-if="uploading" class="backdrop-dialog__hint">{{ t('theme.backdropProcessing') }}</p>
      </div>

      <div class="backdrop-dialog__footer">
        <t-button variant="text" size="small" @click="reset">
          {{ t('theme.backdropReset') }}
        </t-button>
        <span class="backdrop-dialog__footer-hint">{{ t('theme.backdropHint') }}</span>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { useTheme } from '@/composables/useTheme'
import {
  BACKDROP_PRESETS,
  readBackdropImage,
  type BackdropId,
} from '@/composables/backdropTheme'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [value: boolean] }>()

const { t } = useI18n()
const { backdrop, setBackdrop } = useTheme()

const fileInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const uploadError = ref('')

const activeId = computed<BackdropId>(() => backdrop.value.id)

const choices = computed(() => [
  {
    id: 'none' as BackdropId,
    label: t('theme.backdropNone'),
    preview: 'linear-gradient(135deg, #f6f7f9, #e9ebef)',
  },
  ...BACKDROP_PRESETS.map((preset) => ({
    id: preset.id as BackdropId,
    label: t(preset.labelKey),
    preview: `linear-gradient(135deg, ${preset.swatch[0]}, ${preset.swatch[1]})`,
  })),
])

function close() {
  emit('update:visible', false)
}

function selectPreset(id: BackdropId) {
  uploadError.value = ''
  if (setBackdrop(id)) {
    MessagePlugin.success(t('theme.backdropApplied'))
  }
}

function openPicker() {
  uploadError.value = ''
  fileInput.value?.click()
}

async function onFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  uploading.value = true
  uploadError.value = ''
  try {
    const dataUrl = await readBackdropImage(file)
    if (setBackdrop('custom', dataUrl)) {
      MessagePlugin.success(t('theme.backdropApplied'))
      close()
    } else {
      uploadError.value = t('theme.backdropInvalid')
    }
  } catch (err) {
    const reason = err instanceof Error ? err.message : ''
    uploadError.value = reason === 'image-too-large'
      ? t('theme.backdropTooLarge')
      : t('theme.backdropInvalid')
  } finally {
    uploading.value = false
  }
}

function reset() {
  uploadError.value = ''
  setBackdrop('none')
  MessagePlugin.success(t('theme.backdropApplied'))
}
</script>

<style lang="less" scoped>
.backdrop-dialog {
  padding-top: 4px;
}

.backdrop-dialog__desc {
  margin: 0 0 18px;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm);
  line-height: 1.6;
}

.backdrop-dialog__section {
  margin-bottom: 20px;
}

.backdrop-dialog__label {
  display: block;
  margin-bottom: 10px;
  color: var(--td-text-color-primary);
  font-size: var(--app-text-sm);
  font-weight: 600;
}

.backdrop-dialog__grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}

.backdrop-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  background: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  font-family: inherit;
  cursor: pointer;
  transition: border-color var(--app-motion-fast) ease, box-shadow var(--app-motion-fast) ease;

  &:hover {
    border-color: var(--td-brand-color);
  }

  &.is-active {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 1px var(--td-brand-color);
  }
}

.backdrop-card__preview {
  display: flex;
  height: 52px;
  overflow: hidden;
  border-radius: var(--app-radius-sm);
}

.backdrop-card__mock-side {
  width: 26%;
  height: 100%;
  background: rgba(0, 0, 0, 0.14);
}

.backdrop-card__mock-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 7px;
  padding: 12px 10px;
}

.backdrop-card__mock-line {
  height: 6px;
  border-radius: var(--app-radius-pill);
  background: rgba(255, 255, 255, 0.62);
}

.backdrop-card__name {
  padding: 0 2px 2px;
  font-size: var(--app-text-sm);
  text-align: center;
  white-space: nowrap;
}

.backdrop-card__check {
  position: absolute;
  top: 10px;
  right: 10px;
  color: var(--td-brand-color);
}

.backdrop-dialog__upload {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.backdrop-dialog__upload-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: var(--app-radius-sm);
  background: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  font-family: inherit;
  font-size: var(--app-text-sm);
  cursor: pointer;

  &:hover {
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
  }
}

.backdrop-dialog__custom-active {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--td-brand-color);
  font-size: var(--app-text-sm);
}

.backdrop-dialog__upload-hint,
.backdrop-dialog__hint {
  color: var(--td-text-color-placeholder);
  font-size: var(--app-text-xs);
}

.backdrop-dialog__file {
  display: none;
}

.backdrop-dialog__error {
  margin: 8px 0 0;
  color: var(--td-error-color);
  font-size: var(--app-text-xs);
}

.backdrop-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 4px;
  border-top: 1px solid var(--td-component-stroke);
}

.backdrop-dialog__footer-hint {
  color: var(--td-text-color-placeholder);
  font-size: var(--app-text-xs);
}
</style>
